#ifndef PIGPEN_BBSLIST_H
#define PIGPEN_BBSLIST_H

/*
 * BBS LIST -- BBSLIST.DAT, a user-contributed registry of other boards.
 *
 * Callers add entries pointing at other systems (telnet host:port and the
 * like); the list is shown to everyone. The file is a 16-byte header
 * followed by fixed PP_BBS_REC-byte records, serialized field-by-field as
 * little-endian at known offsets (NOT raw struct fwrite), so the format does
 * not depend on compiler struct padding and is identical on 16-bit Watcom DOS
 * and a modern host. See the offset map in bbslist.c.
 *
 * Storage is append-only / newest-last: bl_add() writes to the end and
 * bl_get(0,..) returns the oldest entry.
 */
#include "pigtypes.h"

#define PP_BBS_NAME_MAX  40    /* board name, incl. NUL */
#define PP_BBS_ADDR_MAX  48    /* "vendetta-x.org:23" telnet host:port */
#define PP_BBS_SYSOP_MAX 32    /* sysop handle */
#define PP_BBS_BY_MAX    32    /* handle of the caller who contributed it */

typedef struct {
    char   name[PP_BBS_NAME_MAX];
    char   address[PP_BBS_ADDR_MAX];
    char   sysop[PP_BBS_SYSOP_MAX];
    char   added_by[PP_BBS_BY_MAX];
    pp_u32 when;            /* unix time the entry was added */
} pp_bbs;

/* Open (creating if absent) the BBS list at `path`. 0 on success. */
int    bl_open(const char *path);
void   bl_close(void);

/* Number of entries currently on file. */
pp_u32 bl_count(void);

/* Read a record by index (0..count-1). 0 on success, nonzero on bad index. */
int    bl_get(pp_u32 index, pp_bbs *out);

/* Append a new entry. Returns 0 on success. */
int    bl_add(const pp_bbs *b);

/* Serialization (exposed for unit tests): record <-> PP_BBS_REC bytes. */
#define PP_BBS_REC 160
void   bl_pack(const pp_bbs *b, pp_u8 *rec);
void   bl_unpack(const pp_u8 *rec, pp_bbs *b);

#endif /* PIGPEN_BBSLIST_H */
