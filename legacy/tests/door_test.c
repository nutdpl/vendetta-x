/*
 * door_test.c -- host unit tests for the DOOR registry + drop-file writers.
 */
#include <stdio.h>
#include <string.h>
#include "door.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TMP_DAT  "/tmp/pigpen_doortest.dat"
#define TMP_SYS  "/tmp/pigpen_door.sys"
#define TMP_INF  "/tmp/pigpen_dorinfo1.def"

/* read a whole file into buf (NUL-terminated); return bytes read, -1 on error */
static long slurp(const char *path, char *buf, long max)
{
    FILE *f;
    long n;
    f = fopen(path, "rb");
    if (f == (FILE *)0) return -1;
    n = (long)fread(buf, 1, (size_t)(max - 1), f);
    fclose(f);
    if (n < 0) return -1;
    buf[n] = '\0';
    return n;
}

/* count complete CRLF-terminated lines in buf */
static int count_lines(const char *buf)
{
    int n = 0;
    const char *p = buf;
    while (*p) {
        if (p[0] == '\r' && p[1] == '\n') n++;
        p++;
    }
    return n;
}

/* copy the 1-based CRLF-terminated line `want` into out (without CRLF) */
static void get_line(const char *buf, int want, char *out, int max)
{
    int line = 1;
    int i = 0;
    const char *p = buf;
    out[0] = '\0';
    while (*p) {
        if (line == want) {
            while (p[0] && !(p[0] == '\r' && p[1] == '\n') && i < max - 1) {
                out[i++] = *p++;
            }
            out[i] = '\0';
            return;
        }
        if (p[0] == '\r' && p[1] == '\n') { line++; p += 2; }
        else p++;
    }
}

