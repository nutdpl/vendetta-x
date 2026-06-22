#ifndef PIGPEN_CONFIG_H
#define PIGPEN_CONFIG_H

/*
 * Central board configuration (DESIGN.md). CONFIG.DAT is a 16-byte header
 * followed by exactly ONE fixed-size record (count is always 1) -- not an
 * array like the userbase/oneliner. Same little-endian, field-by-field,
 * tool-readable discipline: the record is serialized at known offsets (NOT a
 * raw struct fwrite), so the format does not depend on compiler struct padding
 * and can be read identically under 16-bit Watcom DOS and a modern host.
 * See the offset map in config.c.
 */
#include "pigtypes.h"

#define PP_BOARD_MAX 40     /* board name, incl. NUL */
#define PP_SYSOP_MAX 32     /* sysop handle, incl. NUL */
#define PP_CFGLOC_MAX 32    /* board location, incl. NUL */
#define PP_IP_MAX     16    /* dotted-quad string "255.255.255.255" + NUL */
#define PP_NET_CARDS  7     /* number of known NIC presets (see config.c) */

/* flags bits */
#define PP_CFG_CLOSED_SYSTEM  0x00000001u   /* bit0: new applications refused */
#define PP_CFG_ALLOW_ALIASES  0x00000002u   /* bit1: aliases permitted */

typedef struct {
    char   board_name[PP_BOARD_MAX];
    char   sysop_name[PP_SYSOP_MAX];
    char   location[PP_CFGLOC_MAX];
    pp_u8  new_user_sl;     /* default security level for new callers */
    pp_u8  new_user_dsl;    /* default download security level */
    pp_u32 flags;           /* PP_CFG_* bits */
    pp_u16 max_msgs;        /* per-area message cap */
    pp_u16 idle_minutes;    /* idle timeout before hang-up */
    /* network (Watt-32 / packet driver) -- drives WATTCP.CFG + the telnet port */
    pp_u8  net_card;        /* index into the NIC preset list (0..PP_NET_CARDS-1) */
    pp_u8  net_dhcp;        /* 1 = DHCP, 0 = static IP */
    pp_u8  pkt_int;         /* packet-driver software interrupt (0x60..0x80) */
    pp_u16 telnet_port;     /* board listen port (23 default) */
    char   ip[PP_IP_MAX];       /* static IP (ignored when DHCP) */
    char   netmask[PP_IP_MAX];
    char   gateway[PP_IP_MAX];
    char   dns[PP_IP_MAX];
} pp_config;

/* On-disk record size (exposed for unit tests). */
#define PP_CONFIG_REC 256

/* Display name + packet-driver hint for NIC preset i (0..PP_NET_CARDS-1). */
const char *cfg_card_name(int i);
const char *cfg_card_driver(int i);

/* Write a Watt-32 WATTCP.CFG from *cfg (IP config + a NIC/vector header
 * comment). Returns 0 on success. */
int  cfg_write_wattcp(const char *path, const pp_config *cfg);

/* Fill *cfg with sensible factory defaults. */
void cfg_defaults(pp_config *cfg);

/* Load the single config record from `path` into *cfg. Returns 0 on success.
 * If the file is missing or damaged it FALLS BACK to cfg_defaults() and still
 * returns 0 -- this never leaves the caller without a usable config. A nonzero
 * return is reserved for a NULL cfg argument. */
int  cfg_load(const char *path, pp_config *cfg);

/* Serialize *cfg as the single record into `path` (header + one record),
 * creating the file if absent. Returns 0 on success. */
int  cfg_save(const char *path, const pp_config *cfg);

/* Serialization (exposed for unit tests): record <-> PP_CONFIG_REC bytes. */
void cfg_pack(const pp_config *cfg, pp_u8 *rec);
void cfg_unpack(const pp_u8 *rec, pp_config *cfg);

#endif /* PIGPEN_CONFIG_H */
