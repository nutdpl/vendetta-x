/*
 * node.c -- NODE.DAT, the multinode presence table (who's-online).
 *
 * RANDOM-ACCESS, rewritten IN PLACE: one fixed slot per node, slot N at
 * file offset 16 + (N-1)*PP_NODE_REC. Unlike the append-only stores, a
 * node seeks to its own slot and overwrites it as its status changes.
 *
 * File layout (all little-endian):
 *   header (16 bytes): "PNOD" | u16 version | u16 recsize | u32 max_nodes | 4 rsvd
 *   then max_nodes records of PP_NODE_REC (112) bytes each:
 *     off   0  u16 node_no
 *     off   2  u8  status
 *     off   3  u8  flags
 *     off   4  handle[32]    (NUL-padded)
 *     off  36  location[24]
 *     off  60  action[40]
 *     off 100  u32 logon_time
 *     off 104  u32 last_update
 *     off 108  4 reserved (pad to 112)
 */
#include <stdio.h>
#include <string.h>
#include "node.h"

#define HDR_SIZE 16
#define ND_VERSION 1

static FILE *g_fp;
static int   g_max;

/* ---- little-endian field helpers ---------------------------------------- */

static void put_u16(pp_u8 *p, pp_u16 v) { p[0] = (pp_u8)(v & 0xff); p[1] = (pp_u8)((v >> 8) & 0xff); }
static void put_u32(pp_u8 *p, pp_u32 v)
{
    p[0] = (pp_u8)(v & 0xff);          p[1] = (pp_u8)((v >> 8) & 0xff);
    p[2] = (pp_u8)((v >> 16) & 0xff);  p[3] = (pp_u8)((v >> 24) & 0xff);
}
static pp_u16 get_u16(const pp_u8 *p) { return (pp_u16)(p[0] | (p[1] << 8)); }
static pp_u32 get_u32(const pp_u8 *p)
{
    return (pp_u32)p[0] | ((pp_u32)p[1] << 8) | ((pp_u32)p[2] << 16) | ((pp_u32)p[3] << 24);
}

/* copy a NUL-padded fixed field */
static void put_str(pp_u8 *p, const char *s, int max)
{
    int i = 0;
    while (i < max && s[i]) { p[i] = (pp_u8)s[i]; i++; }
    while (i < max) { p[i] = 0; i++; }
}

void nd_pack(const pp_node *n, pp_u8 *rec)
{
    memset(rec, 0, PP_NODE_REC);
    put_u16(rec + 0,   n->node_no);
    rec[2] = n->status;
    rec[3] = n->flags;
    put_str(rec + 4,   n->handle,   PP_NODE_HANDLE_MAX);
    put_str(rec + 36,  n->location, PP_NODE_LOC_MAX);
    put_str(rec + 60,  n->action,   PP_NODE_ACTION_MAX);
    put_u32(rec + 100, n->logon_time);
    put_u32(rec + 104, n->last_update);
}

void nd_unpack(const pp_u8 *rec, pp_node *n)
{
    memset(n, 0, sizeof *n);
    n->node_no = get_u16(rec + 0);
    n->status  = rec[2];
    n->flags   = rec[3];
    memcpy(n->handle,   rec + 4,  PP_NODE_HANDLE_MAX - 1);  n->handle[PP_NODE_HANDLE_MAX - 1]   = '\0';
    memcpy(n->location, rec + 36, PP_NODE_LOC_MAX - 1);     n->location[PP_NODE_LOC_MAX - 1]    = '\0';
    memcpy(n->action,   rec + 60, PP_NODE_ACTION_MAX - 1);  n->action[PP_NODE_ACTION_MAX - 1]   = '\0';
    n->logon_time  = get_u32(rec + 100);
    n->last_update = get_u32(rec + 104);
}

/* ---- header ------------------------------------------------------------- */

