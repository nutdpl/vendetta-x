#ifndef PIGPEN_NODE_H
#define PIGPEN_NODE_H

/*
 * MULTINODE PRESENCE TABLE -- NODE.DAT, the who's-online / waiting-for-callers
 * backbone.
 *
 * Unlike every other store in Vendetta/X this one is RANDOM-ACCESS and rewritten
 * IN PLACE: there is exactly one fixed slot per node (1..PP_MAX_NODES), and a
 * running node overwrites its own slot as its status changes. The classic 90s
 * multinode model is several Vendetta/X processes (separate machines, or DESQview
 * windows) sharing one data directory; this table is how they see each other.
 *
 * Format: 16-byte header ("PNOD" | u16 ver | u16 recsize | u32 max_nodes |
 * 4 rsvd) then PP_MAX_NODES fixed PP_NODE_REC-byte records, serialized
 * field-by-field little-endian at known offsets (NOT raw struct fwrite) so the
 * layout is identical on 16-bit Watcom DOS and a modern host, and readable by
 * external tools. nd_open() creates the file with every slot idle if absent.
 *
 * Slot N lives at file offset 16 + (N-1)*PP_NODE_REC. node numbers are 1-based.
 */
#include "pigtypes.h"

#define PP_MAX_NODES        16    /* fixed slot count on file */
#define PP_NODE_HANDLE_MAX  32    /* caller handle, incl. NUL */
#define PP_NODE_LOC_MAX     24    /* caller location */
#define PP_NODE_ACTION_MAX  40    /* "Reading messages", "At main menu", ... */
#define PP_NODE_REC         112   /* bytes on disk per slot (padded for growth) */

/* status values for pp_node.status */
#define ND_IDLE     0    /* no caller -- slot free / waiting for callers */
#define ND_ONLINE   1    /* a caller is connected and active */
#define ND_BETWEEN  2    /* logging on / between calls (claimed, not yet live) */

/* bits for pp_node.flags */
#define ND_FLAG_CHATOK  0x01   /* caller is open to node-to-node chat */

typedef struct {
    pp_u16 node_no;                       /* 1..PP_MAX_NODES (0 = empty slot) */
    pp_u8  status;                        /* ND_IDLE / ND_ONLINE / ND_BETWEEN */
    pp_u8  flags;                         /* ND_FLAG_* */
    char   handle[PP_NODE_HANDLE_MAX];    /* who's on this node ("" if idle) */
    char   location[PP_NODE_LOC_MAX];
    char   action[PP_NODE_ACTION_MAX];    /* what they're doing right now */
    pp_u32 logon_time;                    /* unix time the caller logged on */
    pp_u32 last_update;                   /* unix time of the last slot write */
} pp_node;

/* Open (creating PP_MAX_NODES idle slots if absent) the table. 0 on success.
 * Safe to call from every node process against the same path. */
int  nd_open(const char *path);
void nd_close(void);

/* Claim `node_no` for a caller logging on: status -> ND_BETWEEN, handle +
 * location set, logon_time = when. Returns 0 on success, nonzero on bad node. */
int  nd_claim(int node_no, const char *handle, const char *location, pp_u32 when);

/* Update the live action string (and status -> ND_ONLINE) for `node_no`.
 * Call at menu/command transitions so who's-online stays current. 0 on ok. */
int  nd_action(int node_no, const char *action, pp_u32 when);

/* Toggle the caller's "open to chat" flag. 0 on success. */
int  nd_set_chatok(int node_no, int ok);

/* Release `node_no` back to ND_IDLE (caller logged off / carrier lost). The
 * handle/location/action are cleared. 0 on success. */
int  nd_release(int node_no, pp_u32 when);

/* Read a slot by node number (1..PP_MAX_NODES). 0 on success, nonzero bad. */
int  nd_get(int node_no, pp_node *out);

/* Number of slots on file (== PP_MAX_NODES for a healthy table). */
int  nd_max(void);

/* How many slots are currently ND_ONLINE or ND_BETWEEN. */
int  nd_online_count(void);

/* Serialization (exposed for unit tests): record <-> PP_NODE_REC bytes. */
void nd_pack(const pp_node *n, pp_u8 *rec);
void nd_unpack(const pp_u8 *rec, pp_node *n);

#endif /* PIGPEN_NODE_H */
