/*
 * userbase_test.c -- host unit tests for accounts persistence. `make test`.
 */
#include <stdio.h>
#include <string.h>
#include "userbase.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TMP "/tmp/pigpen_ubtest.dat"

int main(void)
{
    /* 1. record size */
    check("PP_USER_REC == 384", PP_USER_REC == 384);

    /* 2. pack/unpack round-trips every field */
    {
        pp_user a, b;
        pp_u8 rec[PP_USER_REC];
        memset(&a, 0, sizeof a);
        strcpy(a.handle, "Lord Nikon");
        strcpy(a.location, "Hackers, NYC");
        strcpy(a.tagline, "mess with the best, die like the rest");
        strcpy(a.group, "FLT");
        a.sl = 200; a.dsl = 150; a.flags = 3; a.prot = 1;
        a.ar = 0x1234; a.dar = 0x00ff; a.restr = 0x0080;
        a.times_called = 70000; a.first_call = 1000000000u;
        a.last_call = 1700000000u; a.posts = 42;
        strcpy(a.real_name, "Paul Cook");
        strcpy(a.email, "nikon@hackers.nyc");
        strcpy(a.birthdate, "1976-05-18");
        strcpy(a.city, "New York, NY");
        strcpy(a.zip, "10001");
        strcpy(a.computer, "486DX2/66");
        strcpy(a.pwhash, "deadbeef");
        ub_pack(&a, rec);
        ub_unpack(rec, &b);
        check("roundtrip handle",   strcmp(a.handle, b.handle) == 0);
        check("roundtrip location", strcmp(a.location, b.location) == 0);
        check("roundtrip tagline",  strcmp(a.tagline, b.tagline) == 0);
        check("roundtrip group",    strcmp(a.group, b.group) == 0);
        check("roundtrip sl/dsl/flags", a.sl == b.sl && a.dsl == b.dsl && a.flags == b.flags);
        check("roundtrip prot",     a.prot == b.prot);
        check("roundtrip real_name", strcmp(a.real_name, b.real_name) == 0);
        check("roundtrip email",     strcmp(a.email, b.email) == 0);
        check("roundtrip birthdate", strcmp(a.birthdate, b.birthdate) == 0);
        check("roundtrip city",      strcmp(a.city, b.city) == 0);
        check("roundtrip zip",       strcmp(a.zip, b.zip) == 0);
        check("roundtrip computer",  strcmp(a.computer, b.computer) == 0);
        check("roundtrip pwhash",    strcmp(a.pwhash, b.pwhash) == 0);
        check("roundtrip ar/dar/restr", a.ar == b.ar && a.dar == b.dar && a.restr == b.restr);
        check("roundtrip u32 fields",
              a.times_called == b.times_called && a.first_call == b.first_call &&
              a.last_call == b.last_call && a.posts == b.posts);
        /* serialization is little-endian: times_called 70000 = 0x00011170 */
        check("little-endian u32 on disk",
              rec[60]==0x70 && rec[61]==0x11 && rec[62]==0x01 && rec[63]==0x00);
    }

    /* 3. add / find / persist across reopen */
    remove(TMP);
    {
        pp_user u, got;
        pp_u32 idx;
        check("open fresh", ub_open(TMP) == 0 && ub_count() == 0);

        memset(&u, 0, sizeof u);
        strcpy(u.handle, "acid burn"); strcpy(u.location, "the gibson");
        u.times_called = 1;
        check("add #1", ub_add(&u, &idx) == 0 && idx == 0 && ub_count() == 1);

        memset(&u, 0, sizeof u);
        strcpy(u.handle, "cereal"); strcpy(u.location, "a payphone");
        u.times_called = 1;
        check("add #2", ub_add(&u, &idx) == 0 && idx == 1 && ub_count() == 2);

        check("find case-insensitive", ub_find("ACID BURN", &got, &idx) == 1 && idx == 0);
        check("found right record", strcmp(got.location, "the gibson") == 0);
        check("miss returns 0", ub_find("zero cool", (pp_user *)0, (pp_u32 *)0) == 0);

        /* bump a counter and persist */
        got.times_called = 5;
        check("update", ub_update(0, &got) == 0);
        ub_close();
    }
    {
        pp_user got;
        check("reopen keeps count", ub_open(TMP) == 0 && ub_count() == 2);
        check("reopen keeps data", ub_find("acid burn", &got, (pp_u32 *)0) == 1 &&
                                   got.times_called == 5);
        ub_close();
    }
    remove(TMP);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all userbase tests passed");
    return g_fail;
}
