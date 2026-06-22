/*
 * trashcan.c -- BAD-WORD / TRASHCAN filter for posts and handles.
 *
 * Loads a banned-word list from a plain text file into static storage and
 * offers a case-insensitive substring test. Deliberately simple: the moderation
 * policy (reject, queue, warn) lives in the caller; here we only answer the
 * question "does this text contain a banned word?".
 */
#include <stdio.h>
#include <string.h>
#include "trashcan.h"

static char g_words[TC_MAX_WORDS][TC_MAX_LEN];
static int  g_n;

static char lc(char c)
{
    /* ASCII lower-case; leaves CP437 high bytes untouched. */
    if (c >= 'A' && c <= 'Z') return (char)(c - 'A' + 'a');
    return c;
}

/* Case-insensitive: is NUL-terminated needle a substring of haystack? */
static int ci_contains(const char *hay, const char *needle)
{
    int i, j;

    if (needle[0] == '\0') return 0;   /* empty word never matches */
    for (i = 0; hay[i] != '\0'; i++) {
        for (j = 0; needle[j] != '\0'; j++) {
            if (hay[i + j] == '\0') return 0;  /* hit end mid-match */
            if (lc(hay[i + j]) != lc(needle[j])) break;
        }
        if (needle[j] == '\0') return 1;       /* matched whole needle */
    }
    return 0;
}

int tc_load(const char *path)
{
    FILE *f;
    char  line[256];

    g_n = 0;
    if (path == (const char *)0) return 0;
    f = fopen(path, "r");
    if (f == (FILE *)0) return 0;   /* missing file is not an error */

    while (g_n < TC_MAX_WORDS && fgets(line, (int)sizeof line, f) != (char *)0) {
        int a = 0, b, len;

        /* trim leading whitespace */
        while (line[a] == ' ' || line[a] == '\t') a++;
        if (line[a] == '#' || line[a] == '\0' ||
            line[a] == '\n' || line[a] == '\r')
            continue;   /* comment or blank */

        /* trim trailing whitespace / newline */
        b = (int)strlen(line);
        while (b > a && (line[b - 1] == ' '  || line[b - 1] == '\t' ||
                         line[b - 1] == '\n' || line[b - 1] == '\r'))
            b--;

        len = b - a;
        if (len <= 0) continue;
        if (len > TC_MAX_LEN - 1) len = TC_MAX_LEN - 1;   /* truncate over-long */
        memcpy(g_words[g_n], line + a, (size_t)len);
        g_words[g_n][len] = '\0';
        g_n++;
    }

    fclose(f);
    return g_n;
}

int tc_blocked(const char *text)
{
    int i;

    if (text == (const char *)0) return 0;
    for (i = 0; i < g_n; i++)
        if (ci_contains(text, g_words[i])) return 1;
    return 0;
}

int tc_count(void)
{
    return g_n;
}
