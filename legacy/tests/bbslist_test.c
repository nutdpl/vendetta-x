/*
 * bbslist_test.c -- host unit tests for the BBS list registry. `make test`.
 */
#include <stdio.h>
#include <string.h>
#include "bbslist.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TMP "/tmp/pigpen_bltest.dat"

int main(void)
{
    /* 1. record size is the documented 160 bytes */
    check("PP_BBS_REC == 160", PP_BBS_REC == 160);

    /* 2. pack/unpack round-trips every field, little-endian on disk */
    {
        pp_bbs a, b;
        pp_u8 rec[PP_BBS_REC];
        memset(&a, 0, sizeof a);
        strcpy(a.name, "Vendetta/X");
        strcpy(a.address, "vendetta-x.org:23");
        strcpy(a.sysop, "Hambone");
        strcpy(a.added_by, "acid burn");
        a.when = 1700000000u;
        bl_pack(&a, rec);
        bl_unpack(rec, &b);
        check("roundtrip name",     strcmp(a.name, b.name) == 0);
        check("roundtrip address",  strcmp(a.address, b.address) == 0);
        check("roundtrip sysop",    strcmp(a.sysop, b.sysop) == 0);
        check("roundtrip added_by", strcmp(a.added_by, b.added_by) == 0);
        check("roundtrip when",     a.when == b.when);
        /* when 1700000000 = 0x6553f100, little-endian at off 152 */
        check("little-endian u32 on disk",
              rec[152]==0x00 && rec[153]==0xf1 && rec[154]==0x53 && rec[155]==0x65);
    }

    /* 3. add / count / get / persist across reopen */
    remove(TMP);
    {
        pp_bbs b, got;
        check("open fresh", bl_open(TMP) == 0 && bl_count() == 0);

        memset(&b, 0, sizeof b);
        strcpy(b.name, "Vendetta/X"); strcpy(b.address, "vendetta-x.org:23");
        strcpy(b.sysop, "Hambone"); strcpy(b.added_by, "acid burn");
        b.when = 1700000000u;
        check("add #1", bl_add(&b) == 0 && bl_count() == 1);

        memset(&b, 0, sizeof b);
        strcpy(b.name, "Mindvox"); strcpy(b.address, "mindvox.example:2323");
        strcpy(b.sysop, "Cereal"); strcpy(b.added_by, "zero cool");
        b.when = 1700000500u;
        check("add #2", bl_add(&b) == 0 && bl_count() == 2);

        /* newest-last: index 0 is the first added */
        check("get #0", bl_get(0, &got) == 0 && strcmp(got.name, "Vendetta/X") == 0);
        check("get #0 addr", strcmp(got.address, "vendetta-x.org:23") == 0);
        check("get #1", bl_get(1, &got) == 0 && strcmp(got.name, "Mindvox") == 0);
        check("get #1 by", strcmp(got.added_by, "zero cool") == 0);
        check("get out-of-range", bl_get(2, &got) != 0);
        bl_close();
    }
    {
        pp_bbs got;
        check("reopen keeps count", bl_open(TMP) == 0 && bl_count() == 2);
        check("reopen keeps data #0",
              bl_get(0, &got) == 0 && strcmp(got.name, "Vendetta/X") == 0 &&
              got.when == 1700000000u);
        check("reopen keeps data #1",
              bl_get(1, &got) == 0 && strcmp(got.sysop, "Cereal") == 0 &&
              got.when == 1700000500u);
        bl_close();
    }
    remove(TMP);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all bbslist tests passed");
    return g_fail;
}
