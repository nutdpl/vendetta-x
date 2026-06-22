/*
 * acs_test.c -- host unit tests for the ACS evaluator. `make test`.
 */
#include <stdio.h>
#include <string.h>
#include "acs.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-46s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

int main(void)
{
    pp_user u, s;
    memset(&u, 0, sizeof u);
    u.sl = 50; u.dsl = 30;
    u.ar = 0x0007;     /* A, B, C */
    u.dar = 0x0001;    /* A */
    u.restr = 0x0004;  /* C */

    check("empty -> true",            acs_eval("", &u) == 1);
    check("NULL -> true",             acs_eval((const char *)0, &u) == 1);
    check("\"-\" -> true",            acs_eval("-", &u) == 1);
    check("ANY -> true",              acs_eval("ANY", &u) == 1);

    check("SL>=20 (sl 50) true",      acs_eval("SL>=20", &u) == 1);
    check("SL>=80 false",             acs_eval("SL>=80", &u) == 0);
    check("SL>50 false",              acs_eval("SL>50", &u) == 0);
    check("SL<100 true",              acs_eval("SL<100", &u) == 1);
    check("SL=50 true",               acs_eval("SL=50", &u) == 1);
    check("DSL>=30 true",             acs_eval("DSL>=30", &u) == 1);
    check("DSL>=40 false",            acs_eval("DSL>=40", &u) == 0);

    check("AR:A true",                acs_eval("AR:A", &u) == 1);
    check("AR:C true",                acs_eval("AR:C", &u) == 1);
    check("AR:D false",               acs_eval("AR:D", &u) == 0);
    check("DAR:A true",               acs_eval("DAR:A", &u) == 1);
    check("DAR:B false",              acs_eval("DAR:B", &u) == 0);
    check("R:C true (restricted)",    acs_eval("R:C", &u) == 1);
    check("!R:C false",               acs_eval("!R:C", &u) == 0);
    check("!AR:D true",               acs_eval("!AR:D", &u) == 1);

    check("AND both true",            acs_eval("SL>=20&AR:B", &u) == 1);
    check("AND one false",            acs_eval("SL>=20&AR:D", &u) == 0);
    check("OR one true",              acs_eval("SL>=80|AR:A", &u) == 1);
    check("OR both false",            acs_eval("SL>=80|AR:D", &u) == 0);
    check("parens + precedence",      acs_eval("(SL>=80|AR:A)&DSL>=30", &u) == 1);
    check("not over and",             acs_eval("!(SL>=80&AR:A)", &u) == 1);
    check("spaces ignored",           acs_eval("SL>=20 & AR:B", &u) == 1);
    check("SYSOP false (sl 50)",      acs_eval("SYSOP", &u) == 0);

    memset(&s, 0, sizeof s);
    s.sl = 255;
    check("SYSOP true (sl 255)",      acs_eval("SYSOP", &s) == 1);
    check("sysop satisfies SL>=100",  acs_eval("SL>=100", &s) == 1);
    check("sysop fails AR:A",         acs_eval("AR:A", &s) == 0);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all acs tests passed");
    return g_fail;
}
