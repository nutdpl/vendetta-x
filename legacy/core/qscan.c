/*
 * qscan.c -- QSCAN.DAT, per-user/per-area read pointers (highest msg# read).
 * Loaded into memory on open; qs_set upserts on (user_idx, tag) and flushes.
 */
#include <stdio.h>
#include <string.h>
#include "qscan.h"

#define HDR_SIZE   16
#define QS_REC     22
#define QS_VERSION 1
#define TAG_LEN    16

typedef struct {
    pp_u16 user_idx;
    char   tag[TAG_LEN];    /* may be unterminated if full 16 bytes */
    pp_u32 lastread;
} pp_qscan;

static char     g_path[128];
static pp_qscan g_rec[QS_MAX];
static int      g_n;

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

/* Compare a stored tag (fixed 16, possibly unterminated) against a C string. */
static int tag_eq(const char *stored, const char *want)
{
    int i;
    for (i = 0; i < TAG_LEN; i++) {
        char c = stored[i];
        if (want[i] == '\0') return c == '\0';
        if (c != want[i]) return 0;
    }
    /* stored filled all 16 bytes; equal only if want is exactly 16 long */
    return want[TAG_LEN] == '\0';
}

static int find(int user_idx, const char *tag)
{
    int i;
    for (i = 0; i < g_n; i++)
        if (g_rec[i].user_idx == (pp_u16)user_idx && tag_eq(g_rec[i].tag, tag))
            return i;
    return -1;
}

int qs_open(const char *path)
{
    FILE  *f;
    pp_u8  h[HDR_SIZE], rec[QS_REC];
    pp_u32 count, i;

    g_n = 0;
    strncpy(g_path, path, sizeof g_path - 1);
    g_path[sizeof g_path - 1] = '\0';

    f = fopen(path, "rb");
    if (f == (FILE *)0) return 0;
    if (fread(h, 1, HDR_SIZE, f) != HDR_SIZE ||
        h[0] != 'P' || h[1] != 'Q' || h[2] != 'S' || h[3] != 'C' ||
        get_u16(h + 6) != QS_REC) {
        fclose(f);
        return 0;
    }
    count = get_u32(h + 8);
    for (i = 0; i < count && g_n < QS_MAX; i++) {
        if (fread(rec, 1, QS_REC, f) != QS_REC) break;
        g_rec[g_n].user_idx = get_u16(rec + 0);
        memcpy(g_rec[g_n].tag, rec + 2, TAG_LEN);
        g_rec[g_n].lastread = get_u32(rec + 18);
        g_n++;
    }
    fclose(f);
    return 0;
}

static void flush(void)
{
    FILE *f = fopen(g_path, "wb");
    pp_u8 h[HDR_SIZE], rec[QS_REC];
    int   i;
    if (f == (FILE *)0) return;
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'Q'; h[2] = 'S'; h[3] = 'C';
    put_u16(h + 4, QS_VERSION);
    put_u16(h + 6, QS_REC);
    put_u32(h + 8, (pp_u32)g_n);
    fwrite(h, 1, HDR_SIZE, f);
    for (i = 0; i < g_n; i++) {
        memset(rec, 0, QS_REC);
        put_u16(rec + 0, g_rec[i].user_idx);
        put_str(rec + 2, g_rec[i].tag, TAG_LEN);
        put_u32(rec + 18, g_rec[i].lastread);
        fwrite(rec, 1, QS_REC, f);
    }
    fclose(f);
}

pp_u32 qs_get(int user_idx, const char *tag)
{
    int i = find(user_idx, tag);
    return (i < 0) ? (pp_u32)0 : g_rec[i].lastread;
}

void qs_set(int user_idx, const char *tag, pp_u32 lastread)
{
    int i = find(user_idx, tag);
    if (i >= 0) {
        g_rec[i].lastread = lastread;
    } else {
        if (g_n >= QS_MAX) return;
        g_rec[g_n].user_idx = (pp_u16)user_idx;
        put_str((pp_u8 *)g_rec[g_n].tag, tag, TAG_LEN);
        g_rec[g_n].lastread = lastread;
        g_n++;
    }
    flush();
}

int qs_count(void) { return g_n; }

void qs_close(void) { g_n = 0; }
