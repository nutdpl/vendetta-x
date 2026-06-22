/* trashcan_test.c -- host unit tests for the bad-word filter. `make test`. */
#include <stdio.h>
#include "trashcan.h"

static int g_fail;
static void check(const char *n, int c) { printf("%-40s %s\n", n, c ? "ok" : "FAIL"); if (!c) g_fail = 1; }

#define TMP "/tmp/pigpen_tc.txt"

int main(void)
{
    FILE *f = fopen(TMP, "w");
    fputs("# banned words\nspam\nFooBar\n\nnarc\n", f);
    fclose(f);

    check("load count", tc_load(TMP) == 3 && tc_count() == 3);
    check("clean text passes",      tc_blocked("hello from Vendetta/X") == 0);
    check("banned word blocks",     tc_blocked("this is spam") == 1);
    check("case-insensitive",       tc_blocked("FOOBAR rules") == 1);
    check("substring match",        tc_blocked("you narcotics") == 1);
    check("empty text passes",      tc_blocked("") == 0);
    check("missing file -> 0 words", tc_load("/tmp/pigpen_no_such_file") == 0);
    check("no list -> nothing blocked", tc_blocked("spam") == 0);

    remove(TMP);
    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all trashcan tests passed");
    return g_fail;
}
