#ifndef PIGPEN_USERBASE_H
#define PIGPEN_USERBASE_H

/*
 * User accounts on disk (DESIGN.md phase 3). The board remembers callers.
 *
 * Identity is handle-only -- no password ("no nup; we are just glad you
 * called" is the board's whole personality). USER.DAT is a 16-byte header
 * followed by fixed 96-byte records. Records are serialized field-by-field as
 * little-endian at known offsets (NOT raw struct fwrite), so the format does
 * not depend on compiler struct padding and external tools (python, etc.) can
 * read it. See the offset map in userbase.c.
 */
#include "pigtypes.h"

#define PP_HANDLE_MAX 32     /* incl. NUL */
#define PP_LOC_MAX    24
#define PP_TAG_MAX    48     /* one-line "about me" / user note */
#define PP_GRP_MAX    16     /* scene group / affiliation */
/* new-user application profile fields */
#define PP_RNAME_MAX  36     /* real name */
#define PP_EMAIL_MAX  48
#define PP_BDATE_MAX  12     /* birth date, e.g. "1985-03-21" */
#define PP_CITY_MAX   28     /* city / state */
#define PP_ZIP_MAX    12
#define PP_COMP_MAX   24     /* computer type */
#define PP_PW_MAX     16     /* password, stored hashed (8 hex + NUL) */

/* user.flags bits */
#define UF_ANSI  0x01        /* wants ANSI graphics */
#define UF_PAUSE 0x02        /* pause between screens */

typedef struct {
    char   handle[PP_HANDLE_MAX];
    char   location[PP_LOC_MAX];
    pp_u8  sl;              /* security level (board access, 0-255) */
    pp_u8  flags;          /* UF_ANSI | UF_PAUSE */
    pp_u8  dsl;             /* download security level (file access) */
    pp_u8  prot;           /* default protocol (0=Zmodem,1=Ymodem,2=Xmodem) */
    pp_u32 times_called;
    pp_u32 first_call;      /* unix time */
    pp_u32 last_call;       /* unix time */
    pp_u32 posts;
    char   tagline[PP_TAG_MAX];
    char   group[PP_GRP_MAX];
    pp_u16 ar;             /* access-right bits A..P (per-board groups) */
    pp_u16 dar;            /* dir-access-right bits A..P */
    pp_u16 restr;         /* restriction bits A..P (surgical feature blocks) */
    char   real_name[PP_RNAME_MAX];   /* -- new-user application profile -- */
    char   email[PP_EMAIL_MAX];
    char   birthdate[PP_BDATE_MAX];
    char   city[PP_CITY_MAX];
    char   zip[PP_ZIP_MAX];
    char   computer[PP_COMP_MAX];
    char   pwhash[PP_PW_MAX];
} pp_user;

/* Open (creating if absent) the userbase at `path`. 0 on success. */
int    ub_open(const char *path);
void   ub_close(void);

pp_u32 ub_count(void);

/* Find a user by handle (case-insensitive). On hit: fills *out (if non-NULL),
 * sets *index (if non-NULL), returns 1. Miss returns 0. */
int    ub_find(const char *handle, pp_user *out, pp_u32 *index);

/* Append a new user; sets *index. Returns 0 on success. */
int    ub_add(const pp_user *u, pp_u32 *index);

/* Read / overwrite a record by index. 0 on success. */
int    ub_get(pp_u32 index, pp_user *out);
int    ub_update(pp_u32 index, const pp_user *u);

/* Serialization (exposed for unit tests): record <-> PP_USER_REC bytes. */
#define PP_USER_REC 384
void   ub_pack(const pp_user *u, pp_u8 *rec);
void   ub_unpack(const pp_u8 *rec, pp_user *u);

#endif /* PIGPEN_USERBASE_H */
