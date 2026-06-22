/*
 * bbslist.c -- BBSLIST.DAT persistence (user-contributed board registry).
 *
 * File layout (all little-endian):
 *   header (16 bytes): "PBBS" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then `count` records of PP_BBS_REC (160) bytes each:
 *     off   0  name[40]      (NUL-padded)
 *     off  40  address[48]
 *     off  88  sysop[32]
 *     off 120  added_by[32]
 *     off 152  u32 when
 *     off 156  4 reserved
 *
 * Append-only, newest-last: bl_add writes at the end of file.
 */
#include <stdio.h>
#include <string.h>
#include "bbslist.h"

#define HDR_SIZE 16
#define BL_VERSION 1

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

void bl_pack(const pp_bbs *b, pp_u8 *rec)
{
    memset(rec, 0, PP_BBS_REC);
    put_str(rec + 0,   b->name,     PP_BBS_NAME_MAX);
    put_str(rec + 40,  b->address,  PP_BBS_ADDR_MAX);
    put_str(rec + 88,  b->sysop,    PP_BBS_SYSOP_MAX);
    put_str(rec + 120, b->added_by, PP_BBS_BY_MAX);
    put_u32(rec + 152, b->when);
}

void bl_unpack(const pp_u8 *rec, pp_bbs *b)
{
    memset(b, 0, sizeof *b);
    memcpy(b->name,     rec + 0,   PP_BBS_NAME_MAX - 1);  b->name[PP_BBS_NAME_MAX - 1]   = '\0';
    memcpy(b->address,  rec + 40,  PP_BBS_ADDR_MAX - 1);  b->address[PP_BBS_ADDR_MAX - 1] = '\0';
    memcpy(b->sysop,    rec + 88,  PP_BBS_SYSOP_MAX - 1); b->sysop[PP_BBS_SYSOP_MAX - 1] = '\0';
    memcpy(b->added_by, rec + 120, PP_BBS_BY_MAX - 1);    b->added_by[PP_BBS_BY_MAX - 1] = '\0';
    b->when = get_u32(rec + 152);
}

/* ---- header ------------------------------------------------------------- */

static int read_header(void)
{
    pp_u8 h[HDR_SIZE];
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    if (h[0] != 'P' || h[1] != 'B' || h[2] != 'B' || h[3] != 'S') return 1;
    if (get_u16(h + 6) != PP_BBS_REC) return 1;     /* record size mismatch */
    g_count = get_u32(h + 8);
    return 0;
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'B'; h[2] = 'B'; h[3] = 'S';
    put_u16(h + 4, BL_VERSION);
    put_u16(h + 6, PP_BBS_REC);
    put_u32(h + 8, g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- public ------------------------------------------------------------- */

int bl_open(const char *path)
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

void bl_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
}

pp_u32 bl_count(void) { return g_count; }

int bl_get(pp_u32 index, pp_bbs *out)
{
    pp_u8 rec[PP_BBS_REC];
    long off;
    if (g_fp == (FILE *)0 || index >= g_count) return 1;
    off = (long)HDR_SIZE + (long)index * PP_BBS_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(rec, 1, PP_BBS_REC, g_fp) != PP_BBS_REC) return 1;
    bl_unpack(rec, out);
    return 0;
}

int bl_add(const pp_bbs *b)
{
    pp_u8 rec[PP_BBS_REC];
    long off;
    if (g_fp == (FILE *)0) return 1;
    bl_pack(b, rec);
    off = (long)HDR_SIZE + (long)g_count * PP_BBS_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(rec, 1, PP_BBS_REC, g_fp) != PP_BBS_REC) return 1;
    g_count++;
    return write_header();
}
