/*
 * syslog.c -- the SYSOP ACTIVITY LOG / audit trail (data/SYSOP.LOG).
 *
 * A plain text, append-only log. Each sy_log() opens the file in "a" mode,
 * writes one timestamped line, and closes -- so a crash never leaves a half
 * record and concurrent nodes interleave whole lines rather than corrupt.
 */
#include <stdio.h>
#include <string.h>
#include <time.h>
#include "syslog.h"

int sy_log(const char *path, const char *event)
{
    FILE      *f;
    time_t     now;
    struct tm *lt;
    char       stamp[24];

    if (path == (const char *)0 || event == (const char *)0) return 1;

    now = time((time_t *)0);
    lt  = localtime(&now);
    if (lt == (struct tm *)0) return 1;

    /* "[YYYY-MM-DD HH:MM] " -- strftime keeps it C89 and locale-clean. */
    if (strftime(stamp, sizeof stamp, "[%Y-%m-%d %H:%M] ", lt) == 0) return 1;

    f = fopen(path, "a");
    if (f == (FILE *)0) return 1;

    if (fputs(stamp, f) == EOF ||
        fputs(event, f) == EOF ||
        fputc('\n', f) == EOF) {
        fclose(f);
        return 1;
    }
    if (fclose(f) != 0) return 1;
    return 0;
}

int sy_tail(const char *path, char *buf, int max, int lines)
{
    FILE *f;
    long  size, pos, start;
    int   want, found, c;
    int   used;

    if (buf == (char *)0 || max <= 0) return 0;
    buf[0] = '\0';
    if (path == (const char *)0 || lines <= 0) return 0;

    f = fopen(path, "rb");
    if (f == (FILE *)0) return 0;

    if (fseek(f, 0L, SEEK_END) != 0) { fclose(f); return 0; }
    size = ftell(f);
    if (size <= 0) { fclose(f); return 0; }

    /*
     * Walk backwards from EOF counting '\n'. We want the byte just after the
     * (lines)th newline counted from the end -- i.e. the start of the last
     * `lines` lines. A trailing newline at EOF doesn't begin a new line, so we
     * skip counting it.
     */
    want  = lines;
    found = 0;
    start = 0;
    pos   = size;
    while (pos > 0) {
        pos--;
        if (fseek(f, pos, SEEK_SET) != 0) { fclose(f); return 0; }
        c = fgetc(f);
        if (c == '\n') {
            if (pos == size - 1) continue;   /* trailing newline at EOF */
            found++;
            if (found == want) { start = pos + 1; break; }
        }
    }

    /* Copy from `start` to EOF into buf, truncating to max-1 bytes. */
    if (fseek(f, start, SEEK_SET) != 0) { fclose(f); return 0; }
    used = 0;
    while (used < max - 1) {
        c = fgetc(f);
        if (c == EOF) break;
        buf[used++] = (char)c;
    }
    buf[used] = '\0';
    fclose(f);

    /* Count the lines we actually delivered (non-empty trailing lines). */
    {
        int  i, n;
        n = 0;
        for (i = 0; i < used; i++)
            if (buf[i] == '\n') n++;
        if (used > 0 && buf[used - 1] != '\n') n++;   /* unterminated last line */
        return n;
    }
}
