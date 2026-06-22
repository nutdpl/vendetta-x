/*
 * config.c -- CONFIG.DAT persistence (a single record, not an array).
 *
 * File layout (all little-endian):
 *   header (16 bytes): "PCFG" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then exactly ONE record of PP_CONFIG_REC (128) bytes:
 *     off  0  board_name[40]   (NUL-padded)
 *     off 40  sysop_name[32]
 *     off 72  location[32]
 *     off 104 u8  new_user_sl
 *     off 105 u8  new_user_dsl
 *     off 106 2 pad
 *     off 108 u32 flags
 *     off 112 u16 max_msgs
 *     off 114 u16 idle_minutes
 *     off 116 12 reserved
 */
#include <stdio.h>
#include <string.h>
#include "config.h"

#define HDR_SIZE   16
#define CFG_VERSION 1

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

/* ---- serialization ------------------------------------------------------ */

void cfg_pack(const pp_config *cfg, pp_u8 *rec)
{
    memset(rec, 0, PP_CONFIG_REC);
    put_str(rec + 0,   cfg->board_name, PP_BOARD_MAX);
    put_str(rec + 40,  cfg->sysop_name, PP_SYSOP_MAX);
    put_str(rec + 72,  cfg->location,   PP_CFGLOC_MAX);
    rec[104] = cfg->new_user_sl;
    rec[105] = cfg->new_user_dsl;
    put_u32(rec + 108, cfg->flags);
    put_u16(rec + 112, cfg->max_msgs);
    put_u16(rec + 114, cfg->idle_minutes);
    rec[116] = cfg->net_card;
    rec[117] = cfg->net_dhcp;
    rec[118] = cfg->pkt_int;
    put_u16(rec + 120, cfg->telnet_port);
    put_str(rec + 122, cfg->ip,      PP_IP_MAX);
    put_str(rec + 138, cfg->netmask, PP_IP_MAX);
    put_str(rec + 154, cfg->gateway, PP_IP_MAX);
    put_str(rec + 170, cfg->dns,     PP_IP_MAX);
}

void cfg_unpack(const pp_u8 *rec, pp_config *cfg)
{
    memset(cfg, 0, sizeof *cfg);
    memcpy(cfg->board_name, rec + 0,  PP_BOARD_MAX - 1);  cfg->board_name[PP_BOARD_MAX - 1]  = '\0';
    memcpy(cfg->sysop_name, rec + 40, PP_SYSOP_MAX - 1);  cfg->sysop_name[PP_SYSOP_MAX - 1]  = '\0';
    memcpy(cfg->location,   rec + 72, PP_CFGLOC_MAX - 1); cfg->location[PP_CFGLOC_MAX - 1]   = '\0';
    cfg->new_user_sl   = rec[104];
    cfg->new_user_dsl  = rec[105];
    cfg->flags         = get_u32(rec + 108);
    cfg->max_msgs      = get_u16(rec + 112);
    cfg->idle_minutes  = get_u16(rec + 114);
    cfg->net_card      = rec[116];
    cfg->net_dhcp      = rec[117];
    cfg->pkt_int       = rec[118];
    cfg->telnet_port   = get_u16(rec + 120);
    memcpy(cfg->ip,      rec + 122, PP_IP_MAX - 1); cfg->ip[PP_IP_MAX - 1]      = '\0';
    memcpy(cfg->netmask, rec + 138, PP_IP_MAX - 1); cfg->netmask[PP_IP_MAX - 1] = '\0';
    memcpy(cfg->gateway, rec + 154, PP_IP_MAX - 1); cfg->gateway[PP_IP_MAX - 1] = '\0';
    memcpy(cfg->dns,     rec + 170, PP_IP_MAX - 1); cfg->dns[PP_IP_MAX - 1]     = '\0';
}

/* ---- defaults ----------------------------------------------------------- */

void cfg_defaults(pp_config *cfg)
{
    memset(cfg, 0, sizeof *cfg);
    strcpy(cfg->board_name, "Vendetta/X");
    strcpy(cfg->sysop_name, "dan");
    strcpy(cfg->location,   "Somewhere, USA");
    cfg->new_user_sl   = 10;
    cfg->new_user_dsl  = 10;
    cfg->flags         = PP_CFG_ALLOW_ALIASES;   /* open system, aliases ok */
    cfg->max_msgs      = 500;
    cfg->idle_minutes  = 15;
    cfg->net_card      = 0;                       /* NE2000 / clone */
    cfg->net_dhcp      = 1;                        /* DHCP by default */
    cfg->pkt_int       = 0x60;                     /* the usual packet vector */
    cfg->telnet_port   = 23;
    strcpy(cfg->ip,      "192.168.1.50");
    strcpy(cfg->netmask, "255.255.255.0");
    strcpy(cfg->gateway, "192.168.1.1");
    strcpy(cfg->dns,     "1.1.1.1");
}

