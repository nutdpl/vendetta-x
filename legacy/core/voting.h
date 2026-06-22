#ifndef PIGPEN_VOTING_H
#define PIGPEN_VOTING_H

/*
 * The voting booth (VOTE.DAT). Sysop polls in the same little-endian,
 * tool-readable discipline as the userbase/oneliner:
 *   16-byte header ("PVOT" | u16 ver | u16 recsize | u32 count | 4 rsvd)
 *   then `count` fixed VT_REC-byte records.
 *
 * Each record:
 *   question[80] | u8 noptions (2..8) | options[8][40] |
 *   u32 counts[8] | u32 when | u8 voted[32]   (a 256-bit bitset of the
 *   user indices that have already cast a vote in this poll).
 */
#include "pigtypes.h"

#define VT_OPTS_MAX  8
#define VT_Q_MAX     80     /* including the trailing NUL */
#define VT_OPT_MAX   40     /* including the trailing NUL */
#define VT_VOTED_LEN 32     /* bytes -> 256 user indices */

typedef struct {
    char   question[VT_Q_MAX];
    pp_u8  noptions;                       /* 2..8 */
    char   options[VT_OPTS_MAX][VT_OPT_MAX];
    pp_u32 counts[VT_OPTS_MAX];
    pp_u32 when;                           /* unix time the poll was created */
    pp_u8  voted[VT_VOTED_LEN];            /* bitset, bit user_idx */
} pp_poll;

int    vt_open(const char *path);          /* open-or-create; 0 on success */
void   vt_close(void);
int    vt_count(void);
int    vt_get(int i, pp_poll *out);        /* 0 on success, -1 on bad index */
int    vt_add(const pp_poll *p);           /* append; persists; 0 on success */

/* Record a vote: increments counts[option], sets voted bit for user_idx,
 * persists. Returns 0, or -1 on bad args / already voted. */
int    vt_cast(int poll_idx, int option, int user_idx);

/* Nonzero if user_idx has already voted in poll_idx. */
int    vt_has_voted(int poll_idx, int user_idx);

#endif /* PIGPEN_VOTING_H */
