/*
 * door.c -- DOOR support: DOORS.DAT registry + drop-file generation.
 *
 * Part 1: DOORS.DAT (magic "PDOR"), modeled exactly on bbslist.c.
 *   header (16 bytes): "PDOR" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then `count` records of PP_DOOR_REC (144) bytes each:
 *     off   0  name[32]   (NUL-padded)
 *     off  32  cmd[64]
 *     off  96  type u8
 *     off  97  acs[32]
 *     off 129  pad to 144
 *   Append-only, newest-last.
 *
 * Part 2: drop-file writers -- pure text output.
 *   door_write_doorsys -- DOOR.SYS, the 52-line GAP/Synchronet standard.
 *   door_write_dorinfo -- DORINFO1.DEF, the RBBS/QBBS standard.
 *
 * Clean-room: published field layouts implemented from scratch.
 */
#include <stdio.h>
#include <string.h>
#include "door.h"

#define HDR_SIZE 16
#define DR_VERSION 1

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

void dr_pack(const pp_door *d, pp_u8 *rec)
{
    memset(rec, 0, PP_DOOR_REC);
    put_str(rec + 0,  d->name, PP_DOOR_NAME_MAX);
    put_str(rec + 32, d->cmd,  PP_DOOR_CMD_MAX);
    rec[96] = d->type;
    put_str(rec + 97, d->acs,  PP_DOOR_ACS_MAX);
    /* off 129..143 already zeroed by memset */
}

void dr_unpack(const pp_u8 *rec, pp_door *d)
{
    memset(d, 0, sizeof *d);
    memcpy(d->name, rec + 0,  PP_DOOR_NAME_MAX - 1); d->name[PP_DOOR_NAME_MAX - 1] = '\0';
    memcpy(d->cmd,  rec + 32, PP_DOOR_CMD_MAX - 1);  d->cmd[PP_DOOR_CMD_MAX - 1]   = '\0';
    d->type = rec[96];
    memcpy(d->acs,  rec + 97, PP_DOOR_ACS_MAX - 1);  d->acs[PP_DOOR_ACS_MAX - 1]   = '\0';
}

/* ---- header ------------------------------------------------------------- */

static int read_header(void)
{
    pp_u8 h[HDR_SIZE];
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    if (h[0] != 'P' || h[1] != 'D' || h[2] != 'O' || h[3] != 'R') return 1;
    if (get_u16(h + 6) != PP_DOOR_REC) return 1;    /* record size mismatch */
    g_count = get_u32(h + 8);
    return 0;
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'D'; h[2] = 'O'; h[3] = 'R';
    put_u16(h + 4, DR_VERSION);
    put_u16(h + 6, PP_DOOR_REC);
    put_u32(h + 8, g_count);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- public registry ---------------------------------------------------- */

int dr_open(const char *path)
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

void dr_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
}

pp_u32 dr_count(void) { return g_count; }

int dr_get(pp_u32 index, pp_door *out)
{
    pp_u8 rec[PP_DOOR_REC];
    long off;
    if (g_fp == (FILE *)0 || index >= g_count) return 1;
    off = (long)HDR_SIZE + (long)index * PP_DOOR_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fread(rec, 1, PP_DOOR_REC, g_fp) != PP_DOOR_REC) return 1;
    dr_unpack(rec, out);
    return 0;
}

int dr_add(const pp_door *d)
{
    pp_u8 rec[PP_DOOR_REC];
    long off;
    if (g_fp == (FILE *)0) return 1;
    dr_pack(d, rec);
    off = (long)HDR_SIZE + (long)g_count * PP_DOOR_REC;
    if (fseek(g_fp, off, SEEK_SET) != 0) return 1;
    if (fwrite(rec, 1, PP_DOOR_REC, g_fp) != PP_DOOR_REC) return 1;
    g_count++;
    return write_header();
}

/* ---- drop-file writers -------------------------------------------------- */

/* split s->handle into first / last on the first space.
 * first/last are caller-provided buffers of at least `n` bytes. */
static void split_name(const char *handle, char *first, char *last, int n)
{
    int i;
    const char *sp;
    first[0] = '\0';
    last[0] = '\0';
    if (handle == (const char *)0) return;
    sp = strchr(handle, ' ');
    if (sp == (const char *)0) {
        i = 0;
        while (handle[i] && i < n - 1) { first[i] = handle[i]; i++; }
        first[i] = '\0';
        return;
    }
    i = 0;
    while (&handle[i] < sp && i < n - 1) { first[i] = handle[i]; i++; }
    first[i] = '\0';
    sp++;                       /* skip the space */
    i = 0;
    while (sp[i] && i < n - 1) { last[i] = sp[i]; i++; }
    last[i] = '\0';
}

