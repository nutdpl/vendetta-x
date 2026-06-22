#include <stdio.h>
#include "qscan.h"

#define CHECK(c) do { if (!(c)) { printf("FAIL line %d: %s\n", __LINE__, #c); fails++; } } while (0)

int main(void)
{
    int fails = 0;
    const char *path = "data/QSCAN_TEST.DAT";

    remove(path);

    /* fresh open of nonexistent file */
    qs_open(path);
    CHECK(qs_count() == 0);

    /* set (user 5, "gen", 12) */
    qs_set(5, "gen", 12);
    CHECK(qs_count() == 1);
    CHECK(qs_get(5, "gen") == 12);

    /* unknown -> 0 */
    CHECK(qs_get(5, "other") == 0);
    CHECK(qs_get(6, "gen") == 0);

    /* update same key to 20 -- must NOT create a 2nd record */
    qs_set(5, "gen", 20);
    CHECK(qs_count() == 1);
    CHECK(qs_get(5, "gen") == 20);

    /* a distinct key does append */
    qs_set(6, "gen", 7);
    CHECK(qs_count() == 2);
    CHECK(qs_get(6, "gen") == 7);

    qs_close();

    /* reopen persists */
    qs_open(path);
    CHECK(qs_count() == 2);
    CHECK(qs_get(5, "gen") == 20);
    CHECK(qs_get(6, "gen") == 7);
    qs_close();

    remove(path);

    if (fails == 0) printf("ALL PASS\n");
    else printf("%d FAILURES\n", fails);
    return fails ? 1 : 0;
}
