/*
 * filebase_test.c -- host unit tests for the file catalog. `make test`.
 *
 * fb_open uses a fixed "data/<tag>.FIL" path, so we chdir into a scratch
 * directory that contains a data/ subdir before exercising the module.
 */
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/stat.h>
#include "filebase.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TAG "TESTAREA"
#define SCRATCH "/tmp/pigpen_fbtest"

int main(void)
{
    /* 1. record size is the documented 128 bytes */
    check("FB_REC == 128", FB_REC == 128);

    /* prepare a scratch working dir with a data/ subdir, then chdir in */
    system("rm -rf " SCRATCH);
    if (mkdir(SCRATCH, 0777) != 0 ||
        mkdir(SCRATCH "/data", 0777) != 0 ||
        chdir(SCRATCH) != 0) {
        printf("could not set up scratch dir\n");
        return 1;
    }
    remove("data/" TAG ".FIL");

    /* 2. open fresh, add two files, count/get */
    {
        pp_file a, b, got;

        check("open fresh", fb_open(TAG) == 0 && fb_count() == 0);

        memset(&a, 0, sizeof a);
        strcpy(a.name, "README.TXT");
        strcpy(a.desc, "the obligatory readme");
        a.size = 4096u;
        strcpy(a.uploader, "Zero Cool");
        a.when = 1700000000u;
        a.downloads = 0;
        a.flags = 1;
        check("add #1", fb_add(&a) == 0 && fb_count() == 1);

        memset(&b, 0, sizeof b);
        strcpy(b.name, "Vendetta/X.ZIP");
        strcpy(b.desc, "the whole board in a box");
        b.size = 1234567u;
        strcpy(b.uploader, "Acid Burn");
        b.when = 1700000001u;
        b.downloads = 7;
        b.flags = 0;
        check("add #2", fb_add(&b) == 0 && fb_count() == 2);

        check("get #0 roundtrip",
              fb_get(0, &got) == 0 &&
              strcmp(got.name, "README.TXT") == 0 &&
              strcmp(got.desc, "the obligatory readme") == 0 &&
              got.size == 4096u &&
              strcmp(got.uploader, "Zero Cool") == 0 &&
              got.when == 1700000000u &&
              got.downloads == 0 &&
              got.flags == 1);

        check("get #1 roundtrip",
              fb_get(1, &got) == 0 &&
              strcmp(got.name, "Vendetta/X.ZIP") == 0 &&
              got.size == 1234567u &&
              got.downloads == 7);

        check("get out of range fails", fb_get(2, &got) == 1);

        /* 3. inc_downloads bumps the counter */
        check("inc_downloads #1", fb_inc_downloads(1) == 0);
        check("inc_downloads #1 visible",
              fb_get(1, &got) == 0 && got.downloads == 8);
        check("inc_downloads out of range fails", fb_inc_downloads(2) == 1);

        fb_close();
    }

    /* 4. reopen persists everything */
    {
        pp_file got;
        check("reopen keeps count", fb_open(TAG) == 0 && fb_count() == 2);
        check("reopen keeps file #0",
              fb_get(0, &got) == 0 && strcmp(got.name, "README.TXT") == 0 &&
              got.size == 4096u && got.downloads == 0);
        check("reopen keeps inc'd downloads",
              fb_get(1, &got) == 0 && got.downloads == 8 &&
              strcmp(got.uploader, "Acid Burn") == 0);
        fb_close();
    }

    /* little-endian on disk: read raw bytes of size field for record 0.
       header(16) + name(16) + desc(64) = offset 96; size 4096 = 0x00001000 */
    {
        FILE *f = fopen("data/" TAG ".FIL", "rb");
        unsigned char raw[4];
        int ok = 0;
        if (f) {
            if (fseek(f, 16L + 16L + 64L, SEEK_SET) == 0 &&
                fread(raw, 1, 4, f) == 4) {
                ok = (raw[0] == 0x00 && raw[1] == 0x10 &&
                      raw[2] == 0x00 && raw[3] == 0x00);
            }
            fclose(f);
        }
        check("little-endian u32 size on disk", ok);
    }

    remove("data/" TAG ".FIL");
    chdir("/");
    system("rm -rf " SCRATCH);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all filebase tests passed");
    return g_fail;
}
