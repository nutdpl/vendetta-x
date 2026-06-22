/*
 * callers.c -- LASTCALL.DAT, a newest-first ring of recent callers.
 * Loaded into memory on open; mutated and flushed on each push.
 */
#include <stdio.h>
#include <string.h>
#include "callers.h"

#define HDR_SIZE 16
#define LC_REC   60
#define LC_VERSION 1

static char       g_path[128];
static pp_caller  g_ring[LC_MAX];
static int        g_n;

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

int lc_open(const char *path)
{
    FILE *f;
    pp_u8 h[HDR_SIZE], rec[LC_REC];
    pp_u32 count, i;

    g_n = 0;
    strncpy(g_path, path, sizeof g_path - 1);
    g_path[sizeof g_path - 1] = '\0';

    f = fopen(path, "rb");
    if (f == (FILE *)0) return 0;                 /* none yet: empty ring is fine */
    if (fread(h, 1, HDR_SIZE, f) != HDR_SIZE ||
        h[0] != 'P' || h[1] != 'L' || h[2] != 'C' || h[3] != 'L' ||
        get_u16(h + 6) != LC_REC) {
        fclose(f);
        return 0;                                  /* damaged: start empty */
    }
    count = get_u32(h + 8);
    for (i = 0; i < count && g_n < LC_MAX; i++) {
        if (fread(rec, 1, LC_REC, f) != LC_REC) break;
        memcpy(g_ring[g_n].handle,   rec + 0,  31);  g_ring[g_n].handle[31] = '\0';
        memcpy(g_ring[g_n].location, rec + 32, 23);  g_ring[g_n].location[23] = '\0';
        g_ring[g_n].when = get_u32(rec + 56);
        g_n++;
    }
    fclose(f);
    return 0;
}

static void flush(void)
{
    FILE *f = fopen(g_path, "wb");
    pp_u8 h[HDR_SIZE], rec[LC_REC];
    int i;
    if (f == (FILE *)0) return;
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'L'; h[2] = 'C'; h[3] = 'L';
    put_u16(h + 4, LC_VERSION);
    put_u16(h + 6, LC_REC);
    put_u32(h + 8, (pp_u32)g_n);
    fwrite(h, 1, HDR_SIZE, f);
    for (i = 0; i < g_n; i++) {
        memset(rec, 0, LC_REC);
        put_str(rec + 0,  g_ring[i].handle,   32);
        put_str(rec + 32, g_ring[i].location, 24);
        put_u32(rec + 56, g_ring[i].when);
        fwrite(rec, 1, LC_REC, f);
    }
    fclose(f);
}

void lc_push(const char *handle, const char *location, pp_u32 when)
{
    int i, keep = (g_n < LC_MAX) ? g_n : LC_MAX - 1;
    for (i = keep; i > 0; i--)          /* shift down to make room at [0] */
        g_ring[i] = g_ring[i - 1];
    strncpy(g_ring[0].handle, handle, 31);     g_ring[0].handle[31] = '\0';
    strncpy(g_ring[0].location, location, 23); g_ring[0].location[23] = '\0';
    g_ring[0].when = when;
    if (g_n < LC_MAX) g_n++;
    flush();
}

int lc_count(void) { return g_n; }

int lc_get(int i, pp_caller *out)
{
    if (i < 0 || i >= g_n) return 1;
    *out = g_ring[i];
    return 0;
}

void lc_close(void) { g_n = 0; }
