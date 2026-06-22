/*
 * strtab.c -- compiled defaults + VENDX.STR override loader.
 *
 * File format (keyed text, sysop-editable, external-tool friendly):
 *   # comment
 *   KEY=value with \r \n \t \e \\ escapes
 * Unknown keys and malformed lines are skipped, not fatal -- the board boots
 * even with a damaged file. Keys are the names below without the PPS_ prefix.
 */
#include <stdio.h>
#include <string.h>
#include "strtab.h"

static const char *DEFAULTS[PPS_COUNT] = {
    /* PPS_HANDLE_PROMPT  */ "|07\r\n  |15handle|08 (or 'new')|08: |07",
    /* PPS_HANDLE_NOTFOUND*/ "|12\r\n  handle not found.|07 sign up as a new user?  ",
    /* PPS_NEWHANDLE_PROMPT*/"|07\r\n  |15pick a handle|08: |07",
    /* PPS_HANDLE_TAKEN   */ "|12  that handle is taken -- try another.|07",
    /* PPS_LOCATION_PROMPT*/ "|07\r\n  |15where are you calling from|08? |07",
    /* PPS_PRESS_KEY      */ "|08\r\n  press a key for the main menu...|07",
    /* PPS_MISSING_ART    */ "|12\r\n  [art missing]|07\r\n",
    /* PPS_LASTCALL_TITLE */ "recent callers",
    /* PPS_USERLIST_TITLE */ "Vendetta/X user list",
    /* PPS_ONELINER_TITLE */ "the oneliner wall",
    /* PPS_ONELINER_PROMPT*/ "|07\r\n  |15drop a line|08 (enter to skip)|08: |07",
    /* PPS_MSG_TO         */ "|07  to   |08 (enter=all)|08: |15",
    /* PPS_MSG_SUBJ       */ "|07  subj |08: |15",
    /* PPS_MSG_BODY       */ "|08\r\n  type your message. a line with |15/s|08 saves, |15/a|08 aborts.|07\r\n",
    /* PPS_EDIT_HELP      */ "|08 \20 |15^Z|08 save   |15^X|08 abort   |08\372 word-wrap   \372 Vendetta/X editor |07",
    /* PPS_GOODBYE        */ "|07\r\n  |12NO CARRIER|08 -- |07Vendetta/X will miss you.|07\r\n"
};

/* Key names parallel to the enum (order must match strtab.h). */
static const char *NAMES[PPS_COUNT] = {
    "HANDLE_PROMPT",
    "HANDLE_NOTFOUND",
    "NEWHANDLE_PROMPT",
    "HANDLE_TAKEN",
    "LOCATION_PROMPT",
    "PRESS_KEY",
    "MISSING_ART",
    "LASTCALL_TITLE",
    "USERLIST_TITLE",
    "ONELINER_TITLE",
    "ONELINER_PROMPT",
    "MSG_TO",
    "MSG_SUBJ",
    "MSG_BODY",
    "EDIT_HELP",
    "GOODBYE"
};

static const char *g_override[PPS_COUNT];
static char        g_pool[2048];        /* backing store for loaded strings */
static unsigned    g_used;

void pps_init(void)
{
    int i;
    for (i = 0; i < PPS_COUNT; i++)
        g_override[i] = (const char *)0;
    g_used = 0;
}

static int name_to_id(const char *name)
{
    int i;
    for (i = 0; i < PPS_COUNT; i++)
        if (strcmp(name, NAMES[i]) == 0)
            return i;
    return -1;
}

/* Copy src into the pool, expanding backslash escapes; NUL-terminate.
 * Returns a pointer into the pool, or NULL if the pool is full. */
static char *intern(const char *src)
{
    char *out = g_pool + g_used;
    char *p = out;
    char *end = g_pool + sizeof g_pool - 1;
    while (*src && p < end) {
        if (*src == '\\' && src[1]) {
            src++;
            switch (*src) {
                case 'r': *p++ = '\r'; break;
                case 'n': *p++ = '\n'; break;
                case 't': *p++ = '\t'; break;
                case 'e': *p++ = '\x1b'; break;
                default:  *p++ = *src;  break;   /* \\ -> \, and any other */
            }
            src++;
        } else {
            *p++ = *src++;
        }
    }
    if (p >= end) return (char *)0;            /* would overflow: refuse, keep default */
    *p++ = '\0';
    g_used = (unsigned)(p - g_pool);
    return out;
}

int pps_load(const char *path)
{
    FILE *f = fopen(path, "rb");
    char line[512];
    int loaded = 0;
    if (f == (FILE *)0)
        return -1;

    while (fgets(line, (int)sizeof line, f)) {
        char *eq, *key, *val, *p;
        size_t n;

        /* strip trailing CR/LF */
        n = strlen(line);
        while (n > 0 && (line[n - 1] == '\n' || line[n - 1] == '\r'))
            line[--n] = '\0';

        key = line;
        while (*key == ' ' || *key == '\t') key++;
        if (*key == '\0' || *key == '#')            /* blank or comment */
            continue;

        eq = strchr(key, '=');
        if (eq == (char *)0)                          /* no '=': malformed, skip */
            continue;
        *eq = '\0';
        val = eq + 1;

        /* trim trailing space on key */
        p = eq - 1;
        while (p >= key && (*p == ' ' || *p == '\t')) *p-- = '\0';

        {
            int id = name_to_id(key);
            if (id >= 0) {
                char *stored = intern(val);
                if (stored) { g_override[id] = stored; loaded++; }
            }
        }
    }
    fclose(f);
    return loaded;
}

const char *pps_get(int id)
{
    if (id < 0 || id >= PPS_COUNT)
        return "";
    if (g_override[id])
        return g_override[id];
    return DEFAULTS[id];
}
