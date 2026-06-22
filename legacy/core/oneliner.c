/*
 * oneliner.c -- ONELINER.DAT, a newest-first ring of wall posts.
 * Loaded into memory on open; mutated and flushed on each push.
 */
#include <stdio.h>
#include <string.h>
#include "oneliner.h"

#define HDR_SIZE 16
#define OL_REC   100
#define OL_VERSION 1

static char        g_path[128];
static pp_oneliner g_ring[OL_MAX];
static int         g_n;

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

int ol_open(const char *path)
{
    FILE *f;
    pp_u8 h[HDR_SIZE], rec[OL_REC];
    pp_u32 count, i;

    g_n = 0;
    strncpy(g_path, path, sizeof g_path - 1);
    g_path[sizeof g_path - 1] = '\0';

    f = fopen(path, "rb");
    if (f == (FILE *)0) return 0;
    if (fread(h, 1, HDR_SIZE, f) != HDR_SIZE ||
        h[0] != 'P' || h[1] != 'O' || h[2] != 'N' || h[3] != 'E' ||
        get_u16(h + 6) != OL_REC) {
        fclose(f);
        return 0;
    }
    count = get_u32(h + 8);
    for (i = 0; i < count && g_n < OL_MAX; i++) {
        if (fread(rec, 1, OL_REC, f) != OL_REC) break;
        memcpy(g_ring[g_n].author, rec + 0,  23); g_ring[g_n].author[23] = '\0';
        memcpy(g_ring[g_n].text,   rec + 24, 71); g_ring[g_n].text[71] = '\0';
        g_ring[g_n].when = get_u32(rec + 96);
        g_n++;
    }
    fclose(f);
    return 0;
}

static void flush(void)
{
    FILE *f = fopen(g_path, "wb");
    pp_u8 h[HDR_SIZE], rec[OL_REC];
    int i;
    if (f == (FILE *)0) return;
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'O'; h[2] = 'N'; h[3] = 'E';
    put_u16(h + 4, OL_VERSION);
    put_u16(h + 6, OL_REC);
    put_u32(h + 8, (pp_u32)g_n);
    fwrite(h, 1, HDR_SIZE, f);
    for (i = 0; i < g_n; i++) {
        memset(rec, 0, OL_REC);
        put_str(rec + 0,  g_ring[i].author, 24);
        put_str(rec + 24, g_ring[i].text,   72);
        put_u32(rec + 96, g_ring[i].when);
        fwrite(rec, 1, OL_REC, f);
    }
    fclose(f);
}

void ol_push(const char *author, const char *text, pp_u32 when)
{
    int i, keep = (g_n < OL_MAX) ? g_n : OL_MAX - 1;
    for (i = keep; i > 0; i--)
        g_ring[i] = g_ring[i - 1];
    strncpy(g_ring[0].author, author, 23); g_ring[0].author[23] = '\0';
    strncpy(g_ring[0].text,   text,   71); g_ring[0].text[71] = '\0';
    g_ring[0].when = when;
    if (g_n < OL_MAX) g_n++;
    flush();
}

int ol_count(void) { return g_n; }

int ol_get(int i, pp_oneliner *out)
{
    if (i < 0 || i >= g_n) return 1;
    *out = g_ring[i];
    return 0;
}

void ol_close(void) { g_n = 0; }
