/*
 * qwk_test.c -- host unit tests for the QWK/REP packet codec.
 *
 *   cc -std=c89 -pedantic -Wall -Wextra -Icore \
 *      tests/qwk_test.c core/qwk.c -o /tmp/qwk_test && /tmp/qwk_test
 */
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include "qwk.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-50s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define MSGPATH  "/tmp/pigpen_qwk_messages.dat"
#define CTLPATH  "/tmp/pigpen_qwk_control.dat"

/* ---- collector for qwk_read ---- */
#define MAXMSG 8
typedef struct {
    int    n;
    pp_u16 conf[MAXMSG];
    pp_u32 number[MAXMSG];
    char   to[MAXMSG][32];
    char   from[MAXMSG][32];
    char   subject[MAXMSG][32];
    char   body[MAXMSG][4096];
} collector;

static void collect_cb(void *user, const pp_qwk_msg *m)
{
    collector *c = (collector *)user;
    if (c->n >= MAXMSG) return;
    c->conf[c->n]   = m->conference;
    c->number[c->n] = m->number;
    strcpy(c->to[c->n],      m->to);
    strcpy(c->from[c->n],    m->from);
    strcpy(c->subject[c->n], m->subject);
    strcpy(c->body[c->n],    m->body);
    c->n++;
}

/* count 0xE3 bytes and total blocks in MESSAGES.DAT */
static int count_0xE3(const char *path)
{
    FILE *f = fopen(path, "rb");
    int   n = 0;
    int   c;
    if (!f) return -1;
    while ((c = fgetc(f)) != EOF) if (c == 0xE3) n++;
    fclose(f);
    return n;
}

static long file_size(const char *path)
{
    FILE *f = fopen(path, "rb");
    long  sz;
    if (!f) return -1;
    fseek(f, 0L, SEEK_END);
    sz = ftell(f);
    fclose(f);
    return sz;
}

