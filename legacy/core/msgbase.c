/*
 * msgbase.c -- per-area message store (data/<TAG>.MSG).
 *
 * Record layout (little-endian, fixed PP_MSG_REC bytes):
 *   off    0  from[32]
 *   off   32  to[32]
 *   off   64  subject[48]
 *   off  112  u32 when
 *   off  116  u32 flags
 *   off  120  body[960]   (CRLF-joined lines, NUL-padded)
 *   off 1080  u32 reply_to (1-based parent msg number; 0 = root) -- v2
 */
#include <stdio.h>
#include <string.h>
#include "msgbase.h"

#define HDR_SIZE 16
#define MB_VERSION 2

static FILE *g_fp;
static int   g_count;
static pp_u8 g_rec[PP_MSG_REC];     /* scratch, avoid big stack frames on DOS */

static void put_u16(pp_u8 *p, pp_u16 v) { p[0] = (pp_u8)(v & 0xff); p[1] = (pp_u8)((v >> 8) & 0xff); }
static void put_u32(pp_u8 *p, pp_u32 v)
{
    p[0] = (pp_u8)(v & 0xff);         p[1] = (pp_u8)((v >> 8) & 0xff);
    p[2] = (pp_u8)((v >> 16) & 0xff); p[3] = (pp_u8)((v >> 24) & 0xff);
}
static pp_u16 get_u16(const pp_u8 *p) { return (pp_u16)(p[0] | (p[1] << 8)); }
static pp_u32 get_u32(const pp_u8 *p)
{
    return (pp_u32)p[0] | ((pp_u32)p[1] << 8) | ((pp_u32)p[2] << 16) | ((pp_u32)p[3] << 24);
}
static void put_str(pp_u8 *p, const char *s, int max)
{
    int i = 0;
    while (i < max && s[i]) { p[i] = (pp_u8)s[i]; i++; }
    while (i < max) { p[i] = 0; i++; }
}
static void get_str(char *d, const pp_u8 *p, int max)
{
    memcpy(d, p, (size_t)(max - 1));
    d[max - 1] = '\0';
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'M'; h[2] = 'S'; h[3] = 'G';
    put_u16(h + 4, MB_VERSION);
    put_u16(h + 6, PP_MSG_REC);
    put_u32(h + 8, (pp_u32)g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

int mb_open(const char *tag)
{
    char path[80];
    pp_u8 h[HDR_SIZE];

    g_count = 0;
    sprintf(path, "data/%s.MSG", tag);
    g_fp = fopen(path, "r+b");
    if (g_fp == (FILE *)0) {
        g_fp = fopen(path, "w+b");
        if (g_fp == (FILE *)0) return 1;
        return write_header();
    }
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE ||
        h[0] != 'P' || h[1] != 'M' || h[2] != 'S' || h[3] != 'G' ||
        get_u16(h + 6) != PP_MSG_REC) {
        g_count = 0;
        return write_header();
    }
    g_count = (int)get_u32(h + 8);
    return 0;
}

void mb_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
    g_count = 0;
}

int mb_count(void) { return g_count; }

int mb_get(int index, pp_msg *out)
{
    long off;
    if (g_fp == (FILE *)0 || index < 0 || index >= g_count) return 1;
    off = (long)HDR_SIZE + (long)index * PP_MSG_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(g_rec, 1, PP_MSG_REC, g_fp) != PP_MSG_REC) return 1;
    get_str(out->from,    g_rec + 0,   MB_FROM);
    get_str(out->to,      g_rec + 32,  MB_TO);
    get_str(out->subject, g_rec + 64,  MB_SUBJ);
    out->when  = get_u32(g_rec + 112);
    out->flags = get_u32(g_rec + 116);
    get_str(out->body,    g_rec + 120, MB_BODY);
    out->reply_to = get_u32(g_rec + 1080);
    return 0;
}

int mb_add(const pp_msg *m)
{
    long off;
    if (g_fp == (FILE *)0) return 1;
    memset(g_rec, 0, PP_MSG_REC);
    put_str(g_rec + 0,   m->from,    MB_FROM);
    put_str(g_rec + 32,  m->to,      MB_TO);
    put_str(g_rec + 64,  m->subject, MB_SUBJ);
    put_u32(g_rec + 112, m->when);
    put_u32(g_rec + 116, m->flags);
    put_str(g_rec + 120, m->body,    MB_BODY);
    put_u32(g_rec + 1080, m->reply_to);
    off = (long)HDR_SIZE + (long)g_count * PP_MSG_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(g_rec, 1, PP_MSG_REC, g_fp) != PP_MSG_REC) return 1;
    g_count++;
    return write_header();
}

static void q_app(char *out, int *pos, int outsz, const char *s)
{
    while (*s && *pos < outsz - 1) out[(*pos)++] = *s++;
}

int mb_quote(const char *src, const char *who, char *out, int outsz)
{
    int pos = 0;
    const char *p = src;

    if (outsz <= 0) return 0;
    q_app(out, &pos, outsz, (who && *who) ? who : "someone");
    q_app(out, &pos, outsz, " wrote:\r\n");

    while (p && *p && pos < outsz - 1) {
        q_app(out, &pos, outsz, "> ");
        while (*p && *p != '\n' && pos < outsz - 1) {
            if (*p != '\r') out[pos++] = *p;
            p++;
        }
        if (*p == '\n') p++;
        q_app(out, &pos, outsz, "\r\n");
    }
    out[pos] = '\0';
    return pos;
}
