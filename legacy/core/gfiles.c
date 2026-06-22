/*
 * gfiles.c -- GFILES.DAT persistence (G-FILES / BULLETINS catalog).
 *
 * File layout (all little-endian):
 *   header (16 bytes): "PGFL" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then `count` records of PP_GF_REC (144) bytes each:
 *     off   0  title[48]   (NUL-padded)
 *     off  48  file[64]
 *     off 112  acs[32]
 *
 * Append-only, newest-last: gf_add writes at the end of file. This module
 * is the CATALOG only -- it never opens or renders a bulletin's file; that
 * is the caller's responsibility (as is evaluating the acs expression).
 */
#include <stdio.h>
#include <string.h>
#include "gfiles.h"

#define HDR_SIZE   16
#define GF_VERSION 1

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

void gf_pack(const pp_gfile *g, pp_u8 *rec)
{
    memset(rec, 0, PP_GF_REC);
    put_str(rec + 0,   g->title, PP_GF_TITLE_MAX);
    put_str(rec + 48,  g->file,  PP_GF_FILE_MAX);
    put_str(rec + 112, g->acs,   PP_GF_ACS_MAX);
}

void gf_unpack(const pp_u8 *rec, pp_gfile *g)
{
    memset(g, 0, sizeof *g);
    memcpy(g->title, rec + 0,   PP_GF_TITLE_MAX - 1); g->title[PP_GF_TITLE_MAX - 1] = '\0';
    memcpy(g->file,  rec + 48,  PP_GF_FILE_MAX - 1);  g->file[PP_GF_FILE_MAX - 1]  = '\0';
    memcpy(g->acs,   rec + 112, PP_GF_ACS_MAX - 1);   g->acs[PP_GF_ACS_MAX - 1]    = '\0';
}

/* ---- header ------------------------------------------------------------- */

static int read_header(void)
{
    pp_u8 h[HDR_SIZE];
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    if (h[0] != 'P' || h[1] != 'G' || h[2] != 'F' || h[3] != 'L') return 1;
    if (get_u16(h + 6) != PP_GF_REC) return 1;      /* record size mismatch */
    g_count = get_u32(h + 8);
    return 0;
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'G'; h[2] = 'F'; h[3] = 'L';
    put_u16(h + 4, GF_VERSION);
    put_u16(h + 6, PP_GF_REC);
    put_u32(h + 8, g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- public ------------------------------------------------------------- */

int gf_open(const char *path)
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

void gf_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
}

pp_u32 gf_count(void) { return g_count; }

int gf_get(int i, pp_gfile *out)
{
    pp_u8 rec[PP_GF_REC];
    long off;
    if (g_fp == (FILE *)0 || i < 0 || (pp_u32)i >= g_count) return 1;
    off = (long)HDR_SIZE + (long)i * PP_GF_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(rec, 1, PP_GF_REC, g_fp) != PP_GF_REC) return 1;
    gf_unpack(rec, out);
    return 0;
}

int gf_add(const pp_gfile *g)
{
    pp_u8 rec[PP_GF_REC];
    long off;
    if (g_fp == (FILE *)0) return 1;
    gf_pack(g, rec);
    off = (long)HDR_SIZE + (long)g_count * PP_GF_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(rec, 1, PP_GF_REC, g_fp) != PP_GF_REC) return 1;
    g_count++;
    return write_header();
}
