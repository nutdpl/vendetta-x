/*
 * userbase.c -- USER.DAT persistence.
 *
 * File layout (all little-endian):
 *   header (16 bytes): "PUSR" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then `count` records of PP_USER_REC (384) bytes each:
 *     off  0  handle[32]   (NUL-padded)
 *     off 32  location[24]
 *     off 56  u8 acs
 *     off 57  u8 flags
 *     off 58  2 pad
 *     off 60  u32 times_called
 *     off 64  u32 first_call
 *     off 68  u32 last_call
 *     off 72  u32 posts
 *     off 76  tagline[48]
 *     off 124 group[16]
 *     off 140 u16 ar
 *     off 142 u16 dar
 *     off 144 u16 restr
 *     off 146 14 reserved
 */
#include <stdio.h>
#include <string.h>
#include "userbase.h"

#define HDR_SIZE 16
#define UB_VERSION 3

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

void ub_pack(const pp_user *u, pp_u8 *rec)
{
    memset(rec, 0, PP_USER_REC);
    put_str(rec + 0,  u->handle,   PP_HANDLE_MAX);
    put_str(rec + 32, u->location, PP_LOC_MAX);
    rec[56] = u->sl;
    rec[57] = u->flags;
    rec[58] = u->dsl;
    rec[59] = u->prot;
    put_u32(rec + 60, u->times_called);
    put_u32(rec + 64, u->first_call);
    put_u32(rec + 68, u->last_call);
    put_u32(rec + 72, u->posts);
    put_str(rec + 76,  u->tagline, PP_TAG_MAX);
    put_str(rec + 124, u->group,   PP_GRP_MAX);
    put_u16(rec + 140, u->ar);
    put_u16(rec + 142, u->dar);
    put_u16(rec + 144, u->restr);
    /* new-user application profile (appended past the original 160-byte record) */
    put_str(rec + 160, u->real_name, PP_RNAME_MAX);
    put_str(rec + 196, u->email,     PP_EMAIL_MAX);
    put_str(rec + 244, u->birthdate, PP_BDATE_MAX);
    put_str(rec + 256, u->city,      PP_CITY_MAX);
    put_str(rec + 284, u->zip,       PP_ZIP_MAX);
    put_str(rec + 296, u->computer,  PP_COMP_MAX);
    put_str(rec + 320, u->pwhash,    PP_PW_MAX);
}

void ub_unpack(const pp_u8 *rec, pp_user *u)
{
    memset(u, 0, sizeof *u);
    memcpy(u->handle,   rec + 0,  PP_HANDLE_MAX - 1);
    memcpy(u->location, rec + 32, PP_LOC_MAX - 1);
    u->sl           = rec[56];
    u->flags        = rec[57];
    u->dsl          = rec[58];
    u->prot         = rec[59];
    u->times_called = get_u32(rec + 60);
    u->first_call   = get_u32(rec + 64);
    u->last_call    = get_u32(rec + 68);
    u->posts        = get_u32(rec + 72);
    memcpy(u->tagline, rec + 76,  PP_TAG_MAX - 1); u->tagline[PP_TAG_MAX - 1] = '\0';
    memcpy(u->group,   rec + 124, PP_GRP_MAX - 1); u->group[PP_GRP_MAX - 1]   = '\0';
    u->ar           = get_u16(rec + 140);
    u->dar          = get_u16(rec + 142);
    u->restr        = get_u16(rec + 144);
    memcpy(u->real_name, rec + 160, PP_RNAME_MAX - 1); u->real_name[PP_RNAME_MAX - 1] = '\0';
    memcpy(u->email,     rec + 196, PP_EMAIL_MAX - 1); u->email[PP_EMAIL_MAX - 1]     = '\0';
    memcpy(u->birthdate, rec + 244, PP_BDATE_MAX - 1); u->birthdate[PP_BDATE_MAX - 1] = '\0';
    memcpy(u->city,      rec + 256, PP_CITY_MAX - 1);  u->city[PP_CITY_MAX - 1]       = '\0';
    memcpy(u->zip,       rec + 284, PP_ZIP_MAX - 1);   u->zip[PP_ZIP_MAX - 1]         = '\0';
    memcpy(u->computer,  rec + 296, PP_COMP_MAX - 1);  u->computer[PP_COMP_MAX - 1]   = '\0';
    memcpy(u->pwhash,    rec + 320, PP_PW_MAX - 1);    u->pwhash[PP_PW_MAX - 1]       = '\0';
}

/* ---- header ------------------------------------------------------------- */

static int read_header(void)
{
    pp_u8 h[HDR_SIZE];
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    if (h[0] != 'P' || h[1] != 'U' || h[2] != 'S' || h[3] != 'R') return 1;
    if (get_u16(h + 6) != PP_USER_REC) return 1;     /* record size mismatch */
    g_count = get_u32(h + 8);
    return 0;
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'U'; h[2] = 'S'; h[3] = 'R';
    put_u16(h + 4, UB_VERSION);
    put_u16(h + 6, PP_USER_REC);
    put_u32(h + 8, g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- public ------------------------------------------------------------- */

int ub_open(const char *path)
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

void ub_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
}

pp_u32 ub_count(void) { return g_count; }

int ub_get(pp_u32 index, pp_user *out)
{
    pp_u8 rec[PP_USER_REC];
    long off;
    if (g_fp == (FILE *)0 || index >= g_count) return 1;
    off = (long)HDR_SIZE + (long)index * PP_USER_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(rec, 1, PP_USER_REC, g_fp) != PP_USER_REC) return 1;
    ub_unpack(rec, out);
    return 0;
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

int ub_find(const char *handle, pp_user *out, pp_u32 *index)
{
    pp_u32 i;
    pp_user u;
    for (i = 0; i < g_count; i++) {
        if (ub_get(i, &u) != 0) continue;
        if (ci_eq(u.handle, handle)) {
            if (out) *out = u;
            if (index) *index = i;
            return 1;
        }
    }
    return 0;
}

int ub_update(pp_u32 index, const pp_user *u)
{
    pp_u8 rec[PP_USER_REC];
    long off;
    if (g_fp == (FILE *)0 || index >= g_count) return 1;
    ub_pack(u, rec);
    off = (long)HDR_SIZE + (long)index * PP_USER_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(rec, 1, PP_USER_REC, g_fp) != PP_USER_REC) return 1;
    fflush(g_fp);
    return 0;
}

int ub_add(const pp_user *u, pp_u32 *index)
{
    pp_u8 rec[PP_USER_REC];
    long off;
    if (g_fp == (FILE *)0) return 1;
    ub_pack(u, rec);
    off = (long)HDR_SIZE + (long)g_count * PP_USER_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(rec, 1, PP_USER_REC, g_fp) != PP_USER_REC) return 1;
    if (index) *index = g_count;
    g_count++;
    return write_header();
}
