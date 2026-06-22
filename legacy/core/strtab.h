#ifndef PIGPEN_STRTAB_H
#define PIGPEN_STRTAB_H

/*
 * The string table (DESIGN.md: "no English in the binary"). C refers to
 * string IDs only; the text lives in VENDX.STR, editable by the sysop.
 * Robustness lesson from Synchronet's text.dat: stock strings are compiled
 * in as defaults, so a missing or damaged VENDX.STR never stops the board
 * from booting -- the loaded file only *overrides* what it successfully
 * parses. Strings may contain pipe codes; render them via render_strn.
 */

enum {
    PPS_HANDLE_PROMPT = 0,   /* "who goes there?" handle entry prompt */
    PPS_HANDLE_NOTFOUND,     /* unknown handle: offer to sign up */
    PPS_NEWHANDLE_PROMPT,    /* "pick a handle" for a new account */
    PPS_HANDLE_TAKEN,        /* chosen handle already exists */
    PPS_LOCATION_PROMPT,     /* new caller: where are you calling from? */
    PPS_PRESS_KEY,           /* "press a key ..." continue prompt */
    PPS_MISSING_ART,         /* shown when an art file can't be opened */
    PPS_LASTCALL_TITLE,      /* last-callers screen heading text */
    PPS_USERLIST_TITLE,      /* user list heading text */
    PPS_ONELINER_TITLE,      /* oneliner wall heading text */
    PPS_ONELINER_PROMPT,     /* "drop a line ..." add prompt */
    PPS_MSG_TO,              /* post: recipient prompt */
    PPS_MSG_SUBJ,            /* post: subject prompt */
    PPS_MSG_BODY,            /* post: body entry instructions */
    PPS_EDIT_HELP,           /* full-screen editor help/status line */
    PPS_GOODBYE,             /* logoff line */
    PPS_COUNT
};

void        pps_init(void);              /* install compiled defaults */
int         pps_load(const char *path);  /* overlay overrides; count loaded, -1 if no file */
const char *pps_get(int id);             /* never NULL: override, else default, else "" */

#endif /* PIGPEN_STRTAB_H */
