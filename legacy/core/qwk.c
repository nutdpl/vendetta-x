/*
 * qwk.c -- QWK/REP offline-mail packet FORMAT (MESSAGES.DAT + CONTROL.DAT).
 *
 * MESSAGES.DAT is a stream of 128-byte blocks:
 *   block 0   : 128-byte text header (vendor stamp + board name, space-padded)
 *   per msg   : ONE 128-byte header block, then N text blocks.
 *
 * Header block fixed fields (ASCII, space-padded, PUBLISHED offsets):
 *   off   0 : status flag           1   (space = public unread)
 *   off   1 : message number        7   left-justified ASCII
 *   off   8 : date "MM-DD-YY"        8
 *   off  16 : time "HH:MM"           5
 *   off  21 : To                    25
 *   off  46 : From                  25
 *   off  71 : Subject               25
 *   off  96 : password             12   (spaces)
 *   off 108 : reference number      8   (spaces or "0")
 *   off 116 : block count           6   ASCII, INCLUDING the header block
 *   off 122 : active flag           1   (0xE1 = active)
 *   off 123 : conference number     2   little-endian u16
 *   off 125 : logical-message ind.  3   (spaces)
 *                                 ----
 *                                  128
 *
 * Body text: each '\n' stored as 0xE3; final block space-padded to 128.
 * blockcount = 1 + ceil(bodylen / 128).
 */
#include <stdio.h>
#include <string.h>
#include <time.h>
#include "qwk.h"

#define QWK_CR 0xE3   /* on-disk substitute for an end-of-line */

static FILE  *g_fp;
static pp_u32 g_count;

/* ---- field helpers ------------------------------------------------------ */

/* copy up to max bytes of s into p, then space-pad to max. */
static void put_field(pp_u8 *p, const char *s, int max)
{
    int i = 0;
    while (i < max && s[i]) { p[i] = (pp_u8)s[i]; i++; }
    while (i < max) { p[i] = (pp_u8)' '; i++; }
}

/* render an unsigned value as decimal into a max-wide LEFT-justified,
 * space-padded ASCII field. */
static void put_num(pp_u8 *p, pp_u32 v, int max)
{
    char tmp[12];
    int  n = 0;
    int  i;
    if (v == 0) {
        tmp[n++] = '0';
    } else {
        while (v > 0 && n < (int)sizeof tmp) { tmp[n++] = (char)('0' + (int)(v % 10)); v /= 10; }
    }
    /* tmp holds the digits reversed; emit them forwards, then pad. */
    for (i = 0; i < max; i++) {
        p[i] = (n > 0) ? (pp_u8)tmp[--n] : (pp_u8)' ';
    }
}

/* parse a left-justified ASCII unsigned decimal out of a fixed field. */
static pp_u32 get_num(const pp_u8 *p, int max)
{
    pp_u32 v = 0;
    int    i;
    for (i = 0; i < max; i++) {
        pp_u8 c = p[i];
        if (c >= '0' && c <= '9') v = v * 10 + (pp_u32)(c - '0');
        else if (c == ' ') continue;
        else break;
    }
    return v;
}

/* copy a fixed field, trim trailing spaces, NUL-terminate into out[outsz]. */
static void get_field(const pp_u8 *p, int max, char *out, int outsz)
{
    int n = (max < outsz - 1) ? max : outsz - 1;
    int i;
    for (i = 0; i < n; i++) out[i] = (char)p[i];
    while (i > 0 && (out[i - 1] == ' ' || out[i - 1] == '\0')) i--;
    out[i] = '\0';
}

static void put_u16le(pp_u8 *p, pp_u16 v)
{
    p[0] = (pp_u8)(v & 0xff);
    p[1] = (pp_u8)((v >> 8) & 0xff);
}

static pp_u16 get_u16le(const pp_u8 *p)
{
    return (pp_u16)(p[0] | (p[1] << 8));
}

/* ---- writing ------------------------------------------------------------ */