int main(void)
{
    pp_qwk_msg m;
    collector  col;
    int        written;
    int        readback;
    long       fsz;
    char       bigbody[600];
    int        i;

    remove(MSGPATH);
    remove(CTLPATH);

    /* ---- build a packet with 3 messages ---- */
    check("qwk_begin", qwk_begin(MSGPATH, "Vendetta/X") == 0);

    /* msg 1: short body, single text block */
    memset(&m, 0, sizeof m);
    m.conference = 1;
    m.number = 100;
    m.when = 1700000000u;
    strcpy(m.to, "All");
    strcpy(m.from, "Acid Burn");
    strcpy(m.subject, "Welcome");
    m.body = "Hello world.\nLine two.";
    check("qwk_add #1", qwk_add(&m) == 0);

    /* msg 2: empty-ish body (1 char), header-only-ish */
    memset(&m, 0, sizeof m);
    m.conference = 2;
    m.number = 101;
    m.when = 1700000500u;
    strcpy(m.to, "Zero Cool");
    strcpy(m.from, "Cereal Killer");
    strcpy(m.subject, "RE: Welcome");
    m.body = "ok";
    check("qwk_add #2", qwk_add(&m) == 0);

    /* msg 3: long body spanning multiple 128-byte blocks, with newlines */
    for (i = 0; i < (int)sizeof bigbody - 1; i++) {
        bigbody[i] = (char)('A' + (i % 26));
        if (i % 40 == 39) bigbody[i] = '\n';
    }
    bigbody[sizeof bigbody - 1] = '\0';
    /* ensure no trailing newline so round-trip compares exactly */
    if (bigbody[sizeof bigbody - 2] == '\n') bigbody[sizeof bigbody - 2] = 'Z';

    memset(&m, 0, sizeof m);
    m.conference = 7;
    m.number = 102;
    m.when = 1700001000u;
    strcpy(m.to, "Sysop");
    strcpy(m.from, "The Plague");
    strcpy(m.subject, "Garbage File");
    m.body = bigbody;
    check("qwk_add #3", qwk_add(&m) == 0);

    written = qwk_finish();
    check("qwk_finish count == 3", written == 3);

    /* ---- file shape ---- */
    fsz = file_size(MSGPATH);
    /* block0(1) + msg1(1+1) + msg2(1+1) + msg3(1 + ceil(599/128)=5) = 11 blocks */
    check("file is whole 128-byte blocks", fsz > 0 && (fsz % 128) == 0);
    check("block-0 present (>=128 bytes)", fsz >= 128);
    {
        long expect_blocks = 1 + 2 + 2 + (1 + 5);
        check("total block count matches headers", fsz == expect_blocks * 128);
    }
    check("0xE3 newlines present on disk", count_0xE3(MSGPATH) > 0);

    /* ---- read back ---- */
    memset(&col, 0, sizeof col);
    readback = qwk_read(MSGPATH, collect_cb, &col);
    check("qwk_read count == 3", readback == 3);
    check("collected 3", col.n == 3);

    /* msg 1 */
    check("m1 conference", col.conf[0] == 1);
    check("m1 number", col.number[0] == 100);
    check("m1 to", strcmp(col.to[0], "All") == 0);
    check("m1 from", strcmp(col.from[0], "Acid Burn") == 0);
    check("m1 subject", strcmp(col.subject[0], "Welcome") == 0);
    check("m1 body roundtrip", strcmp(col.body[0], "Hello world.\nLine two.") == 0);
    check("m1 no stray 0xE3 in body", strchr(col.body[0], (char)0xE3) == NULL);

    /* msg 2 */
    check("m2 conference", col.conf[1] == 2);
    check("m2 number", col.number[1] == 101);
    check("m2 from", strcmp(col.from[1], "Cereal Killer") == 0);
    check("m2 subject", strcmp(col.subject[1], "RE: Welcome") == 0);
    check("m2 body roundtrip", strcmp(col.body[1], "ok") == 0);

    /* msg 3 */
    check("m3 conference", col.conf[2] == 7);
    check("m3 number", col.number[2] == 102);
    check("m3 to", strcmp(col.to[2], "Sysop") == 0);
    check("m3 from", strcmp(col.from[2], "The Plague") == 0);
    check("m3 subject", strcmp(col.subject[2], "Garbage File") == 0);
    check("m3 multi-block body roundtrip", strcmp(col.body[2], bigbody) == 0);
    check("m3 newlines preserved", strchr(col.body[2], '\n') != NULL);
    check("m3 no stray 0xE3", strchr(col.body[2], (char)0xE3) == NULL);

    /* ---- malformed packet -> negative ---- */
    {
        /* truncate to block0 + a header claiming a block count but no text */
        FILE *f = fopen("/tmp/pigpen_qwk_bad.dat", "wb");
        check("open bad packet", f != NULL);
        if (f) {
            char blk[128];
            memset(blk, ' ', sizeof blk);
            fwrite(blk, 1, 128, f);            /* block 0 */
            /* header claiming 4 blocks but we write none of them */
            memset(blk, ' ', sizeof blk);
            memcpy(blk + 116, "4     ", 6);
            fwrite(blk, 1, 128, f);
            fclose(f);
        }
        check("qwk_read malformed < 0",
              qwk_read("/tmp/pigpen_qwk_bad.dat", collect_cb, &col) < 0);
        remove("/tmp/pigpen_qwk_bad.dat");
    }

    /* ---- CONTROL.DAT ---- */
    {
        const char *confs[3];
        confs[0] = "Main Board";
        confs[1] = "Programming";
        confs[2] = "Underground";
        check("qwk_write_control",
              qwk_write_control(CTLPATH, "Vendetta/X", "Cyberspace, NET",
                                "Hambone", confs, 3) == 0);
        check("CONTROL.DAT nonempty", file_size(CTLPATH) > 0);

        printf("\n---- CONTROL.DAT ----\n");
        {
            FILE *f = fopen(CTLPATH, "rb");
            int   c;
            if (f) {
                while ((c = fgetc(f)) != EOF) {
                    if (c == '\r') continue;          /* show LF only */
                    putchar(c);
                }
                fclose(f);
            }
        }
        printf("---------------------\n");
    }

    remove(MSGPATH);
    remove(CTLPATH);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all qwk tests passed");
    return g_fail;
}
