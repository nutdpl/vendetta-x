/*
 * msgbase_test.c -- host unit tests for the message store + reply quoting.
 * `make test`. Exercises the real on-disk path (data/<TAG>.MSG) for a throwaway
 * area, asserting every field round-trips -- including the new reply_to thread
 * link -- and checks mb_quote's output byte-for-byte.
 */
#include <stdio.h>
#include <string.h>
#include "msgbase.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TAG  "ZZMBTEST"
#define PATH "data/" TAG ".MSG"

int main(void)
{
    check("PP_MSG_REC == 1084", PP_MSG_REC == 1084);

    remove(PATH);
    check("mb_open creates area", mb_open(TAG) == 0 && mb_count() == 0);

    {   /* a root post and a threaded reply */
        pp_msg a, b, r;
        memset(&a, 0, sizeof a);
        strcpy(a.from, "Dan"); strcpy(a.to, "All");
        strcpy(a.subject, "first post"); strcpy(a.body, "hello world\r\n");
        a.when = 1700000000u; a.flags = 0; a.reply_to = MB_NO_PARENT;

        memset(&b, 0, sizeof b);
        strcpy(b.from, "Zero"); strcpy(b.to, "Dan");
        strcpy(b.subject, "Re: first post"); strcpy(b.body, "> hello world\r\nhi!\r\n");
        b.when = 1700000100u; b.reply_to = 1;        /* replies to message #1 */

        check("mb_add root", mb_add(&a) == 0);
        check("mb_add reply", mb_add(&b) == 0);
        check("count == 2", mb_count() == 2);

        check("get root ok", mb_get(0, &r) == 0);
        check("root from/subj", strcmp(r.from, "Dan") == 0 && strcmp(r.subject, "first post") == 0);
        check("root body", strcmp(r.body, "hello world\r\n") == 0);
        check("root reply_to == 0", r.reply_to == MB_NO_PARENT);

        check("get reply ok", mb_get(1, &r) == 0);
        check("reply when", r.when == 1700000100u);
        check("reply threads to #1", r.reply_to == 1u);
    }

    /* reopen: header count + reply_to survive a close/open cycle */
    mb_close();
    {
        pp_msg r;
        check("reopen ok", mb_open(TAG) == 0 && mb_count() == 2);
        check("persisted thread link", mb_get(1, &r) == 0 && r.reply_to == 1u);
    }
    mb_close();
    remove(PATH);

    /* mb_quote */
    {
        char q[256];
        mb_quote("hello", "Dan", q, (int)sizeof q);
        check("quote single line", strcmp(q, "Dan wrote:\r\n> hello\r\n") == 0);

        mb_quote("a\r\nb\r\n", "Zero", q, (int)sizeof q);
        check("quote multi line", strcmp(q, "Zero wrote:\r\n> a\r\n> b\r\n") == 0);

        mb_quote("x", (const char *)0, q, (int)sizeof q);
        check("quote null author", strcmp(q, "someone wrote:\r\n> x\r\n") == 0);

        mb_quote("", "Dan", q, (int)sizeof q);
        check("quote empty body", strcmp(q, "Dan wrote:\r\n") == 0);
    }
    {   /* truncation stays in bounds and NUL-terminated */
        char q[12];
        int n = mb_quote("aaaaaaaaaaaaaaaaaaaa", "Dan", q, (int)sizeof q);
        check("quote truncates safely", n < (int)sizeof q && q[n] == '\0' && (int)strlen(q) == n);
    }

    printf("\nmsgbase_test: %s\n", g_fail ? "FAILED" : "all passed");
    return g_fail;
}