int qwk_begin(const char *msgpath, const char *board)
{
    pp_u8 blk[QWK_BLOCK];
    char  stamp[QWK_BLOCK + 1];
    int   n;

    g_count = 0;
    g_fp = fopen(msgpath, "wb");
    if (g_fp == (FILE *)0) return 1;

    /* "Produced by Vendetta/X" + the board name, the rest space-padded. */
    stamp[0] = '\0';
    strcpy(stamp, "Produced by Vendetta/X <");
    n = (int)strlen(stamp);
    if (board) {
        int i = 0;
        while (board[i] && n < QWK_BLOCK - 1) { stamp[n++] = board[i++]; }
    }
    if (n < QWK_BLOCK - 1) stamp[n++] = '>';
    stamp[n] = '\0';

    put_field(blk, stamp, QWK_BLOCK);
    if (fwrite(blk, 1, QWK_BLOCK, g_fp) != QWK_BLOCK) {
        fclose(g_fp); g_fp = (FILE *)0; return 1;
    }
    return 0;
}

int qwk_add(const pp_qwk_msg *m)
{
    pp_u8      hdr[QWK_BLOCK];
    pp_u8      txt[QWK_BLOCK];
    char       datebuf[16];
    char       timebuf[16];
    const char *body;
    long       bodylen;
    long       textblocks;
    pp_u32     blockcount;
    long       i;
    int        col;
    struct tm *tmv;
    time_t     t;

    if (g_fp == (FILE *)0 || m == (const pp_qwk_msg *)0) return 1;

    body = m->body ? m->body : "";
    bodylen = (long)strlen(body);
    textblocks = (bodylen + QWK_BLOCK - 1) / QWK_BLOCK;   /* ceil */
    if (bodylen == 0) textblocks = 0;
    blockcount = (pp_u32)(1 + textblocks);

    /* format date/time from the unix timestamp. */
    t = (time_t)m->when;
    tmv = gmtime(&t);
    if (tmv != (struct tm *)0) {
        sprintf(datebuf, "%02d-%02d-%02d",
                tmv->tm_mon + 1, tmv->tm_mday, (tmv->tm_year + 1900) % 100);
        sprintf(timebuf, "%02d:%02d", tmv->tm_hour, tmv->tm_min);
    } else {
        strcpy(datebuf, "01-01-80");
        strcpy(timebuf, "00:00");
    }

    /* ---- header block ---- */
    memset(hdr, ' ', QWK_BLOCK);
    hdr[0] = (pp_u8)' ';                            /* status: public unread */
    put_num(hdr + 1, m->number, 7);                 /* message number */
    put_field(hdr + 8,  datebuf, 8);                /* MM-DD-YY */
    put_field(hdr + 16, timebuf, 5);                /* HH:MM */
    put_field(hdr + 21, m->to,      QWK_TO_MAX);
    put_field(hdr + 46, m->from,    QWK_FROM_MAX);
    put_field(hdr + 71, m->subject, QWK_SUBJ_MAX);
    put_field(hdr + 96, "", 12);                    /* password (spaces) */
    put_field(hdr + 108, "0", 8);                   /* reference number */
    put_num(hdr + 116, blockcount, 6);              /* block count incl. header */
    hdr[122] = (pp_u8)0xE1;                          /* active */
    put_u16le(hdr + 123, m->conference);            /* conference number */
    hdr[125] = (pp_u8)' ';                          /* logical-message ind. */
    hdr[126] = (pp_u8)' ';
    hdr[127] = (pp_u8)' ';

    if (fwrite(hdr, 1, QWK_BLOCK, g_fp) != QWK_BLOCK) return 1;

    /* ---- text blocks ---- */
    col = 0;
    for (i = 0; i < bodylen; i++) {
        char c = body[i];
        txt[col++] = (c == '\n') ? (pp_u8)QWK_CR : (pp_u8)c;
        if (col == QWK_BLOCK) {
            if (fwrite(txt, 1, QWK_BLOCK, g_fp) != QWK_BLOCK) return 1;
            col = 0;
        }
    }
    if (col > 0) {                                  /* pad final block */
        while (col < QWK_BLOCK) txt[col++] = (pp_u8)' ';
        if (fwrite(txt, 1, QWK_BLOCK, g_fp) != QWK_BLOCK) return 1;
    }

    g_count++;
    return 0;
}

int qwk_finish(void)
{
    int n = (int)g_count;
    if (g_fp) {
        fflush(g_fp);
        fclose(g_fp);
        g_fp = (FILE *)0;
    }
    return n;
}

/* ---- CONTROL.DAT -------------------------------------------------------- */