int main(void)
{
    char buf[8192];
    char line[256];

    remove(TMP_DAT);
    remove(TMP_SYS);
    remove(TMP_INF);

    /* 1. record size is the documented 144 bytes */
    check("PP_DOOR_REC == 144", PP_DOOR_REC == 144);

    /* 2. pack/unpack round-trips every field, byte offsets honored */
    {
        pp_door a, b;
        pp_u8 rec[PP_DOOR_REC];
        memset(&a, 0, sizeof a);
        strcpy(a.name, "LORD");
        strcpy(a.cmd, "C:\\DOORS\\LORD\\START.BAT %1");
        a.type = DROP_DOORSYS;
        strcpy(a.acs, "-");
        dr_pack(&a, rec);
        dr_unpack(rec, &b);
        check("roundtrip name", strcmp(a.name, b.name) == 0);
        check("roundtrip cmd",  strcmp(a.cmd, b.cmd) == 0);
        check("roundtrip type", a.type == b.type);
        check("roundtrip acs",  strcmp(a.acs, b.acs) == 0);
        /* byte offsets: name @0, cmd @32, type @96, acs @97 */
        check("name @ off 0",  memcmp(rec + 0, "LORD", 4) == 0 && rec[4] == 0);
        check("cmd @ off 32",  memcmp(rec + 32, "C:\\DOORS", 8) == 0);
        check("type @ off 96", rec[96] == DROP_DOORSYS);
        check("acs @ off 97",  rec[97] == '-' && rec[98] == 0);
        /* pad region 129..143 deterministically zeroed */
        {
            int i, z = 1;
            for (i = 129; i < PP_DOOR_REC; i++) if (rec[i] != 0) z = 0;
            check("pad 129..143 zero", z);
        }
    }

    /* 3. registry: open fresh, add, count, get, persist across reopen */
    {
        pp_door d, got;
        check("open fresh", dr_open(TMP_DAT) == 0 && dr_count() == 0);

        memset(&d, 0, sizeof d);
        strcpy(d.name, "LORD");
        strcpy(d.cmd, "C:\\DOORS\\LORD\\START.BAT %1");
        d.type = DROP_DOORSYS;
        strcpy(d.acs, "-");
        check("add #1", dr_add(&d) == 0 && dr_count() == 1);

        memset(&d, 0, sizeof d);
        strcpy(d.name, "TradeWars");
        strcpy(d.cmd, "C:\\DOORS\\TW2002\\TW.EXE");
        d.type = DROP_DORINFO;
        strcpy(d.acs, "s20");
        check("add #2", dr_add(&d) == 0 && dr_count() == 2);

        check("get #0", dr_get(0, &got) == 0 && strcmp(got.name, "LORD") == 0);
        check("get #0 cmd", strcmp(got.cmd, "C:\\DOORS\\LORD\\START.BAT %1") == 0);
        check("get #0 type", got.type == DROP_DOORSYS);
        check("get #1", dr_get(1, &got) == 0 && strcmp(got.name, "TradeWars") == 0);
        check("get #1 type", got.type == DROP_DORINFO);
        check("get #1 acs", strcmp(got.acs, "s20") == 0);
        check("get out-of-range", dr_get(2, &got) != 0);
        dr_close();
    }
    {
        pp_door got;
        check("reopen keeps count", dr_open(TMP_DAT) == 0 && dr_count() == 2);
        check("reopen keeps data #0",
              dr_get(0, &got) == 0 && strcmp(got.name, "LORD") == 0);
        check("reopen keeps data #1",
              dr_get(1, &got) == 0 && strcmp(got.cmd, "C:\\DOORS\\TW2002\\TW.EXE") == 0 &&
              got.type == DROP_DORINFO);
        dr_close();
    }

    /* 4. DOOR.SYS writer: exactly 52 lines, fields on correct lines */
    {
        pp_door_session s;
        long n;
        memset(&s, 0, sizeof s);
        s.handle = "acid burn";
        s.location = "New York, NY";
        s.node = 3;
        s.sl = 50;
        s.dsl = 40;
        s.seconds_left = 3600;     /* 60 minutes */
        s.ansi = 1;
        s.baud = 38400;
        s.times_on = 42;

        check("door_write_doorsys ok", door_write_doorsys(TMP_SYS, &s) == 0);
        n = slurp(TMP_SYS, buf, (long)sizeof buf);
        check("doorsys readable", n > 0);
        check("doorsys 52 lines", count_lines(buf) == 52);

        get_line(buf, 1, line, (int)sizeof line);
        check("doorsys L1 com port 0", strcmp(line, "0") == 0);
        get_line(buf, 2, line, (int)sizeof line);
        check("doorsys L2 baud", strcmp(line, "38400") == 0);
        get_line(buf, 4, line, (int)sizeof line);
        check("doorsys L4 node", strcmp(line, "3") == 0);
        get_line(buf, 10, line, (int)sizeof line);
        check("doorsys L10 handle", strcmp(line, "acid burn") == 0);
        get_line(buf, 11, line, (int)sizeof line);
        check("doorsys L11 location", strcmp(line, "New York, NY") == 0);
        get_line(buf, 15, line, (int)sizeof line);
        check("doorsys L15 security", strcmp(line, "50") == 0);
        get_line(buf, 16, line, (int)sizeof line);
        check("doorsys L16 times on", strcmp(line, "42") == 0);
        get_line(buf, 18, line, (int)sizeof line);
        check("doorsys L18 seconds left", strcmp(line, "3600") == 0);
        get_line(buf, 19, line, (int)sizeof line);
        check("doorsys L19 minutes left", strcmp(line, "60") == 0);
        get_line(buf, 20, line, (int)sizeof line);
        check("doorsys L20 graphics GR", strcmp(line, "GR") == 0);
    }

    /* 5. DOOR.SYS NG mode when no ANSI */
    {
        pp_door_session s;
        memset(&s, 0, sizeof s);
        s.handle = "zero cool";
        s.baud = 2400;
        s.ansi = 0;
        check("door_write_doorsys ng ok", door_write_doorsys(TMP_SYS, &s) == 0);
        slurp(TMP_SYS, buf, (long)sizeof buf);
        check("doorsys still 52 lines", count_lines(buf) == 52);
        get_line(buf, 20, line, (int)sizeof line);
        check("doorsys L20 graphics NG", strcmp(line, "NG") == 0);
    }

    /* 6. DORINFO1.DEF writer: 13 lines, name split, fields on right lines */
    {
        pp_door_session s;
        long n;
        memset(&s, 0, sizeof s);
        s.handle = "acid burn";
        s.location = "New York, NY";
        s.node = 3;
        s.sl = 50;
        s.seconds_left = 1800;     /* 30 minutes */
        s.ansi = 1;
        s.baud = 38400;
        s.times_on = 42;

        check("door_write_dorinfo ok", door_write_dorinfo(TMP_INF, &s) == 0);
        n = slurp(TMP_INF, buf, (long)sizeof buf);
        check("dorinfo readable", n > 0);
        check("dorinfo 13 lines", count_lines(buf) == 13);

        get_line(buf, 1, line, (int)sizeof line);
        check("dorinfo L1 system", strcmp(line, "Vendetta/X") == 0);
        get_line(buf, 4, line, (int)sizeof line);
        check("dorinfo L4 com port 0", strcmp(line, "0") == 0);
        get_line(buf, 5, line, (int)sizeof line);
        check("dorinfo L5 baud line", strcmp(line, "38400 BAUD,N,8,1") == 0);
        get_line(buf, 7, line, (int)sizeof line);
        check("dorinfo L7 first name", strcmp(line, "acid") == 0);
        get_line(buf, 8, line, (int)sizeof line);
        check("dorinfo L8 last name", strcmp(line, "burn") == 0);
        get_line(buf, 9, line, (int)sizeof line);
        check("dorinfo L9 location", strcmp(line, "New York, NY") == 0);
        get_line(buf, 10, line, (int)sizeof line);
        check("dorinfo L10 ansi 1", strcmp(line, "1") == 0);
        get_line(buf, 11, line, (int)sizeof line);
        check("dorinfo L11 security", strcmp(line, "50") == 0);
        get_line(buf, 12, line, (int)sizeof line);
        check("dorinfo L12 minutes", strcmp(line, "30") == 0);
        get_line(buf, 13, line, (int)sizeof line);
        check("dorinfo L13 fossil", strcmp(line, "-1") == 0);
    }

    /* 7. DORINFO name with no space -> all in first, last empty */
    {
        pp_door_session s;
        memset(&s, 0, sizeof s);
        s.handle = "Hambone";
        s.baud = 9600;
        s.ansi = 0;
        check("door_write_dorinfo nospace ok", door_write_dorinfo(TMP_INF, &s) == 0);
        slurp(TMP_INF, buf, (long)sizeof buf);
        check("dorinfo nospace 13 lines", count_lines(buf) == 13);
        get_line(buf, 7, line, (int)sizeof line);
        check("dorinfo L7 whole handle", strcmp(line, "Hambone") == 0);
        get_line(buf, 8, line, (int)sizeof line);
        check("dorinfo L8 empty last", strcmp(line, "") == 0);
        get_line(buf, 10, line, (int)sizeof line);
        check("dorinfo L10 ansi 0", strcmp(line, "0") == 0);
    }

    remove(TMP_DAT);
    remove(TMP_SYS);
    remove(TMP_INF);

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all door tests passed");
    return g_fail;
}
