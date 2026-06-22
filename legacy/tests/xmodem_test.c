/*
 * xmodem_test.c -- host unit tests for the XMODEM/XMODEM-1K/YMODEM codec.
 *
 * Three layers:
 *   (a) xm_crc16 against the canonical "123456789" vector and a zero block.
 *   (b) xm_build_block -> xm_check_block round-trips (128/1024, checksum/CRC),
 *       plus a corruption check that must fail validation.
 *   (c) a full in-memory LOOPBACK driving xm_send against xm_recv.
 *
 * Loopback design (see also the run-summary printed at the end):
 *   Two byte ring buffers wire the endpoints together -- s2r carries
 *   sender->receiver bytes, r2s the reverse. xm_send and xm_recv are each
 *   straight-line BLOCKING loops, so they cannot be called one-after-another
 *   in a single thread of control (the sender would fill s2r and block waiting
 *   for an ACK that the not-yet-running receiver never produces). We give them
 *   genuine concurrency with fork(): the rings live in a shared mmap region,
 *   the CHILD runs xm_send, the PARENT runs xm_recv, and they ping-pong real
 *   bytes exactly like two ends of a telnet link. Our ep_get is non-blocking
 *   and returns -1 when its inbound ring is momentarily empty; the protocol
 *   treats -1 as a timeout/retry and simply re-reads, so the two ends self-
 *   synchronise. The child's send return code is passed back through a 1-byte
 *   pipe. All large buffers are heap/shared-memory, never file-scope statics.
 */
/* host-only harness: POSIX fork/mmap/pipe for a real concurrent loopback.
 * glibc hides these prototypes under -std=c89 unless we ask for them. */
#define _GNU_SOURCE 1
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/mman.h>
#include <sys/wait.h>
#include <unistd.h>
#include "xmodem.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-48s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

/* ---- (a) CRC ------------------------------------------------------------ */

static void test_crc(void)
{
    pp_u8 v[9];
    pp_u8 zero[128];
    int   i;

    for (i = 0; i < 9; i++) v[i] = (pp_u8)('1' + i);   /* "123456789" */
    check("crc16(\"123456789\") == 0x31C3", xm_crc16(v, 9) == 0x31C3);

    for (i = 0; i < 128; i++) zero[i] = 0;
    check("crc16(128 zero bytes) == 0x0000", xm_crc16(zero, 128) == 0x0000);
}

/* ---- (b) build/check round-trip ----------------------------------------- */

static void roundtrip(int use_crc, int blocklen)
{
    pp_u8 data[1024];
    pp_u8 out[3 + 1024 + 2];
    pp_u8 got[1024];
    int   n;
    int   bn;
    int   i;
    char  label[80];

    for (i = 0; i < blocklen; i++) data[i] = (pp_u8)((i * 7 + 3) & 0xFF);

    n = xm_build_block(use_crc, 5, data, blocklen, out);

    sprintf(label, "build len %d %s total bytes", blocklen, use_crc ? "crc" : "sum");
    check(label, n == 3 + blocklen + (use_crc ? 2 : 1));

    sprintf(label, "header byte len %d %s", blocklen, use_crc ? "crc" : "sum");
    check(label, out[0] == (blocklen == 1024 ? XM_STX : XM_SOH));

    sprintf(label, "seq/~seq complement len %d %s", blocklen, use_crc ? "crc" : "sum");
    check(label, (pp_u8)(out[1] + out[2]) == 0xFF && out[1] == 5);

    /* body = everything after the header byte */
    bn = -1;
    sprintf(label, "check_block ok len %d %s", blocklen, use_crc ? "crc" : "sum");
    check(label, xm_check_block(use_crc, out + 1, blocklen, &bn, got) == 1);

    sprintf(label, "blocknum recovered len %d %s", blocklen, use_crc ? "crc" : "sum");
    check(label, bn == 5);

    sprintf(label, "data recovered len %d %s", blocklen, use_crc ? "crc" : "sum");
    check(label, memcmp(data, got, (size_t)blocklen) == 0);

    /* corrupt one data byte -> validation must fail */
    out[3 + blocklen / 2] ^= 0xFF;
    sprintf(label, "corruption detected len %d %s", blocklen, use_crc ? "crc" : "sum");
    check(label, xm_check_block(use_crc, out + 1, blocklen, &bn, got) == 0);
}

/* ---- (c) loopback transport over shared-memory rings -------------------- */

#define RING_CAP 65536

typedef struct {
    volatile int head;
    volatile int tail;
    pp_u8        buf[RING_CAP];
} ring;

static int ring_get(ring *r)
{
    int b;
    if (r->head == r->tail) return -1;          /* empty -> "timeout" */
    b = r->buf[r->head];
    r->head = (r->head + 1) % RING_CAP;
    return b;
}
static void ring_put(ring *r, pp_u8 v)
{
    int next = (r->tail + 1) % RING_CAP;
    while (next == r->head) { /* full: wait for peer to drain */ }
    r->buf[r->tail] = v;
    r->tail = next;
}

