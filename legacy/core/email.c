/*
 * email.c -- data/MAIL.DAT, a flat append-only private mailbox.
 *
 * Record layout (little-endian, fixed PP_MAIL_REC bytes):
 *   off    0  from[32]
 *   off   32  to[32]       (recipient handle)
 *   off   64  subject[48]
 *   off  112  u32 when
 *   off  116  u32 flags    (bit0=read, bit1=deleted)
 *   off  120  body[600]    (NUL-padded)
 */
#include <stdio.h>
#include <string.h>
#include "email.h"

#define HDR_SIZE 16
#define EM_VERSION 1

static FILE *g_fp;
static int   g_count;
static pp_u8 g_rec[PP_MAIL_REC];    /* scratch, avoid big stack frames on DOS */

/* ---- little-endian field helpers ---------------------------------------- */

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

static int ci_eq(const char *a, const char *b)
{
    while (*a && *b) {
        int ca = *a, cb = *b;
        if (ca >= 'A' && ca <= 'Z') ca += 32;
        if (cb >= 'A' && cb <= 'Z') cb += 32;
        if (ca != cb) return 0;
        a++; b++;
    }
    return *a == *b;
}

/* ---- header ------------------------------------------------------------- */

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'E'; h[2] = 'M'; h[3] = 'L';
    put_u16(h + 4, EM_VERSION);
    put_u16(h + 6, PP_MAIL_REC);
    put_u32(h + 8, (pp_u32)g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- (de)serialization -------------------------------------------------- */

static void pack(const pp_mail *m, pp_u8 *rec)
{
    memset(rec, 0, PP_MAIL_REC);
    put_str(rec + 0,   m->from,    EM_FROM);
    put_str(rec + 32,  m->to,      EM_TO);
    put_str(rec + 64,  m->subject, EM_SUBJ);
    put_u32(rec + 112, m->when);
    put_u32(rec + 116, m->flags);
    put_str(rec + 120, m->body,    EM_BODY);
}

static void unpack(const pp_u8 *rec, pp_mail *m)
{
    get_str(m->from,    rec + 0,   EM_FROM);
    get_str(m->to,      rec + 32,  EM_TO);
    get_str(m->subject, rec + 64,  EM_SUBJ);
    m->when  = get_u32(rec + 112);
    m->flags = get_u32(rec + 116);
    get_str(m->body,    rec + 120, EM_BODY);
}

/* ---- public ------------------------------------------------------------- */

int em_open(const char *path)
{
    pp_u8 h[HDR_SIZE];

    g_count = 0;
    g_fp = fopen(path, "r+b");
    if (g_fp == (FILE *)0) {
        g_fp = fopen(path, "w+b");          /* create fresh */
        if (g_fp == (FILE *)0) return 1;
        return write_header();
    }
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE ||
        h[0] != 'P' || h[1] != 'E' || h[2] != 'M' || h[3] != 'L' ||
        get_u16(h + 6) != PP_MAIL_REC) {
        g_count = 0;                         /* empty or damaged -> reinit */
        return write_header();
    }
    g_count = (int)get_u32(h + 8);
    return 0;
}

void em_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
    g_count = 0;
}

int em_count(void) { return g_count; }

int em_get(int index, pp_mail *out)
{
    long off;
    if (g_fp == (FILE *)0 || index < 0 || index >= g_count) return 1;
    off = (long)HDR_SIZE + (long)index * PP_MAIL_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(g_rec, 1, PP_MAIL_REC, g_fp) != PP_MAIL_REC) return 1;
    unpack(g_rec, out);
    return 0;
}

int em_send(const pp_mail *m)
{
    long off;
    if (g_fp == (FILE *)0) return 1;
    pack(m, g_rec);
    off = (long)HDR_SIZE + (long)g_count * PP_MAIL_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(g_rec, 1, PP_MAIL_REC, g_fp) != PP_MAIL_REC) return 1;
    g_count++;
    return write_header();
}

int em_update(int index, const pp_mail *m)
{
    long off;
    if (g_fp == (FILE *)0 || index < 0 || index >= g_count) return 1;
    pack(m, g_rec);
    off = (long)HDR_SIZE + (long)index * PP_MAIL_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(g_rec, 1, PP_MAIL_REC, g_fp) != PP_MAIL_REC) return 1;
    fflush(g_fp);
    return 0;
}

int em_count_to(const char *handle)
{
    int i, n = 0;
    pp_mail m;
    for (i = 0; i < g_count; i++) {
        if (em_get(i, &m) != 0) continue;
        if (m.flags & EM_FLAG_DELETED) continue;
        if (ci_eq(m.to, handle)) n++;
    }
    return n;
}

int em_get_to(const char *handle, int n, pp_mail *out, int *out_index)
{
    int i, seen = 0;
    pp_mail m;
    if (n < 0) return 1;
    for (i = 0; i < g_count; i++) {
        if (em_get(i, &m) != 0) continue;
        if (m.flags & EM_FLAG_DELETED) continue;
        if (!ci_eq(m.to, handle)) continue;
        if (seen == n) {
            if (out) *out = m;
            if (out_index) *out_index = i;
            return 0;
        }
        seen++;
    }
    return 1;
}