static int read_header(void)
{
    pp_u8 h[HDR_SIZE];
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fread(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    if (h[0] != 'P' || h[1] != 'N' || h[2] != 'O' || h[3] != 'D') return 1;
    if (get_u16(h + 6) != PP_NODE_REC) return 1;     /* record size mismatch */
    g_max = (int)get_u32(h + 8);
    return 0;
}

static int write_header(void)
{
    pp_u8 h[HDR_SIZE];
    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'N'; h[2] = 'O'; h[3] = 'D';
    put_u16(h + 4, ND_VERSION);
    put_u16(h + 6, PP_NODE_REC);
    put_u32(h + 8, (pp_u32)PP_MAX_NODES);
    if (fseek(g_fp, 0L, SEEK_SET) != 0) return 1;
    if (fwrite(h, 1, HDR_SIZE, g_fp) != HDR_SIZE) return 1;
    fflush(g_fp);
    return 0;
}

/* write the header followed by PP_MAX_NODES idle slots */
static int init_file(void)
{
    pp_u8 rec[PP_NODE_REC];
    pp_node n;
    int i;
    g_max = PP_MAX_NODES;
    if (write_header() != 0) return 1;
    for (i = 1; i <= PP_MAX_NODES; i++) {
        memset(&n, 0, sizeof n);
        n.node_no = (pp_u16)i;
        n.status  = ND_IDLE;
        nd_pack(&n, rec);
        if (fwrite(rec, 1, PP_NODE_REC, g_fp) != PP_NODE_REC) return 1;
    }
    fflush(g_fp);
    return 0;
}

/* ---- slot helpers ------------------------------------------------------- */

static long slot_off(int node_no)
{
    return (long)HDR_SIZE + (long)(node_no - 1) * PP_NODE_REC;
}

static int read_slot(int node_no, pp_node *out)
{
    pp_u8 rec[PP_NODE_REC];
    if (g_fp == (FILE *)0 || node_no < 1 || node_no > g_max) return 1;
    if (fseek(g_fp, slot_off(node_no), SEEK_SET) != 0) return 1;
    if (fread(rec, 1, PP_NODE_REC, g_fp) != PP_NODE_REC) return 1;
    nd_unpack(rec, out);
    return 0;
}

static int write_slot(int node_no, const pp_node *n)
{
    pp_u8 rec[PP_NODE_REC];
    if (g_fp == (FILE *)0 || node_no < 1 || node_no > g_max) return 1;
    nd_pack(n, rec);
    if (fseek(g_fp, slot_off(node_no), SEEK_SET) != 0) return 1;
    if (fwrite(rec, 1, PP_NODE_REC, g_fp) != PP_NODE_REC) return 1;
    fflush(g_fp);
    return 0;
}

/* ---- public ------------------------------------------------------------- */

int nd_open(const char *path)
{
    g_max = 0;
    g_fp = fopen(path, "r+b");
    if (g_fp == (FILE *)0) {
        g_fp = fopen(path, "w+b");      /* create fresh */
        if (g_fp == (FILE *)0) return 1;
        return init_file();
    }
    if (read_header() != 0) {           /* empty or damaged -> reinitialize */
        return init_file();
    }
    return 0;
}

void nd_close(void)
{
    if (g_fp) { fclose(g_fp); g_fp = (FILE *)0; }
}

int nd_claim(int node_no, const char *handle, const char *location, pp_u32 when)
{
    pp_node n;
    if (read_slot(node_no, &n) != 0) return 1;
    n.node_no = (pp_u16)node_no;
    n.status  = ND_BETWEEN;
    strncpy(n.handle,   handle,   PP_NODE_HANDLE_MAX - 1); n.handle[PP_NODE_HANDLE_MAX - 1]   = '\0';
    strncpy(n.location, location, PP_NODE_LOC_MAX - 1);    n.location[PP_NODE_LOC_MAX - 1]    = '\0';
    n.action[0]    = '\0';
    n.logon_time   = when;
    n.last_update  = when;
    return write_slot(node_no, &n);
}

int nd_action(int node_no, const char *action, pp_u32 when)
{
    pp_node n;
    if (read_slot(node_no, &n) != 0) return 1;
    n.status = ND_ONLINE;
    strncpy(n.action, action, PP_NODE_ACTION_MAX - 1); n.action[PP_NODE_ACTION_MAX - 1] = '\0';
    n.last_update = when;
    return write_slot(node_no, &n);
}

int nd_set_chatok(int node_no, int ok)
{
    pp_node n;
    if (read_slot(node_no, &n) != 0) return 1;
    if (ok) n.flags = (pp_u8)(n.flags | ND_FLAG_CHATOK);
    else    n.flags = (pp_u8)(n.flags & ~ND_FLAG_CHATOK);
    return write_slot(node_no, &n);
}

int nd_release(int node_no, pp_u32 when)
{
    pp_node n;
    if (read_slot(node_no, &n) != 0) return 1;
    n.node_no = (pp_u16)node_no;
    n.status  = ND_IDLE;
    n.flags   = 0;
    n.handle[0]   = '\0';
    n.location[0] = '\0';
    n.action[0]   = '\0';
    n.logon_time  = 0;
    n.last_update = when;
    return write_slot(node_no, &n);
}

int nd_get(int node_no, pp_node *out)
{
    return read_slot(node_no, out);
}

int nd_max(void)
{
    return g_max;
}

int nd_online_count(void)
{
    pp_node n;
    int i, c = 0;
    for (i = 1; i <= g_max; i++) {
        if (read_slot(i, &n) != 0) continue;
        if (n.status == ND_ONLINE || n.status == ND_BETWEEN) c++;
    }
    return c;
}