/* safe accessor for an optional const string */
static const char *opt(const char *s) { return s ? s : ""; }

/*
 * DOOR.SYS -- the 52-line GAP/Synchronet dialect. One field per line, CRLF.
 *
 *  1  COM port (0 = local/telnet)
 *  2  baud rate
 *  3  data bits (8)
 *  4  node number
 *  5  DTE rate / locked baud
 *  6  screen display ("Y")
 *  7  printer toggle ("N")
 *  8  page bell ("N")
 *  9  caller alarm ("N")
 * 10  user full name (we use the handle)
 * 11  calling-from / location (city, state)
 * 12  home phone
 * 13  work/data phone
 * 14  password
 * 15  security level
 * 16  total times on
 * 17  last call date (MM/DD/YY)
 * 18  seconds remaining this call
 * 19  minutes remaining this call
 * 20  graphics mode ("GR" ANSI / "NG" none / "7E")
 * 21  page length (lines)
 * 22  expert mode ("Y"/"N")
 * 23  conferences registered in
 * 24  conference exited to door from
 * 25  user expiration date (MM/DD/YY)
 * 26  user record number
 * 27  default protocol
 * 28  total uploads
 * 29  total downloads
 * 30  daily download K total
 * 31  daily download max K
 * 32  birthday (MM/DD/YY)
 * 33  full path to USERFILE / main dir
 * 34  full path to GEN/menu dir
 * 35  sysop's name (comment)
 * 36  caller's handle/alias
 * 37  next event time (HH:MM)
 * 38  error-correcting connection ("Y"/"N")
 * 39  is ANSI in NG mode ("Y"/"N")
 * 40  use record locking ("Y"/"N")
 * 41  BBS default text color
 * 42  time credits in minutes
 * 43  last new-files scan date (MM/DD/YY)
 * 44  time of this call (HH:MM)
 * 45  time of last call (HH:MM)
 * 46  max daily files allowed
 * 47  files downloaded today
 * 48  total K uploaded ever
 * 49  total K downloaded ever
 * 50  user comment
 * 51  total doors opened
 * 52  total messages left
 */
