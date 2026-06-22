/* node_test.c -- host unit tests for the multinode presence table. `make test`. */
#include <stdio.h>
#include <string.h>
#include "node.h"

static int g_fail;
static void check(const char *n, int c) { printf("%-44s %s\n", n, c ? "ok" : "FAIL"); if (!c) g_fail = 1; }

#define TMP "/tmp/pp_node_test.dat"

int main(void)
{
    pp_node n;
    pp_u8 rec[PP_NODE_REC];
    int i, all_idle;

    remove(TMP);

    /* create fresh -- every slot idle, node_no set to its 1-based number */
    check("open fresh", nd_open(TMP) == 0);
    check("max == PP_MAX_NODES", nd_max() == PP_MAX_NODES);
    check("fresh online_count 0", nd_online_count() == 0);

    all_idle = 1;
    for (i = 1; i <= PP_MAX_NODES; i++) {
        if (nd_get(i, &n) != 0) { all_idle = 0; break; }
        if (n.node_no != (pp_u16)i || n.status != ND_IDLE ||
            n.flags != 0 || n.handle[0] != '\0' || n.location[0] != '\0' ||
            n.action[0] != '\0' || n.logon_time != 0 || n.last_update != 0)
            { all_idle = 0; break; }
    }
    check("all slots idle on create", all_idle);

    /* claim a node */
    check("claim node 3", nd_claim(3, "pigboss", "Texas", 1000) == 0);
    check("get claimed node 3",
        nd_get(3, &n) == 0 && n.node_no == 3 && n.status == ND_BETWEEN &&
        strcmp(n.handle, "pigboss") == 0 && strcmp(n.location, "Texas") == 0 &&
        n.logon_time == 1000 && n.last_update == 1000 && n.action[0] == '\0');
    check("claim bumps online_count", nd_online_count() == 1);

    /* action update -> ND_ONLINE */
    check("action node 3", nd_action(3, "Reading messages", 1050) == 0);
    check("get action node 3",
        nd_get(3, &n) == 0 && n.status == ND_ONLINE &&
        strcmp(n.action, "Reading messages") == 0 && n.last_update == 1050 &&
        strcmp(n.handle, "pigboss") == 0 && n.logon_time == 1000);
    check("online_count still 1", nd_online_count() == 1);

    /* chatok toggle */
    check("set chatok on", nd_set_chatok(3, 1) == 0);
    check("chatok flag set", nd_get(3, &n) == 0 && (n.flags & ND_FLAG_CHATOK));
    check("set chatok off", nd_set_chatok(3, 0) == 0);
    check("chatok flag clear", nd_get(3, &n) == 0 && !(n.flags & ND_FLAG_CHATOK));

    /* a second node online */
    check("claim node 7", nd_claim(7, "lurker", "NY", 2000) == 0);
    check("online_count 2", nd_online_count() == 2);

    /* release back to idle */
    check("release node 3", nd_release(3, 1100) == 0);
    check("get released node 3",
        nd_get(3, &n) == 0 && n.node_no == 3 && n.status == ND_IDLE &&
        n.flags == 0 && n.handle[0] == '\0' && n.location[0] == '\0' &&
        n.action[0] == '\0' && n.logon_time == 0 && n.last_update == 1100);
    check("online_count back to 1", nd_online_count() == 1);

    /* bounds checking */
    check("get node 0 fails", nd_get(0, &n) != 0);
    check("get node max+1 fails", nd_get(PP_MAX_NODES + 1, &n) != 0);
    check("claim node 0 fails", nd_claim(0, "x", "y", 1) != 0);
    check("action node max+1 fails", nd_action(PP_MAX_NODES + 1, "z", 1) != 0);
    check("release node 0 fails", nd_release(0, 1) != 0);
    check("chatok node max+1 fails", nd_set_chatok(PP_MAX_NODES + 1, 1) != 0);

    /* persistence across reopen */
    nd_close();
    check("reopen persists max", nd_open(TMP) == 0 && nd_max() == PP_MAX_NODES);
    check("reopen node 7 online", nd_get(7, &n) == 0 && n.status == ND_BETWEEN &&
        strcmp(n.handle, "lurker") == 0);
    nd_close();

    /* pack -> unpack round trip, with exact byte-offset assertions */
    memset(&n, 0, sizeof n);
    n.node_no     = 0x1234;
    n.status      = ND_ONLINE;
    n.flags       = ND_FLAG_CHATOK;
    strcpy(n.handle,   "alpha");
    strcpy(n.location, "Mars");
    strcpy(n.action,   "At main menu");
    n.logon_time  = 0xAABBCCDDUL;
    n.last_update = 0x11223344UL;
    nd_pack(&n, rec);

    /* node_no u16 @0 little-endian */
    check("pack node_no @0", rec[0] == 0x34 && rec[1] == 0x12);
    /* status @2, flags @3 */
    check("pack status @2", rec[2] == ND_ONLINE);
    check("pack flags @3", rec[3] == ND_FLAG_CHATOK);
    /* handle @4 */
    check("pack handle @4", memcmp(rec + 4, "alpha", 6) == 0);
    /* location @36 */
    check("pack location @36", memcmp(rec + 36, "Mars", 5) == 0);
    /* action @60 */
    check("pack action @60", memcmp(rec + 60, "At main menu", 13) == 0);
    /* logon_time u32 @100 little-endian */
    check("pack logon_time @100",
        rec[100] == 0xDD && rec[101] == 0xCC && rec[102] == 0xBB && rec[103] == 0xAA);
    /* last_update u32 @104 little-endian */
    check("pack last_update @104",
        rec[104] == 0x44 && rec[105] == 0x33 && rec[106] == 0x22 && rec[107] == 0x11);
    /* pad bytes @108..111 zeroed */
    check("pack pad @108 zero",
        rec[108] == 0 && rec[109] == 0 && rec[110] == 0 && rec[111] == 0);

    {
        pp_node m;
        nd_unpack(rec, &m);
        check("roundtrip node_no", m.node_no == 0x1234);
        check("roundtrip status", m.status == ND_ONLINE);
        check("roundtrip flags", m.flags == ND_FLAG_CHATOK);
        check("roundtrip handle", strcmp(m.handle, "alpha") == 0);
        check("roundtrip location", strcmp(m.location, "Mars") == 0);
        check("roundtrip action", strcmp(m.action, "At main menu") == 0);
        check("roundtrip logon_time", m.logon_time == 0xAABBCCDDUL);
        check("roundtrip last_update", m.last_update == 0x11223344UL);
    }

    remove(TMP);
    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all node tests passed");
    return g_fail;
}
