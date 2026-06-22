#ifndef PIGPEN_CALLERS_H
#define PIGPEN_CALLERS_H

/*
 * Persisted last-callers log (DESIGN.md phase 3). A small newest-first ring in
 * LASTCALL.DAT: 16-byte header ("PLCL" | u16 ver | u16 recsize | u32 count |
 * 4 rsvd) then up to LC_MAX 60-byte records (handle[32], location[24], u32
 * when). Same little-endian, tool-readable discipline as the userbase.
 */
#include "pigtypes.h"

#define LC_MAX 10

typedef struct {
    char   handle[32];
    char   location[24];
    pp_u32 when;            /* unix time */
} pp_caller;

int  lc_open(const char *path);     /* load (creating if absent); 0 on success */
void lc_close(void);
void lc_push(const char *handle, const char *location, pp_u32 when);  /* prepend; persists */
int  lc_count(void);
int  lc_get(int i, pp_caller *out); /* i=0 is newest; 0 on success */

#endif /* PIGPEN_CALLERS_H */
