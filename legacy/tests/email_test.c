/*
 * email_test.c -- host unit tests for private mail persistence. `make test`.
 */
#include <stdio.h>
#include <string.h>
#include "email.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TMP "/tmp/pigpen_emtest.dat"

int main(void)
{
    /* 1. record size is the documented 720 bytes */
    check("PP_MAIL_REC == 720", PP_MAIL_REC == 720);

    /* 2. send two messages to different recipients */
    remove(TMP);
    {
        pp_mail m;
        check("open fresh", em_open(TMP) == 0 && em_count() == 0);

        memset(&m, 0, sizeof m);
        strcpy(m.from, "acid burn"); strcpy(m.to, "Zero Cool");
        strcpy(m.subject, "the gibson"); strcpy(m.body, "hack the planet");
        m.when = 1700000000u; m.flags = 0;
        check("send #1", em_send(&m) == 0 && em_count() == 1);

        memset(&m, 0, sizeof m);
        strcpy(m.from, "cereal"); strcpy(m.to, "phreak");
        strcpy(m.subject, "garbage"); strcpy(m.body, "look in the trash");
        m.when = 1700000001u; m.flags = 0;
        check("send #2", em_send(&m) == 0 && em_count() == 2);

        /* 3. count / count_to case-insensitive */
        check("count == 2", em_count() == 2);
        check("count_to exact", em_count_to("Zero Cool") == 1);
        check("count_to case-insensitive", em_count_to("ZERO COOL") == 1);
        check("count_to lower", em_count_to("zero cool") == 1);
        check("count_to other", em_count_to("phreak") == 1);
        check("count_to miss", em_count_to("nobody") == 0);

        /* 4. get_to fetches the right record + absolute index */
        {
            pp_mail g;
            int idx = -1;
            check("get_to hit", em_get_to("zero cool", 0, &g, &idx) == 0);
            check("get_to subject", strcmp(g.subject, "the gibson") == 0);
            check("get_to from",    strcmp(g.from, "acid burn") == 0);
            check("get_to body",    strcmp(g.body, "hack the planet") == 0);
            check("get_to index 0", idx == 0);
            check("get_to n out of range", em_get_to("zero cool", 1, (pp_mail *)0, (int *)0) == 1);

            /* 5. mark it read via em_update */
            g.flags |= EM_FLAG_READ;
            check("update sets read", em_update(idx, &g) == 0);
        }
        em_close();
    }

    /* 6. reopen: persistence + flags survive */
    {
        pp_mail g;
        int idx = -1;
        check("reopen keeps count", em_open(TMP) == 0 && em_count() == 2);
        check("reopen get_to", em_get_to("ZERO COOL", 0, &g, &idx) == 0 && idx == 0);
        check("reopen read flag set", (g.flags & EM_FLAG_READ) != 0);
        check("reopen body intact", strcmp(g.body, "hack the planet") == 0);

        /* second mailbox still readable + not flagged */
        check("reopen other count_to", em_count_to("phreak") == 1);
        check("reopen other get_to", em_get_to("phreak", 0, &g, (int *)0) == 0);
        check("reopen other not read", (g.flags & EM_FLAG_READ) == 0);

        /* 7. delete flag hides from count_to / get_to */
        check("get index 1", em_get(1, &g) == 0);
        g.flags |= EM_FLAG_DELETED;
        check("update sets deleted", em_update(1, &g) == 0);
        check("deleted hidden from count_to", em_count_to("phreak") == 0);
        check("deleted hidden from get_to", em_get_to("phreak", 0, (pp_mail *)0, (int *)0) == 1);
        check("total count unchanged", em_count() == 2);
        em_close();
    }

    /* 8. little-endian on disk: when of msg #0 = 1700000000 = 0x6553F100 */
    {
        FILE *f = fopen(TMP, "rb");
        pp_u8 rec[PP_MAIL_REC];
        check("reopen raw", f != (FILE *)0);
        if (f) {
            fseek(f, 16L + 112L, SEEK_SET);  /* header + when offset of rec 0 */
            check("read when bytes", fread(rec, 1, 4, f) == 4);
            check("little-endian when",
                  rec[0]==0x00 && rec[1]==0xF1 && rec[2]==0x53 && rec[3]==0x65);
            fclose(f);
        }
    }
    remove(TMP);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all email tests passed");
    return g_fail;
}
