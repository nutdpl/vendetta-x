/*
 * page.c -- NODEMSG.DAT, the inter-node message bus.
 *
 * File layout (all little-endian):
 *   header (16 bytes): "PMSG" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then `count` records of PP_NMSG_REC (128) bytes each:
 *     off   0  u32 seq
 *     off   4  u16 to_node
 *     off   6  u16 from_node
 *     off   8  u8  kind
 *     off   9  from_handle[32]   (NUL-padded)
 *     off  41  text[80]          (NUL-padded)
 *     off 121  u32 when
 *     off 125  3 reserved (pad to 128)
 *
 * Append-only, newest-last: pg_send writes at the end of file and never
 * rewrites or deletes records. seq is assigned monotonically; since records
 * are never removed, seq == (count just before the append) + 1, i.e. the Nth
 * record (1-based) always carries seq N. We therefore derive seq from the
 * header count, which is the highest seq on file.
 */
#include <stdio.h>
#include <string.h>
#include "page.h"

#define HDR_SIZE 16
#define PG_VERSION 1

static FILE  *g_fp;
static pp_u32 g_count;

/* ---- little-endian field helpers ---------------------------------------- */

static void put_u16(pp_u8 *p, pp_u16 v) { p[0] = (pp_u8)(v & 0xff); p[1] = (pp_u8)((v >> 8) & 0xff); }
static void put_u32(pp_u8 *p, pp_u32 v)
{
    p[0] = (pp_u8)(v & 0xff);          p[1] = (pp_u8)((v >> 8) & 0xff);
    p[2] = (pp_u8)((v >> 16) & 0xff);  p[3] = (pp_u8)((v >> 24) & 0xff);
}
static pp_u16 get_u16(const pp_u8 *p) { return (pp_u16)(p[0] | (p[1] << 8)); }
static pp_u32 get_u32(const pp_u8 *p)
{
    return (pp_u32)p[0] | ((pp_u32)p[1] << 8) | ((pp_u32)p[2] << 16) | ((pp_u32)p[3] << 24);
}

/* copy a NUL-padded fixed field */
static void put_str(pp_u8 *p, const char *s, int max)
{
    int i = 0;
    while (i < max && s[i]) { p[i] = (pp_u8)s[i]; i++; }
    while (i < max) { p[i] = 0; i++; }
}

void pg_pack(const pp_nodemsg *m, pp_u8 *rec)
{
    memset(rec, 0, PP_NMSG_REC);
    put_u32(rec + 0,  m->seq);
    put_u16(rec + 4,  m->to_node);
    put_u16(rec + 6,  m->from_node);
    rec[8] = m->kind;
    put_str(rec + 9,  m->from_handle, PP_MSG_HANDLE_MAX);
    put_str(rec + 41, m->text,        PP_MSG_TEXT_MAX);
    put_u32(rec + 121, m->when);
}

void pg_unpack(const pp_u8 *rec, pp_nodemsg *m)
{
    memset(m, 0, sizeof *m);
    m->seq       = get_u32(rec + 0);
    m->to_node   = get_u16(rec + 4);
    m->from_node = get_u16(rec + 6);
    m->kind      = rec[8];
    memcpy(m->from_handle, rec + 9,  PP_MSG_HANDLE_MAX - 1); m->from_handle[PP_MSG_HANDLE_MAX - 1] = '\0';
    memcpy(m->text,        rec + 41, PP_MSG_TEXT_MAX - 1);   m->text[PP_MSG_TEXT_MAX - 1] = '\0';
    m->when      = get_u32(rec + 121);
}

/* ---- header ------------------------------------------------------------- */

static int read_header(void)
{
    pp_u8 h[HDR_SIZE];
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    if (h[0] != 'P' || h[1] != 'M' || h[2] != 'S' || h[3] != 'G') return 1;
    if (get_u16(h + 6) != PP_NMSG_REC) return 1;     /* record size mismatch */
    g_count = get_u32(h + 8);
    return 0;
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'M'; h[2] = 'S'; h[3] = 'G';
    put_u16(h + 4, PG_VERSION);
    put_u16(h + 6, PP_NMSG_REC);
    put_u32(h + 8, g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- public ------------------------------------------------------------- */

int pg_open(const char *path)
{
    g_count = 0;
    g_fp = fopen(path, "r+b");
    if (g_fp == (FILE *)0) {
        g_fp = fopen(path, "w+b");      /* create fresh */
        if (g_fp == (FILE *)0) return 1;
        return write_header();
    }
    if (read_header() != 0) {           /* empty or damaged -> reinitialize */
        g_count = 0;
        return write_header();
    }
    return 0;
}

void pg_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
}

pp_u32 pg_count(void) { return g_count; }

pp_u32 pg_seq(void) { return g_count; }   /* seq == count: append-only, never deleted */

pp_u32 pg_send(int to_node, int from_node, const char *from_handle,
               int kind, const char *text, pp_u32 when)
{
    pp_nodemsg m;
    pp_u8 rec[PP_NMSG_REC];
    long off;

    if (g_fp == (FILE *)0) return 0;

    /* Re-read the header so seq is monotonic even with concurrent openers
     * having appended since we opened. */
    if (read_header() != 0) return 0;

    memset(&m, 0, sizeof m);
    m.seq       = g_count + 1;
    m.to_node   = (pp_u16)to_node;
    m.from_node = (pp_u16)from_node;
    m.kind      = (pp_u8)kind;
    if (from_handle) { strncpy(m.from_handle, from_handle, PP_MSG_HANDLE_MAX - 1); }
    if (text)        { strncpy(m.text,        text,        PP_MSG_TEXT_MAX - 1); }
    m.from_handle[PP_MSG_HANDLE_MAX - 1] = '\0';
    m.text[PP_MSG_TEXT_MAX - 1] = '\0';
    m.when = when;

    pg_pack(&m, rec);
    off = (long)HDR_SIZE + (long)g_count * PP_NMSG_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 0;
    if (fwrite(rec, 1, PP_NMSG_REC, g_fp) != PP_NMSG_REC) return 0;
    g_count++;
    if (write_header() != 0) return 0;
    return m.seq;
}

int pg_poll(int node_no, pp_u32 *cursor, pp_nodemsg *out)
{
    pp_u8 rec[PP_NMSG_REC];
    pp_nodemsg m;
    pp_u32 i;

    if (g_fp == (FILE *)0 || cursor == (pp_u32 *)0 || out == (pp_nodemsg *)0) return 0;

    /* Pick up any records appended by other openers. */
    if (read_header() != 0) return 0;

    for (i = 0; i < g_count; i++) {
        long off = (long)HDR_SIZE + (long)i * PP_NMSG_REC;
        if (fseek(g_fp, off, SEEK_SET) != 0) return 0;
        if (fread(rec, 1, PP_NMSG_REC, g_fp) != PP_NMSG_REC) return 0;
        pg_unpack(rec, &m);
        if (m.seq > *cursor && m.to_node == (pp_u16)node_no) {
            *out = m;
            *cursor = m.seq;
            return 1;
        }
    }
    return 0;
}
