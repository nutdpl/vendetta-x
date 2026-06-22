/*
 * config_test.c -- host unit tests for board CONFIG.DAT persistence.
 */
#include <stdio.h>
#include <string.h>
#include "config.h"

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define TMP "/tmp/pigpen_cfgtest.dat"

int main(void)
{
    /* 1. record size is the documented 128 bytes */
    check("PP_CONFIG_REC == 256", PP_CONFIG_REC == 256);

    /* 2. defaults are sensible */
    {
        pp_config d;
        cfg_defaults(&d);
        check("default board_name", strcmp(d.board_name, "Vendetta/X") == 0);
        check("default sysop_name", strcmp(d.sysop_name, "dan") == 0);
        check("default new_user_sl 10",  d.new_user_sl == 10);
        check("default new_user_dsl 10", d.new_user_dsl == 10);
        check("default not closed", (d.flags & PP_CFG_CLOSED_SYSTEM) == 0);
        check("default allows aliases", (d.flags & PP_CFG_ALLOW_ALIASES) != 0);
        check("default max_msgs > 0", d.max_msgs > 0);
        check("default idle_minutes > 0", d.idle_minutes > 0);
    }

    /* 3. pack/unpack round-trips every field, little-endian on disk */
    {
        pp_config a, b;
        pp_u8 rec[PP_CONFIG_REC];
        memset(&a, 0, sizeof a);
        strcpy(a.board_name, "Pirate's Cove");
        strcpy(a.sysop_name, "Zero Cool");
        strcpy(a.location,   "NYC");
        a.new_user_sl = 20; a.new_user_dsl = 30;
        a.flags = PP_CFG_CLOSED_SYSTEM | PP_CFG_ALLOW_ALIASES;
        a.max_msgs = 1000; a.idle_minutes = 25;
        a.net_card = 2; a.net_dhcp = 0; a.pkt_int = 0x62; a.telnet_port = 2323;
        strcpy(a.ip, "10.0.0.5"); strcpy(a.netmask, "255.0.0.0");
        strcpy(a.gateway, "10.0.0.1"); strcpy(a.dns, "8.8.8.8");
        cfg_pack(&a, rec);
        cfg_unpack(rec, &b);
        check("roundtrip board_name", strcmp(a.board_name, b.board_name) == 0);
        check("roundtrip sysop_name", strcmp(a.sysop_name, b.sysop_name) == 0);
        check("roundtrip location",   strcmp(a.location, b.location) == 0);
        check("roundtrip sl/dsl", a.new_user_sl == b.new_user_sl && a.new_user_dsl == b.new_user_dsl);
        check("roundtrip flags",  a.flags == b.flags);
        check("roundtrip max_msgs/idle", a.max_msgs == b.max_msgs && a.idle_minutes == b.idle_minutes);
        check("roundtrip net card/dhcp/int/port",
              a.net_card == b.net_card && a.net_dhcp == b.net_dhcp &&
              a.pkt_int == b.pkt_int && a.telnet_port == b.telnet_port);
        check("roundtrip ip/mask/gw/dns",
              strcmp(a.ip, b.ip) == 0 && strcmp(a.netmask, b.netmask) == 0 &&
              strcmp(a.gateway, b.gateway) == 0 && strcmp(a.dns, b.dns) == 0);
        /* flags 3 = 0x00000003 little-endian at off 108 */
        check("little-endian flags on disk",
              rec[108]==0x03 && rec[109]==0x00 && rec[110]==0x00 && rec[111]==0x00);
        /* max_msgs 1000 = 0x03E8 little-endian at off 112 */
        check("little-endian u16 on disk", rec[112]==0xE8 && rec[113]==0x03);
    }

    /* 4. save / load roundtrip across files */
    remove(TMP);
    {
        pp_config out, in;
        memset(&out, 0, sizeof out);
        strcpy(out.board_name, "Vendetta/X");
        strcpy(out.sysop_name, "dan");
        strcpy(out.location,   "Hog Heaven");
        out.new_user_sl = 15; out.new_user_dsl = 5;
        out.flags = PP_CFG_CLOSED_SYSTEM;
        out.max_msgs = 750; out.idle_minutes = 12;
        check("save ok", cfg_save(TMP, &out) == 0);

        memset(&in, 0xAA, sizeof in);
        check("load ok", cfg_load(TMP, &in) == 0);
        check("load keeps board_name", strcmp(in.board_name, "Vendetta/X") == 0);
        check("load keeps sysop_name", strcmp(in.sysop_name, "dan") == 0);
        check("load keeps location",   strcmp(in.location, "Hog Heaven") == 0);
        check("load keeps sl/dsl", in.new_user_sl == 15 && in.new_user_dsl == 5);
        check("load keeps flags",  in.flags == PP_CFG_CLOSED_SYSTEM);
        check("load keeps max_msgs/idle", in.max_msgs == 750 && in.idle_minutes == 12);
    }
    remove(TMP);

    /* 5. load of a missing file yields defaults and still returns 0 */
    {
        pp_config in, d;
        cfg_defaults(&d);
        memset(&in, 0x55, sizeof in);
        check("load missing returns 0", cfg_load(TMP, &in) == 0);
        check("load missing -> defaults board", strcmp(in.board_name, d.board_name) == 0);
        check("load missing -> defaults sl", in.new_user_sl == d.new_user_sl);
        check("load missing -> defaults flags", in.flags == d.flags);
    }

    /* 6. load of a damaged file (bad magic) yields defaults, returns 0 */
    {
        FILE *f;
        pp_config in, d;
        cfg_defaults(&d);
        f = fopen(TMP, "wb");
        if (f) { fputs("garbage not a real header at all here", f); fclose(f); }
        memset(&in, 0x33, sizeof in);
        check("load damaged returns 0", cfg_load(TMP, &in) == 0);
        check("load damaged -> defaults", strcmp(in.board_name, d.board_name) == 0);
        remove(TMP);
    }

    /* 7. NULL cfg is the one error case */
    check("load NULL cfg fails", cfg_load(TMP, (pp_config *)0) != 0);
    check("save NULL cfg fails", cfg_save(TMP, (const pp_config *)0) != 0);
    remove(TMP);

    /* 8. NIC presets + WATTCP.CFG generation */
    {
        pp_config c;
        char buf[1024];
        FILE *f;
        size_t n;
        cfg_defaults(&c);
        check("card 0 name", strstr(cfg_card_name(0), "NE2000") != (char *)0);
        check("card 0 driver", strcmp(cfg_card_driver(0), "NE2000.COM") == 0);
        check("card out-of-range safe", cfg_card_name(99)[0] != '\0');

        /* DHCP config */
        check("write wattcp dhcp", cfg_write_wattcp(TMP, &c) == 0);
        f = fopen(TMP, "rb"); n = f ? fread(buf, 1, sizeof buf - 1, f) : 0;
        if (f) fclose(f);
        buf[n] = '\0';
        check("wattcp has MY_IP = DHCP", strstr(buf, "MY_IP = DHCP") != (char *)0);
        check("wattcp names the card", strstr(buf, "NE2000") != (char *)0);
        check("wattcp names the driver+vector", strstr(buf, "NE2000.COM at int 0x60") != (char *)0);

        /* static config */
        c.net_dhcp = 0; strcpy(c.ip, "10.0.0.5"); strcpy(c.gateway, "10.0.0.1");
        check("write wattcp static", cfg_write_wattcp(TMP, &c) == 0);
        f = fopen(TMP, "rb"); n = f ? fread(buf, 1, sizeof buf - 1, f) : 0;
        if (f) fclose(f);
        buf[n] = '\0';
        check("wattcp static MY_IP", strstr(buf, "MY_IP = 10.0.0.5") != (char *)0);
        check("wattcp static GATEWAY", strstr(buf, "GATEWAY = 10.0.0.1") != (char *)0);
        check("wattcp static has no DHCP", strstr(buf, "= DHCP") == (char *)0);
        remove(TMP);
    }

    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all config tests passed");
    return g_fail;
}