/* ---- NIC presets + WATTCP.CFG generation -------------------------------- */

/* Known cards: display name + the packet-driver TSR you'd typically load for
 * it. Watt-32 itself talks to whatever packet driver sits at pkt_int -- the
 * card choice picks the driver + documents the setup; the vector is what the
 * stack actually uses. */
static const char *const CARD_NAME[PP_NET_CARDS] = {
    "NE2000 / clone",
    "3Com 3C509 (EtherLink III)",
    "Intel EtherExpress Pro",
    "WD / SMC 80x3",
    "3Com 3C503 (EtherLink II)",
    "AMD PCnet / Lance",
    "Generic packet driver"
};
static const char *const CARD_DRIVER[PP_NET_CARDS] = {
    "NE2000.COM", "3C509.COM", "EEPRO.COM", "WD8003E.COM",
    "3C503.COM", "PCNTPK.COM", "PKTDRV.COM"
};

const char *cfg_card_name(int i)
{
    return (i >= 0 && i < PP_NET_CARDS) ? CARD_NAME[i] : "?";
}
const char *cfg_card_driver(int i)
{
    return (i >= 0 && i < PP_NET_CARDS) ? CARD_DRIVER[i] : "PKTDRV.COM";
}

int cfg_write_wattcp(const char *path, const pp_config *cfg)
{
    FILE *f = fopen(path, "wb");
    int card = cfg->net_card;
    if (f == (FILE *)0) return 1;
    fprintf(f, "; WATTCP.CFG -- generated by VXCFG for %s\r\n", cfg->board_name);
    fprintf(f, "; NIC: %s  (load %s at int 0x%02X)\r\n",
            cfg_card_name(card), cfg_card_driver(card), (unsigned)cfg->pkt_int);
    fprintf(f, "; board listens on telnet port %u\r\n", (unsigned)cfg->telnet_port);
    if (cfg->net_dhcp) {
        fprintf(f, "MY_IP = DHCP\r\n");
    } else {
        fprintf(f, "MY_IP = %s\r\n",      cfg->ip);
        fprintf(f, "NETMASK = %s\r\n",    cfg->netmask);
        fprintf(f, "GATEWAY = %s\r\n",    cfg->gateway);
    }
    if (cfg->dns[0]) fprintf(f, "NAMESERVER = %s\r\n", cfg->dns);
    fclose(f);
    return 0;
}

/* ---- load / save -------------------------------------------------------- */

int cfg_load(const char *path, pp_config *cfg)
{
    FILE *f;
    pp_u8 h[HDR_SIZE], rec[PP_CONFIG_REC];

    if (cfg == (pp_config *)0) return 1;

    f = fopen(path, "rb");
    if (f == (FILE *)0) { cfg_defaults(cfg); return 0; }

    if (fread(h, 1, HDR_SIZE, f) != HDR_SIZE ||
        h[0] != 'P' || h[1] != 'C' || h[2] != 'F' || h[3] != 'G' ||
        get_u16(h + 6) != PP_CONFIG_REC ||
        get_u32(h + 8) < 1) {
        fclose(f);
        cfg_defaults(cfg);
        return 0;
    }
    if (fread(rec, 1, PP_CONFIG_REC, f) != PP_CONFIG_REC) {
        fclose(f);
        cfg_defaults(cfg);
        return 0;
    }
    fclose(f);
    cfg_unpack(rec, cfg);
    return 0;
}

int cfg_save(const char *path, const pp_config *cfg)
{
    FILE *f;
    pp_u8 h[HDR_SIZE], rec[PP_CONFIG_REC];

    if (cfg == (const pp_config *)0) return 1;

    f = fopen(path, "wb");
    if (f == (FILE *)0) return 1;

    memset(h, 0, HDR_SIZE);
    h[0] = 'P'; h[1] = 'C'; h[2] = 'F'; h[3] = 'G';
    put_u16(h + 4, CFG_VERSION);
    put_u16(h + 6, PP_CONFIG_REC);
    put_u32(h + 8, 1u);                 /* always exactly one record */
    if (fwrite(h, 1, HDR_SIZE, f) != HDR_SIZE) { fclose(f); return 1; }

    cfg_pack(cfg, rec);
    if (fwrite(rec, 1, PP_CONFIG_REC, f) != PP_CONFIG_REC) { fclose(f); return 1; }

    fclose(f);
    return 0;
}
