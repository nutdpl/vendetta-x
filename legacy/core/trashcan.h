#ifndef PIGPEN_TRASHCAN_H
#define PIGPEN_TRASHCAN_H

/*
 * trashcan.h -- BAD-WORD / TRASHCAN filter (BBS-FEATURE-STUDY: "Trashcan").
 *
 * Moderates posts and handles by matching against a banned-word list loaded
 * from a plain text file (default data/TRASHCAN): one word per line, lines
 * beginning with '#' are comments, blank lines are ignored. Matching is a
 * case-insensitive substring test, so "ass" blocks "Bass" -- keep the list
 * deliberately coarse and trust a sysop to curate it.
 *
 * The list lives in static storage; tc_load() replaces it wholesale and is
 * tolerant of a missing file (returns 0, never fails).
 */

#define TC_MAX_WORDS 256   /* most banned words we will hold        */
#define TC_MAX_LEN   32     /* longest banned word (incl. NUL slot)  */

int tc_load(const char *path);   /* (re)load list; returns count loaded, 0 if absent */
int tc_blocked(const char *text); /* 1 if text contains any banned word, else 0      */
int tc_count(void);               /* number of words currently loaded                */

#endif /* PIGPEN_TRASHCAN_H */