int door_write_doorsys(const char *path, const pp_door_session *s)
{
    FILE *f;
    long mins_left;
    f = fopen(path, "wb");
    if (f == (FILE *)0) return 1;

    mins_left = s->seconds_left / 60;

    fprintf(f, "0\r\n");                              /* 1  COM port (local) */
    fprintf(f, "%d\r\n", s->baud);                    /* 2  baud */
    fprintf(f, "8\r\n");                              /* 3  data bits */
    fprintf(f, "%d\r\n", s->node);                    /* 4  node */
    fprintf(f, "%d\r\n", s->baud);                    /* 5  locked DTE rate */
    fprintf(f, "Y\r\n");                              /* 6  screen display */
    fprintf(f, "N\r\n");                              /* 7  printer */
    fprintf(f, "N\r\n");                              /* 8  page bell */
    fprintf(f, "N\r\n");                              /* 9  caller alarm */
    fprintf(f, "%s\r\n", opt(s->handle));             /* 10 user full name */
    fprintf(f, "%s\r\n", opt(s->location));           /* 11 calling from */
    fprintf(f, "000-000-0000\r\n");                   /* 12 home phone */
    fprintf(f, "000-000-0000\r\n");                   /* 13 work phone */
    fprintf(f, "PASSWORD\r\n");                       /* 14 password */
    fprintf(f, "%d\r\n", s->sl);                      /* 15 security level */
    fprintf(f, "%lu\r\n", (unsigned long)s->times_on);/* 16 times on */
    fprintf(f, "01/01/00\r\n");                       /* 17 last call date */
    fprintf(f, "%ld\r\n", s->seconds_left);           /* 18 seconds left */
    fprintf(f, "%ld\r\n", mins_left);                 /* 19 minutes left */
    fprintf(f, "%s\r\n", s->ansi ? "GR" : "NG");      /* 20 graphics mode */
    fprintf(f, "24\r\n");                             /* 21 page length */
    fprintf(f, "N\r\n");                              /* 22 expert mode */
    fprintf(f, "1\r\n");                              /* 23 conferences in */
    fprintf(f, "1\r\n");                              /* 24 conf exited from */
    fprintf(f, "12/31/99\r\n");                       /* 25 expiration date */
    fprintf(f, "%d\r\n", s->node);                    /* 26 user record number */
    fprintf(f, "Z\r\n");                              /* 27 default protocol */
    fprintf(f, "0\r\n");                              /* 28 total uploads */
    fprintf(f, "0\r\n");                              /* 29 total downloads */
    fprintf(f, "0\r\n");                              /* 30 daily dl K total */
    fprintf(f, "0\r\n");                              /* 31 daily dl max K */
    fprintf(f, "01/01/70\r\n");                       /* 32 birthday */
    fprintf(f, "C:\\BBS\\\r\n");                       /* 33 userfile/main dir */
    fprintf(f, "C:\\BBS\\GEN\\\r\n");                  /* 34 gen/menu dir */
    fprintf(f, "Vendetta/X\r\n");                     /* 35 sysop name */
    fprintf(f, "%s\r\n", opt(s->handle));             /* 36 caller handle */
    fprintf(f, "00:00\r\n");                          /* 37 next event time */
    fprintf(f, "Y\r\n");                              /* 38 error-correcting */
    fprintf(f, "%s\r\n", s->ansi ? "Y" : "N");        /* 39 ANSI in NG mode */
    fprintf(f, "N\r\n");                              /* 40 record locking */
    fprintf(f, "7\r\n");                              /* 41 default text color */
    fprintf(f, "%ld\r\n", mins_left);                 /* 42 time credits (min) */
    fprintf(f, "01/01/00\r\n");                       /* 43 last newfiles scan */
    fprintf(f, "00:00\r\n");                          /* 44 time of this call */
    fprintf(f, "00:00\r\n");                          /* 45 time of last call */
    fprintf(f, "0\r\n");                              /* 46 max daily files */
    fprintf(f, "0\r\n");                              /* 47 files dl today */
    fprintf(f, "0\r\n");                              /* 48 total K uploaded */
    fprintf(f, "0\r\n");                              /* 49 total K downloaded */
    fprintf(f, "Vendetta/X\r\n");                     /* 50 user comment */
    fprintf(f, "0\r\n");                              /* 51 total doors */
    fprintf(f, "0\r\n");                              /* 52 total messages */

    if (ferror(f)) { fclose(f); return 1; }
    if (fclose(f) != 0) return 1;
    return 0;
}

/*
 * DORINFO1.DEF -- the RBBS/QBBS dialect. 13 lines, CRLF.
 *
 *  1  system / board name
 *  2  sysop first name
 *  3  sysop last name
 *  4  COM port ("COM0" -> "0" local)
 *  5  baud,parity,databits,stopbits  (e.g. "0 BAUD,N,8,1")
 *  6  network type / indicator ("0")
 *  7  caller first name
 *  8  caller last name
 *  9  caller city/location
 * 10  ANSI flag (1 = yes, 0 = no)
 * 11  security level
 * 12  time left, in minutes
 * 13  FOSSIL flag ("-1")
 */
int door_write_dorinfo(const char *path, const pp_door_session *s)
{
    FILE *f;
    char first[64];
    char last[64];
    f = fopen(path, "wb");
    if (f == (FILE *)0) return 1;

    split_name(s->handle, first, last, (int)sizeof first);

    fprintf(f, "Vendetta/X\r\n");                     /* 1  system name */
    fprintf(f, "Sysop\r\n");                          /* 2  sysop first */
    fprintf(f, "Vendetta\r\n");                       /* 3  sysop last */
    fprintf(f, "0\r\n");                              /* 4  COM port (local) */
    fprintf(f, "%d BAUD,N,8,1\r\n", s->baud);         /* 5  baud/parity line */
    fprintf(f, "0\r\n");                              /* 6  network indicator */
    fprintf(f, "%s\r\n", first);                      /* 7  caller first */
    fprintf(f, "%s\r\n", last);                       /* 8  caller last */
    fprintf(f, "%s\r\n", opt(s->location));           /* 9  caller city */
    fprintf(f, "%d\r\n", s->ansi ? 1 : 0);            /* 10 ANSI flag */
    fprintf(f, "%d\r\n", s->sl);                      /* 11 security level */
    fprintf(f, "%ld\r\n", s->seconds_left / 60);      /* 12 time left (min) */
    fprintf(f, "-1\r\n");                             /* 13 FOSSIL flag */

    if (ferror(f)) { fclose(f); return 1; }
    if (fclose(f) != 0) return 1;
    return 0;
}
