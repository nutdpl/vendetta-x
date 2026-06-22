#ifndef PIGPEN_QSCAN_H
#define PIGPEN_QSCAN_H

/*
 * Per-user, per-area READ POINTERS (QSCAN.DAT). Lets the board show
 * "new since last read" and resume where a caller left off.
 *
 * 16-byte header ("PQSC" | u16 ver | u16 recsize | u32 count | 4 rsvd) then
 * fixed 22-byte records (u16 user_idx, char tag[16], u32 lastread), little-
 * endian, field-by-field -- same tool-readable discipline as oneliner.c.
 */
#include "pigtypes.h"

#define QS_MAX 1024

int    qs_open(const char *path);
void   qs_close(void);
pp_u32 qs_get(int user_idx, const char *tag);                 /* 0 if none */
void   qs_set(int user_idx, const char *tag, pp_u32 lastread); /* upsert; persists */
int    qs_count(void);

#endif /* PIGPEN_QSCAN_H */
