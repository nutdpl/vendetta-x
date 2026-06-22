/*
 * test_syslog.c -- standalone test for core/syslog.c.
 *
 *   cc -std=c89 -pedantic -Wall -Wextra -Icore -Iio \
 *      tests/test_syslog.c core/syslog.c -o /tmp/test_syslog && /tmp/test_syslog
 *
 * Logs 3 events to a temp file, reads it back to confirm 3 timestamped lines,
 * then checks sy_tail returns the last 2.
 */
#include <stdio.h>
#include <string.h>
#include "syslog.h"

#define TMP "/tmp/pigpen_sysop_test.log"

static int g_fail;

static void check(int cond, const char *what)
{
    if (cond) {
        printf("  ok   %s\n", what);
    } else {
        printf("  FAIL %s\n", what);
        g_fail++;
    }
}

/* A valid line looks like "[YYYY-MM-DD HH:MM] ..." */
static int stamped(const char *line)
{
    if (strlen(line) < 19) return 0;
    return line[0] == '[' &&
           line[5] == '-' && line[8] == '-' &&
           line[11] == ' ' && line[14] == ':' &&
           line[17] == ']' && line[18] == ' ';
}

int main(void)
{
    FILE *f;
    char  fileline[256];
    char  buf[1024];
    int   nlines, n2, ev;
    char *p, *nl;

    remove(TMP);

    check(sy_log(TMP, "SYSTEM startup") == 0, "sy_log event 1 returns 0");
    check(sy_log(TMP, "USER zorro logged on (node 1)") == 0, "sy_log event 2 returns 0");
    check(sy_log(TMP, "SYSOP validated user #42") == 0, "sy_log event 3 returns 0");

    /* Read the file back: expect 3 timestamped lines. */
    f = fopen(TMP, "r");
    check(f != NULL, "log file exists");
    nlines = 0;
    if (f != NULL) {
        while (fgets(fileline, sizeof fileline, f) != NULL) {
            nl = strchr(fileline, '\n');
            if (nl != NULL) *nl = '\0';
            printf("    | %s\n", fileline);
            check(stamped(fileline), "line is timestamped");
            nlines++;
        }
        fclose(f);
    }
    check(nlines == 3, "file has exactly 3 lines");

    /* sy_tail should return the last 2 lines. */
    n2 = sy_tail(TMP, buf, (int)sizeof buf, 2);
    check(n2 == 2, "sy_tail returns 2");

    /* Verify the two tailed lines are events 2 and 3, in order. */
    ev = 0;
    p  = buf;
    while (*p) {
        nl = strchr(p, '\n');
        if (nl != NULL) *nl = '\0';
        ev++;
        if (ev == 1) check(strstr(p, "zorro") != NULL, "tail line 1 is event 2");
        if (ev == 2) check(strstr(p, "#42")   != NULL, "tail line 2 is event 3");
        if (nl == NULL) break;
        p = nl + 1;
    }
    check(ev == 2, "tail produced 2 lines");

    /* Edge cases. */
    check(sy_tail(TMP, buf, (int)sizeof buf, 10) == 3, "tail(10) on 3-line log = 3");
    check(sy_log(NULL, "x") != 0, "sy_log(NULL path) fails");
    check(sy_tail("/tmp/pigpen_nonexistent.log", buf, (int)sizeof buf, 2) == 0,
          "tail of missing file = 0");

    remove(TMP);

    if (g_fail == 0) {
        printf("\nALL TESTS PASSED\n");
        return 0;
    }
    printf("\n%d TEST(S) FAILED\n", g_fail);
    return 1;
}
