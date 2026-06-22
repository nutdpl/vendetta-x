#ifndef PIGPEN_PAGE_H
#define PIGPEN_PAGE_H

/*
 * INTER-NODE MESSAGE BUS -- NODEMSG.DAT.
 *
 * A small append-only log that carries short messages between nodes: page-the-
 * sysop requests, node-to-node chat lines, and acknowledgements. Every message
 * gets a monotonic sequence number. A node reads its inbox by polling with a
 * cursor (the last seq it has consumed), so it only ever sees new traffic --
 * no per-node mutable read pointers on disk, which keeps the multi-writer story
 * simple (everyone only ever appends).
 *
 * Format: 16-byte header ("PMSG" | u16 ver | u16 recsize | u32 count | 4 rsvd)
 * then fixed PP_NMSG_REC-byte records, little-endian field-by-field at known
 * offsets -- same tool-readable discipline as the other stores.
 *
 * Addressing: to_node is a node number (1..PP_MAX_NODES) or PG_SYSOP (0) for
 * the sysop console. from_node is the sender's node (0 if the sysop console).
 */
#include "pigtypes.h"

#define PG_SYSOP            0     /* to_node value meaning "the sysop console" */
#define PP_MSG_HANDLE_MAX   32
#define PP_MSG_TEXT_MAX     80
#define PP_NMSG_REC          128   /* bytes on disk per message (padded) */

/* message kinds (pp_nodemsg.kind) */
#define PG_CHAT   0    /* a node-to-node chat line */
#define PG_PAGE   1    /* "paging you" -- request to chat / page the sysop */
#define PG_ACK    2    /* acknowledgement ("on my way" / chat accepted) */
#define PG_END    3    /* end-of-chat marker */

typedef struct {
    pp_u32 seq;                          /* monotonic; assigned by pg_send */
    pp_u16 to_node;                      /* destination node, or PG_SYSOP */
    pp_u16 from_node;                    /* sender node (0 = sysop console) */
    pp_u8  kind;                         /* PG_CHAT / PG_PAGE / PG_ACK / PG_END */
    char   from_handle[PP_MSG_HANDLE_MAX];
    char   text[PP_MSG_TEXT_MAX];
    pp_u32 when;                         /* unix time */
} pp_nodemsg;

/* Open (creating if absent) the bus at `path`. 0 on success. */
int    pg_open(const char *path);
void   pg_close(void);

/* Append a message addressed to `to_node`. Returns the assigned sequence
 * number (>=1) on success, or 0 on failure. */
pp_u32 pg_send(int to_node, int from_node, const char *from_handle,
               int kind, const char *text, pp_u32 when);

/* Poll for the next message addressed to `node_no` with seq > *cursor.
 * On a hit: copies it to *out, advances *cursor to that seq, returns 1.
 * On no new message: returns 0 and leaves *cursor unchanged. Call in a loop
 * to drain the inbox. */
int    pg_poll(int node_no, pp_u32 *cursor, pp_nodemsg *out);

/* The highest sequence number on file (0 if empty). A node sets its cursor to
 * this at logon so it skips backlog and only sees traffic from here on. */
pp_u32 pg_seq(void);

/* Total messages on file. */
pp_u32 pg_count(void);

/* Serialization (exposed for unit tests): record <-> PP_NMSG_REC bytes. */
void   pg_pack(const pp_nodemsg *m, pp_u8 *rec);
void   pg_unpack(const pp_u8 *rec, pp_nodemsg *m);

#endif /* PIGPEN_PAGE_H */