typedef struct { ring *in; ring *out; } endpoint;

static int  ep_get(void *io, int timeout_ms)
{
    /*
     * Block (spin) until a byte arrives: the protocol expects get() to wait up
     * to timeout_ms, so an instant -1 on an empty ring would burn all retries
     * before the peer process scheduled. With two real processes a spin is the
     * faithful stand-in for a blocking transport read. timeout_ms is ignored.
     */
    ring *r = ((endpoint *)io)->in;
    int   b;
    (void)timeout_ms;
    for (;;) {
        b = ring_get(r);
        if (b >= 0) return b;
    }
}
static void ep_put(void *io, pp_u8 b)
{
    ring_put(((endpoint *)io)->out, b);
}

/* shared memory layout for the two rings */
typedef struct { ring s2r; ring r2s; } shared;

static int run_loopback(const pp_u8 *payload, long len, int allow_1k,
                        pp_u8 *recvbuf, long maxlen, long *outlen, int *send_rc)
{
    shared  *sh;
    int      pfd[2];
    pid_t    pid;
    endpoint snd;
    endpoint rcv;
    int      recv_rc;

    sh = (shared *)mmap((void *)0, sizeof(shared), PROT_READ | PROT_WRITE,
                        MAP_SHARED | MAP_ANON, -1, 0);
    if (sh == (shared *)MAP_FAILED) return -100;
    sh->s2r.head = sh->s2r.tail = 0;
    sh->r2s.head = sh->r2s.tail = 0;

    if (pipe(pfd) != 0) { munmap((void *)sh, sizeof(shared)); return -101; }

    pid = fork();
    if (pid < 0) { munmap((void *)sh, sizeof(shared)); return -102; }

    if (pid == 0) {
        /* CHILD: the SENDER. writes s2r, reads r2s. */
        int rc;
        char rcb;
        close(pfd[0]);
        snd.in = &sh->r2s;
        snd.out = &sh->s2r;
        rc = xm_send(ep_get, ep_put, &snd, payload, len, allow_1k);
        rcb = (char)rc;
        if (write(pfd[1], &rcb, 1) != 1) { /* nothing to do */ }
        close(pfd[1]);
        _exit(0);
    }

    /* PARENT: the RECEIVER. writes r2s, reads s2r. */
    close(pfd[1]);
    rcv.in = &sh->s2r;
    rcv.out = &sh->r2s;
    recv_rc = xm_recv(ep_get, ep_put, &rcv, recvbuf, maxlen, outlen);

    {
        char rcb = 0;
        if (read(pfd[0], &rcb, 1) == 1) *send_rc = (signed char)rcb;
        else *send_rc = -103;
    }
    close(pfd[0]);
    waitpid(pid, (int *)0, 0);
    munmap((void *)sh, sizeof(shared));
    return recv_rc;
}

static void test_loopback(int allow_1k, long len, const char *tag)
{
    pp_u8 *payload;
    pp_u8 *recvbuf;
    long   outlen;
    int    send_rc;
    int    recv_rc;
    long   i;
    int    match;
    char   label[80];

    payload = (pp_u8 *)malloc((size_t)len);
    recvbuf = (pp_u8 *)malloc((size_t)len + 1024);   /* room for ^Z padding */
    for (i = 0; i < len; i++) payload[i] = (pp_u8)((i * 31 + 7) & 0xFF);

    outlen = 0;
    send_rc = -999;
    recv_rc = run_loopback(payload, len, allow_1k, recvbuf, len + 1024,
                           &outlen, &send_rc);

    sprintf(label, "%s recv returns XM_OK", tag);
    check(label, recv_rc == XM_OK);
    sprintf(label, "%s send returns XM_OK", tag);
    check(label, send_rc == XM_OK);

    /* outlen rounds up to whole blocks; the payload prefix must be intact and
     * any tail must be ^Z padding (documented XMODEM behaviour). */
    sprintf(label, "%s outlen covers payload", tag);
    check(label, outlen >= len);

    match = (outlen >= len) && (memcmp(payload, recvbuf, (size_t)len) == 0);
    sprintf(label, "%s payload prefix intact", tag);
    check(label, match);

    {
        int padok = 1;
        for (i = len; i < outlen; i++) if (recvbuf[i] != 0x1A) padok = 0;
        sprintf(label, "%s tail is ^Z padding", tag);
        check(label, padok);
    }

    free(payload);
    free(recvbuf);
}

int main(void)
{
    test_crc();

    roundtrip(0, 128);
    roundtrip(1, 128);
    roundtrip(0, 1024);
    roundtrip(1, 1024);

    /* multi-block payloads, both with and without 1K blocks */
    test_loopback(0, 300, "loopback-128 ");        /* 3 x 128 blocks */
    test_loopback(1, 5000, "loopback-1k  ");       /* 1024-byte STX blocks */
    test_loopback(0, 5000, "loopback-128big");     /* many 128 blocks */
    test_loopback(1, 1, "loopback-tiny ");         /* single short block */

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all xmodem tests passed");
    return g_fail;
}
