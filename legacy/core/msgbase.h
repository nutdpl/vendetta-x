#ifndef PIGPEN_MSGBASE_H
#define PIGPEN_MSGBASE_H

/*
 * Message bases (DESIGN.md phase 3 -- the heart of "it's a board"). Each area
 * is its own file data/<TAG>.MSG: a 16-byte header ("PMSG" | u16 ver | u16
 * recsize | u32 count | 4 rsvd) then append-only fixed PP_MSG_REC (1080) byte
 * records. Bodies are fixed-max (classic BBS) so the format stays a simple
 * tool-readable record array like the rest of the data/ stores. One area open
 * at a time (single node).
 */
#include "pigtypes.h"

#define MB_FROM 32
#define MB_TO   32
#define MB_SUBJ 48
#define MB_BODY 960
#define PP_MSG_REC 1084

#define MB_NO_PARENT 0u        /* reply_to value meaning "this is a root post" */

typedef struct {
    char   from[MB_FROM];
    char   to[MB_TO];
    char   subject[MB_SUBJ];
    pp_u32 when;            /* unix time */
    pp_u32 flags;
    char   body[MB_BODY];   /* lines joined with CRLF, NUL-terminated */
    pp_u32 reply_to;        /* 1-based number of the parent msg, or MB_NO_PARENT */
} pp_msg;

int  mb_open(const char *tag);   /* data/<TAG>.MSG, creating if absent; 0 ok */
void mb_close(void);
int  mb_count(void);
int  mb_get(int index, pp_msg *out);
int  mb_add(const pp_msg *m);

/* Build a quoted-reply body in out (<= outsz, NUL-terminated): an attribution
 * line "<who> wrote:" then every line of src prefixed with "> ". src lines are
 * separated by '\n' ('\r' is dropped). Truncates safely. Returns strlen(out). */
int  mb_quote(const char *src, const char *who, char *out, int outsz);

#endif /* PIGPEN_MSGBASE_H */