int qwk_write_control(const char *path, const char *board, const char *location,
                      const char *sysop, const char **conf_names, int nconf)
{
    FILE      *f;
    int        i;
    int        maxconf;
    time_t     t;
    struct tm *tmv;
    char       datebuf[32];

    f = fopen(path, "wb");
    if (f == (FILE *)0) return 1;

    t = time((time_t *)0);
    tmv = gmtime(&t);
    if (tmv != (struct tm *)0) {
        sprintf(datebuf, "%02d-%02d-%04d,%02d:%02d:%02d",
                tmv->tm_mon + 1, tmv->tm_mday, tmv->tm_year + 1900,
                tmv->tm_hour, tmv->tm_min, tmv->tm_sec);
    } else {
        strcpy(datebuf, "01-01-1980,00:00:00");
    }

    /* Published CONTROL.DAT layout, CRLF lines:
     *   board name
     *   location (city, state)
     *   sysop phone (placeholder)
     *   "00000000,BBSID"  (serial + board id)
     *   sysop name
     *   date "MM-DD-YYYY,HH:MM:SS"
     *   caller (the QWK user) name
     *   "0"  (mail counter / placeholder)
     *   max conference number (count - 1)
     *   then per conference: number line, name line
     */
    fprintf(f, "%s\r\n", board ? board : "");
    fprintf(f, "%s\r\n", location ? location : "");
    fprintf(f, "000-000-0000\r\n");
    fprintf(f, "00000000,VENDETTAX\r\n");
    fprintf(f, "%s\r\n", sysop ? sysop : "");
    fprintf(f, "%s\r\n", datebuf);
    fprintf(f, "%s\r\n", "VENDETTAX");
    fprintf(f, "0\r\n");
    fprintf(f, "0\r\n");

    maxconf = (nconf > 0) ? nconf - 1 : 0;
    fprintf(f, "%d\r\n", maxconf);
    for (i = 0; i < nconf; i++) {
        fprintf(f, "%d\r\n", i);
        fprintf(f, "%s\r\n", (conf_names && conf_names[i]) ? conf_names[i] : "");
    }

    fflush(f);
    fclose(f);
    return 0;
}

/* ---- reading ------------------------------------------------------------ */

int qwk_read(const char *msgpath, qwk_msg_cb cb, void *user)
{
    static char body[8 * 1024];          /* reused body buffer */
    pp_u8       hdr[QWK_BLOCK];
    pp_u8       txt[QWK_BLOCK];
    FILE       *f;
    pp_qwk_msg  m;
    pp_u32      blockcount;
    long        textblocks;
    long        b;
    int         j;
    int         pos;
    int         count = 0;

    f = fopen(msgpath, "rb");
    if (f == (FILE *)0) return -1;

    /* skip block 0 */
    if (fread(hdr, 1, QWK_BLOCK, f) != QWK_BLOCK) { fclose(f); return -1; }

    for (;;) {
        size_t got = fread(hdr, 1, QWK_BLOCK, f);
        if (got == 0) break;                       /* clean EOF */
        if (got != QWK_BLOCK) { fclose(f); return -1; }

        blockcount = get_num(hdr + 116, 6);
        if (blockcount == 0) { fclose(f); return -1; }   /* malformed */
        textblocks = (long)blockcount - 1;

        memset(&m, 0, sizeof m);
        m.number     = get_num(hdr + 1, 7);
        m.conference = get_u16le(hdr + 123);
        get_field(hdr + 21, QWK_TO_MAX,   m.to,      (int)sizeof m.to);
        get_field(hdr + 46, QWK_FROM_MAX, m.from,    (int)sizeof m.from);
        get_field(hdr + 71, QWK_SUBJ_MAX, m.subject, (int)sizeof m.subject);

        /* read text blocks into the reused buffer */
        pos = 0;
        for (b = 0; b < textblocks; b++) {
            if (fread(txt, 1, QWK_BLOCK, f) != QWK_BLOCK) { fclose(f); return -1; }
            for (j = 0; j < QWK_BLOCK; j++) {
                char c = (txt[j] == (pp_u8)QWK_CR) ? '\n' : (char)txt[j];
                if (pos < (int)sizeof body - 1) body[pos++] = c;
            }
        }
        /* trim trailing ^Z / space padding (but keep interior newlines). */
        while (pos > 0 && (body[pos - 1] == ' ' || body[pos - 1] == 0x1A ||
                           body[pos - 1] == '\n' || body[pos - 1] == '\0')) {
            pos--;
        }
        body[pos] = '\0';
        m.body = body;

        if (cb) cb(user, &m);
        count++;
    }

    fclose(f);
    return count;
}
