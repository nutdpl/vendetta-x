/*
 * voting.c -- VOTE.DAT persistence (the voting booth).
 *
 * File layout (all little-endian, serialized field by field):
 *   header (16 bytes): "PVOT" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then `count` records of VT_REC (480) bytes each:
 *     off   0  question[80]   (NUL-padded)
 *     off  80  u8  noptions
 *     off  81  options[8][40] (8 * 40 = 320, each NUL-padded)
 *     off 401  u32 counts[8]  (8 * 4 = 32)
 *     off 433  u32 when
 *     off 437  voted[32]      (256-bit bitset of user indices)
 *     off 469  11 reserved
 */
#include <stdio.h>
#include <string.h>
#include "voting.h"

#define HDR_SIZE   16
#define VT_REC     480
#define VT_VERSION 1

#define OFF_QUESTION 0
#define OFF_NOPT     80
#define OFF_OPTIONS  81
#define OFF_COUNTS   401
#define OFF_WHEN     433
#define OFF_VOTED    437

static FILE *g_fp;
static int   g_count;

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

/* ---- record pack/unpack ------------------------------------------------- */

static void pack(const pp_poll *p, pp_u8 *rec)
{
    int i;
    memset(rec, 0, VT_REC);
    put_str(rec + OFF_QUESTION, p->question, VT_Q_MAX);
    rec[OFF_NOPT] = p->noptions;
    for (i = 0; i < VT_OPTS_MAX; i++)
        put_str(rec + OFF_OPTIONS + i * VT_OPT_MAX, p->options[i], VT_OPT_MAX);
    for (i = 0; i < VT_OPTS_MAX; i++)
        put_u32(rec + OFF_COUNTS + i * 4, p->counts[i]);
    put_u32(rec + OFF_WHEN, p->when);
    memcpy(rec + OFF_VOTED, p->voted, VT_VOTED_LEN);
}

static void unpack(const pp_u8 *rec, pp_poll *p)
{
    int i;
    memset(p, 0, sizeof *p);
    memcpy(p->question, rec + OFF_QUESTION, VT_Q_MAX - 1);
    p->question[VT_Q_MAX - 1] = '\0';
    p->noptions = rec[OFF_NOPT];
    for (i = 0; i < VT_OPTS_MAX; i++) {
        memcpy(p->options[i], rec + OFF_OPTIONS + i * VT_OPT_MAX, VT_OPT_MAX - 1);
        p->options[i][VT_OPT_MAX - 1] = '\0';
    }
    for (i = 0; i < VT_OPTS_MAX; i++)
        p->counts[i] = get_u32(rec + OFF_COUNTS + i * 4);
    p->when = get_u32(rec + OFF_WHEN);
    memcpy(p->voted, rec + OFF_VOTED, VT_VOTED_LEN);
}

/* ---- header ------------------------------------------------------------- */

static int read_header(void)
{
    pp_u8 h[HDR_SIZE];
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    if (h[0] != 'P' || h[1] != 'V' || h[2] != 'O' || h[3] != 'T') return 1;
    if (get_u16(h + 6) != VT_REC) return 1;     /* record size mismatch */
    g_count = (int)get_u32(h + 8);
    return 0;
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'V'; h[2] = 'O'; h[3] = 'T';
    put_u16(h + 4, VT_VERSION);
    put_u16(h + 6, VT_REC);
    put_u32(h + 8, (pp_u32)g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- public ------------------------------------------------------------- */

int vt_open(const char *path)
{
    g_count = 0;
    g_fp = fopen(path, "r+b");
    if (g_fp == (FILE *)0) {
        g_fp = fopen(path, "w+b");          /* create fresh */
        if (g_fp == (FILE *)0) return 1;
        return write_header();
    }
    if (read_header() != 0) {               /* empty or damaged -> reinitialize */
        g_count = 0;
        return write_header();
    }
    return 0;
}

void vt_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
}

int vt_count(void) { return g_count; }

int vt_get(int i, pp_poll *out)
{
    pp_u8 rec[VT_REC];
    long off;
    if (g_fp == (FILE *)0 || i < 0 || i >= g_count) return -1;
    off = (long)HDR_SIZE + (long)i * VT_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return -1;
    if (fread(rec, 1, VT_REC, g_fp) != VT_REC) return -1;
    unpack(rec, out);
    return 0;
}

int vt_add(const pp_poll *p)
{
    pp_u8 rec[VT_REC];
    long off;
    if (g_fp == (FILE *)0) return -1;
    pack(p, rec);
    off = (long)HDR_SIZE + (long)g_count * VT_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return -1;
    if (fwrite(rec, 1, VT_REC, g_fp) != VT_REC) return -1;
    g_count++;
    if (write_header() != 0) return -1;
    return 0;
}

static int put_record(int i, const pp_poll *p)
{
    pp_u8 rec[VT_REC];
    long off;
    if (g_fp == (FILE *)0 || i < 0 || i >= g_count) return -1;
    pack(p, rec);
    off = (long)HDR_SIZE + (long)i * VT_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return -1;
    if (fwrite(rec, 1, VT_REC, g_fp) != VT_REC) return -1;
    fflush(g_fp);
    return 0;
}

int vt_has_voted(int poll_idx, int user_idx)
{
    pp_poll p;
    if (user_idx < 0 || user_idx >= VT_VOTED_LEN * 8) return 0;
    if (vt_get(poll_idx, &p) != 0) return 0;
    return (p.voted[user_idx >> 3] & (1 << (user_idx & 7))) ? 1 : 0;
}

int vt_cast(int poll_idx, int option, int user_idx)
{
    pp_poll p;
    if (user_idx < 0 || user_idx >= VT_VOTED_LEN * 8) return -1;
    if (vt_get(poll_idx, &p) != 0) return -1;
    if (option < 0 || option >= (int)p.noptions) return -1;
    if (p.voted[user_idx >> 3] & (1 << (user_idx & 7))) return -1;  /* already voted */
    p.counts[option]++;
    p.voted[user_idx >> 3] |= (pp_u8)(1 << (user_idx & 7));
    if (put_record(poll_idx, &p) != 0) return -1;
    return 0;
}
