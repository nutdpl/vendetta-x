#ifndef PIGPEN_QWK_H
#define PIGPEN_QWK_H

/*
 * QWK / REP offline-mail packets.
 *
 * QWK is the classic offline-reader format: the board exports new messages as
 * a .QWK packet (a MESSAGES.DAT blob + a CONTROL.DAT manifest), the caller
 * reads/replies offline in an Ascii/QWK reader, then uploads a .REP packet
 * (a single MESSAGES-style blob) the board imports.
 *
 * MESSAGES.DAT is a stream of 128-byte blocks: block 0 is a fixed text header,
 * then each message is one 128-byte HEADER block (fixed-offset fields: status,
 * number, MM-DD-YY date, HH:MM time, to[25], from[25], subject[25], a 6-char
 * password, reference number, block count incl. the header, active flag, and a
 * conference number) followed by ceil(textlen/128) text blocks whose lines end
 * in 0xE3 (the QWK pi-substitution for CR). All fields are fixed-width ASCII;
 * implement the PUBLISHED layout from scratch (clean-room -- copy no code).
 *
 * This module owns the FORMAT only (build a packet, parse a packet); choosing
 * which messages to export and applying imported replies lives in the board.
 */
#include "pigtypes.h"

#define QWK_BLOCK      128
#define QWK_TO_MAX      25
#define QWK_FROM_MAX    25
#define QWK_SUBJ_MAX    25

/* One logical message handed to/from the packet layer. body uses '\n' line
 * endings (the codec converts to/from the on-disk 0xE3 convention). */
typedef struct {
    pp_u16 conference;
    pp_u32 number;                 /* message number within the conference */
    pp_u32 when;                   /* unix time (codec formats MM-DD-YY HH:MM) */
    char   to[QWK_TO_MAX + 1];
    char   from[QWK_FROM_MAX + 1];
    char   subject[QWK_SUBJ_MAX + 1];
    const char *body;              /* NUL-terminated; '\n' separated */
} pp_qwk_msg;

/* ---- writing a .QWK packet --------------------------------------------- */
/* qwk_begin opens MESSAGES.DAT at `msgpath` and writes the block-0 header
 * stamped with `board`. Then qwk_add appends each message. qwk_finish flushes
 * and closes. CONTROL.DAT is written separately by qwk_write_control. */
int  qwk_begin(const char *msgpath, const char *board);
int  qwk_add(const pp_qwk_msg *m);              /* 0 on success */
int  qwk_finish(void);                          /* returns message count written */

/* Write CONTROL.DAT (board identity + conference list) at `path`. */
int  qwk_write_control(const char *path, const char *board, const char *location,
                       const char *sysop, const char **conf_names, int nconf);

/* ---- reading a .REP / .QWK MESSAGES.DAT blob --------------------------- */
/* Parse `msgpath`, invoking cb once per message. The pp_qwk_msg passed to cb
 * is valid only for the duration of the call (body points at a reused buffer).
 * Returns the number of messages parsed, or <0 on a malformed packet. */
typedef void (*qwk_msg_cb)(void *user, const pp_qwk_msg *m);
int  qwk_read(const char *msgpath, qwk_msg_cb cb, void *user);

#endif /* PIGPEN_QWK_H */
