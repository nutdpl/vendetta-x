/*
 * voting_test.c -- host unit tests for the voting booth. `make test`.
 */
#include <stdio.h>
#include <string.h>
#include "voting.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TMP "/tmp/pigpen_vttest.dat"

int main(void)
{
    remove(TMP);
    {
        pp_poll p, got;
        check("open fresh", vt_open(TMP) == 0 && vt_count() == 0);

        memset(&p, 0, sizeof p);
        strcpy(p.question, "Best phreak tool?");
        p.noptions = 3;
        strcpy(p.options[0], "Blue box");
        strcpy(p.options[1], "Red box");
        strcpy(p.options[2], "Beige box");
        p.when = 1700000000u;
        check("add 3-option poll", vt_add(&p) == 0 && vt_count() == 1);

        /* user 5 votes option 1, user 200 votes option 1 */
        check("cast user 5 -> opt 1",   vt_cast(0, 1, 5) == 0);
        check("cast user 200 -> opt 1", vt_cast(0, 1, 200) == 0);
        check("cast user 7 -> opt 0",   vt_cast(0, 0, 7) == 0);

        check("counts after votes",
              vt_get(0, &got) == 0 &&
              got.counts[0] == 1 && got.counts[1] == 2 && got.counts[2] == 0);

        check("has_voted user 5",   vt_has_voted(0, 5) == 1);
        check("has_voted user 200", vt_has_voted(0, 200) == 1);
        check("has_voted user 7",   vt_has_voted(0, 7) == 1);
        check("not voted user 9",   vt_has_voted(0, 9) == 0);

        /* double vote rejected, counts unchanged */
        check("double vote rejected", vt_cast(0, 2, 5) == -1);
        check("counts unchanged after double",
              vt_get(0, &got) == 0 && got.counts[1] == 2 && got.counts[2] == 0);

        /* bad args */
        check("bad option rejected",   vt_cast(0, 3, 30) == -1);
        check("bad poll rejected",     vt_cast(9, 0, 30) == -1);
        check("bad user idx rejected", vt_cast(0, 0, 256) == -1);
        check("get bad index",         vt_get(9, &got) == -1);

        vt_close();
    }

    /* reopen persists */
    {
        pp_poll got;
        check("reopen keeps count", vt_open(TMP) == 0 && vt_count() == 1);
        check("reopen keeps data",
              vt_get(0, &got) == 0 &&
              strcmp(got.question, "Best phreak tool?") == 0 &&
              got.noptions == 3 &&
              strcmp(got.options[2], "Beige box") == 0 &&
              got.counts[0] == 1 && got.counts[1] == 2 &&
              got.when == 1700000000u);
        check("reopen keeps voted bits",
              vt_has_voted(0, 5) == 1 && vt_has_voted(0, 200) == 1 &&
              vt_has_voted(0, 9) == 0);
        check("reopen double vote still rejected", vt_cast(0, 0, 5) == -1);
        vt_close();
    }
    remove(TMP);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all voting tests passed");
    return g_fail;
}
