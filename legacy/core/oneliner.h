#ifndef PIGPEN_ONELINER_H
#define PIGPEN_ONELINER_H

/*
 * The oneliner wall (DESIGN.md phase 3). A newest-first ring in ONELINER.DAT:
 * 16-byte header ("PONE" | u16 ver | u16 recsize | u32 count | 4 rsvd) then up
 * to OL_MAX 100-byte records (author[24], text[72], u32 when). Same
 * little-endian, tool-readable discipline as the userbase/callers.
 */
#include "pigtypes.h"

#define OL_MAX 15

typedef struct {
    char   author[24];
    char   text[72];
    pp_u32 when;            /* unix time */
} pp_oneliner;

int  ol_open(const char *path);
void ol_close(void);
void ol_push(const char *author, const char *text, pp_u32 when);  /* prepend; persists */
int  ol_count(void);
int  ol_get(int i, pp_oneliner *out);   /* i=0 is newest; 0 on success */

#endif /* PIGPEN_ONELINER_H */
