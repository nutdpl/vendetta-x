/*
 * telnet_test.c -- host unit tests for the telnet codec. Build: see `make test`.
 * Exits non-zero on first failure so CI fails loudly.
 */
#include <stdio.h>
#include <string.h>
#include "telnet.h"

static int g_fail;

static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

/* hex compare helper */
static int eq(const pp_u8 *a, const pp_u8 *b, int n) { return memcmp(a, b, (size_t)n) == 0; }

int main(void)
{
    telnet_t t;
    pp_u8 user[256], reply[256];
    int ulen, rlen;

    /* 1. opening greeting: WILL ECHO, WILL SGA, WILL BIN, DO BIN */
    {
        pp_u8 out[16];
        pp_u8 want[] = { TN_IAC,TN_WILL,TN_OPT_ECHO, TN_IAC,TN_WILL,TN_OPT_SGA,
                         TN_IAC,TN_WILL,TN_OPT_BINARY, TN_IAC,TN_DO,TN_OPT_BINARY };
        int n;
        telnet_init(&t);
        n = telnet_greeting(&t, out);
        check("greeting bytes", n == 12 && eq(out, want, 12));
    }

    /* 2. plain data passes through untouched */
    {
        pp_u8 in[] = { 'h','i','!' };
        telnet_init(&t);
        ulen = telnet_recv(&t, in, 3, user, reply, &rlen);
        check("plain data passthrough", ulen == 3 && rlen == 0 && eq(user, in, 3));
    }

    /* 3. IAC command is stripped from the user stream */
    {
        pp_u8 in[] = { 'a', TN_IAC, TN_NOP, 'b' };
        telnet_init(&t);
        ulen = telnet_recv(&t, in, 4, user, reply, &rlen);
        check("IAC NOP stripped", ulen == 2 && user[0]=='a' && user[1]=='b' && rlen==0);
    }

    /* 4. escaped IAC (0xFF 0xFF) yields one literal 0xFF data byte */
    {
        pp_u8 in[] = { 'x', TN_IAC, TN_IAC, 'y' };
        telnet_init(&t);
        ulen = telnet_recv(&t, in, 4, user, reply, &rlen);
        check("escaped 0xFF -> one data byte",
              ulen==3 && user[0]=='x' && user[1]==0xFF && user[2]=='y');
    }

    /* 5. CR LF collapses to a single CR */
    {
        pp_u8 in[] = { 'o','k', '\r','\n' };
        telnet_init(&t);
        ulen = telnet_recv(&t, in, 4, user, reply, &rlen);
        check("CR LF -> CR", ulen==3 && user[0]=='o' && user[1]=='k' && user[2]=='\r');
    }

    /* 6. CR NUL also collapses to a single CR */
    {
        pp_u8 in[] = { 'z', '\r', 0x00 };
        telnet_init(&t);
        ulen = telnet_recv(&t, in, 3, user, reply, &rlen);
        check("CR NUL -> CR", ulen==2 && user[0]=='z' && user[1]=='\r');
    }

    /* 7. DO BINARY (already willed in greeting) -> no reply; DO TTYPE -> WONT */
    {
        pp_u8 in[] = { TN_IAC, TN_DO, TN_OPT_BINARY,  TN_IAC, TN_DO, 24 /*TTYPE*/ };
        pp_u8 wantr[] = { TN_IAC, TN_WONT, 24 };
        telnet_init(&t);
        telnet_greeting(&t, reply);                 /* sets my[BINARY]=1 */
        ulen = telnet_recv(&t, in, 6, user, reply, &rlen);
        check("DO already-on silent, DO unknown -> WONT",
              ulen==0 && rlen==3 && eq(reply, wantr, 3));
    }

    /* 8. client WILL BINARY -> DO BINARY; client WILL ECHO -> DONT ECHO */
    {
        pp_u8 in[] = { TN_IAC, TN_WILL, TN_OPT_BINARY, TN_IAC, TN_WILL, TN_OPT_ECHO };
        pp_u8 wantr[] = { TN_IAC, TN_DO, TN_OPT_BINARY, TN_IAC, TN_DONT, TN_OPT_ECHO };
        telnet_init(&t);
        ulen = telnet_recv(&t, in, 6, user, reply, &rlen);
        check("WILL BIN->DO BIN, WILL ECHO->DONT ECHO",
              ulen==0 && rlen==6 && eq(reply, wantr, 6));
    }

    /* 9. subnegotiation body (IAC SB ... IAC SE) is skipped entirely */
    {
        pp_u8 in[] = { 'p', TN_IAC, TN_SB, 24, 1, 'x','t','e','r','m', TN_IAC, TN_SE, 'q' };
        telnet_init(&t);
        ulen = telnet_recv(&t, in, (int)sizeof in, user, reply, &rlen);
        check("subnegotiation skipped", ulen==2 && user[0]=='p' && user[1]=='q' && rlen==0);
    }

    /* 10. state survives a buffer split mid-IAC-sequence */
    {
        pp_u8 a[] = { 'm', TN_IAC };
        pp_u8 b[] = { TN_DO, TN_OPT_SGA };
        telnet_init(&t);
        ulen = telnet_recv(&t, a, 2, user, reply, &rlen);
        check("split: first half yields just 'm'", ulen==1 && user[0]=='m' && rlen==0);
        ulen = telnet_recv(&t, b, 2, user, reply, &rlen);
        /* SGA not yet willed in this test -> WILL SGA reply */
        check("split: second half completes DO SGA -> WILL SGA",
              ulen==0 && rlen==3 && reply[0]==TN_IAC && reply[1]==TN_WILL && reply[2]==TN_OPT_SGA);
    }

    /* 11. outbound 0xFF (CP437 byte) is doubled on the wire */
    {
        pp_u8 in[] = { 'A', 0xFF, 'B' };
        pp_u8 out[8];
        pp_u8 want[] = { 'A', 0xFF, 0xFF, 'B' };
        int n = telnet_send(in, 3, out);
        check("outbound 0xFF doubled", n==4 && eq(out, want, 4));
    }

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all telnet tests passed");
    return g_fail;
}
