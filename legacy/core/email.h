#ifndef PIGPEN_EMAIL_H
#define PIGPEN_EMAIL_H

/*
 * Private email (DESIGN.md phase 3 -- caller-to-caller mail). One flat,
 * append-only mailbox data/MAIL.DAT shared by all users: a 16-byte header
 * ("PEML" | u16 ver | u16 recsize | u32 count | 4 rsvd) then fixed
 * PP_MAIL_REC (720) byte records serialized field-by-field as little-endian
 * (NOT raw struct fwrite), so the format is independent of compiler padding
 * and tool-readable like the rest of data/. Mail is never physically removed;
 * deletion just sets a flag (bit1). One mailbox open at a time (single node).
 */
#include "pigtypes.h"

#define EM_FROM 32
#define EM_TO   32          /* recipient handle */
#define EM_SUBJ 48
#define EM_BODY 600
#define PP_MAIL_REC 720

/* flag bits */
#define EM_FLAG_READ    0x01u
#define EM_FLAG_DELETED 0x02u

typedef struct {
    char   from[EM_FROM];
    char   to[EM_TO];           /* recipient handle */
    char   subject[EM_SUBJ];
    pp_u32 when;                /* unix time */
    pp_u32 flags;               /* bit0=read, bit1=deleted */
    char   body[EM_BODY];       /* NUL-terminated */
} pp_mail;

/* Open (creating if absent) the mailbox at `path`. 0 on success. */
int  em_open(const char *path);
void em_close(void);

/* Total record count (including read/deleted). */
int  em_count(void);

/* Append a new message + persist. Returns 0 on success. */
int  em_send(const pp_mail *m);

/* Read / overwrite a record by absolute index. 0 on success. */
int  em_get(int index, pp_mail *out);
int  em_update(int index, const pp_mail *m);

/* Count non-deleted mail whose 'to' matches `handle` (case-insensitive). */
int  em_count_to(const char *handle);

/* Fetch the n-th (0-based) non-deleted message addressed to `handle`
 * (case-insensitive). On hit: fills *out (if non-NULL), sets *out_index to the
 * absolute record index (if non-NULL), returns 0. Miss returns 1. */
int  em_get_to(const char *handle, int n, pp_mail *out, int *out_index);

#endif /* PIGPEN_EMAIL_H */
