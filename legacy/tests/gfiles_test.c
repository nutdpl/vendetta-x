/* gfiles_test.c -- host unit tests for the bulletins catalog. `make test`. */
#include <stdio.h>
#include <string.h>
#include "gfiles.h"

static int g_fail;
static void check(const char *n, int c) { printf("%-40s %s\n", n, c ? "ok" : "FAIL"); if (!c) g_fail = 1; }

#define TMP "/tmp/pigpen_gf.dat"

int main(void)
{
    pp_gfile a, b;
    remove(TMP);
    check("open fresh", gf_open(TMP) == 0 && gf_count() == 0);
    memset(&a, 0, sizeof a);
    strcpy(a.title, "the rules"); strcpy(a.file, "gfiles/rules.ans"); strcpy(a.acs, "-");
    check("add #1", gf_add(&a) == 0 && gf_count() == 1);
    memset(&a, 0, sizeof a);
    strcpy(a.title, "sysop notes"); strcpy(a.file, "gfiles/notes.ans"); strcpy(a.acs, "SYSOP");
    check("add #2", gf_add(&a) == 0 && gf_count() == 2);
    check("get #0", gf_get(0, &b) == 0 && strcmp(b.title, "the rules") == 0 && strcmp(b.acs, "-") == 0);
    check("get #1 acs", gf_get(1, &b) == 0 && strcmp(b.acs, "SYSOP") == 0);
    gf_close();
    check("reopen persists", gf_open(TMP) == 0 && gf_count() == 2);
    check("reopen data", gf_get(1, &b) == 0 && strcmp(b.file, "gfiles/notes.ans") == 0);
    gf_close();
    remove(TMP);
    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all gfiles tests passed");
    return g_fail;
}
