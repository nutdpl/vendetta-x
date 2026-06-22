/*
 * filebase.c -- per-area file catalog (data/<TAG>.FIL).
 *
 * Record layout (little-endian, fixed FB_REC bytes):
 *   off    0  name[16]      (8.3 filename, NUL-padded)
 *   off   16  desc[64]
 *   off   80  u32 size
 *   off   84  uploader[32]
 *   off  116  u32 when
 *   off  120  u32 downloads
 *   off  124  u32 flags
 *
 * No transfer protocol -- this is just the catalog/listing layer. One area
 * open at a time, mirroring core/msgbase.c.
 */
#include <stdio.h>
#include <string.h>
#include "filebase.h"

#define HDR_SIZE 16
#define FB_VERSION 1

static FILE *g_fp;
static int   g_count;
static pp_u8 g_rec[FB_REC];     /* scratch, avoid big stack frames on DOS */

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
    h[0] = 'P'; h[1] = 'F'; h[2] = 'I'; h[3] = 'L';
    put_u16(h + 4, FB_VERSION);
    put_u16(h + 6, FB_REC);
    put_u32(h + 8, (pp_u32)g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

int fb_open(const char *tag)
{
    char path[80];
    pp_u8 h[HDR_SIZE];

    g_count = 0;
    sprintf(path, "data/%s.FIL", tag);
    g_fp = fopen(path, "r+b");
    if (g_fp == (FILE *)0) {
        g_fp = fopen(path, "w+b");
        if (g_fp == (FILE *)0) return 1;
        return write_header();
    }
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE ||
        h[0] != 'P' || h[1] != 'F' || h[2] != 'I' || h[3] != 'L' ||
        get_u16(h + 6) != FB_REC) {
        g_count = 0;
        return write_header();
    }
    g_count = (int)get_u32(h + 8);
    return 0;
}

void fb_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
    g_count = 0;
}

int fb_count(void) { return g_count; }

int fb_get(int index, pp_file *out)
{
    long off;
    if (g_fp == (FILE *)0 || index < 0 || index >= g_count) return 1;
    off = (long)HDR_SIZE + (long)index * FB_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(g_rec, 1, FB_REC, g_fp) != FB_REC) return 1;
    get_str(out->name,     g_rec + 0,   FB_NAME);
    get_str(out->desc,     g_rec + 16,  FB_DESC);
    out->size      = get_u32(g_rec + 80);
    get_str(out->uploader, g_rec + 84,  FB_UPLOADER);
    out->when      = get_u32(g_rec + 116);
    out->downloads = get_u32(g_rec + 120);
    out->flags     = get_u32(g_rec + 124);
    return 0;
}

int fb_add(const pp_file *f)
{
    long off;
    if (g_fp == (FILE *)0) return 1;
    memset(g_rec, 0, FB_REC);
    put_str(g_rec + 0,   f->name,     FB_NAME);
    put_str(g_rec + 16,  f->desc,     FB_DESC);
    put_u32(g_rec + 80,  f->size);
    put_str(g_rec + 84,  f->uploader, FB_UPLOADER);
    put_u32(g_rec + 116, f->when);
    put_u32(g_rec + 120, f->downloads);
    put_u32(g_rec + 124, f->flags);
    off = (long)HDR_SIZE + (long)g_count * FB_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(g_rec, 1, FB_REC, g_fp) != FB_REC) return 1;
    g_count++;
    return write_header();
}

int fb_inc_downloads(int index)
{
    long off;
    pp_u32 dl;
    if (g_fp == (FILE *)0 || index < 0 || index >= g_count) return 1;
    off = (long)HDR_SIZE + (long)index * FB_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(g_rec, 1, FB_REC, g_fp) != FB_REC) return 1;
    dl = get_u32(g_rec + 120) + 1;
    put_u32(g_rec + 120, dl);
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(g_rec, 1, FB_REC, g_fp) != FB_REC) return 1;
    fflush(g_fp);
    return 0;
}
