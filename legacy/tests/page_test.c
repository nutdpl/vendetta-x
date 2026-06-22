/* page_test.c -- host unit tests for the inter-node message bus. `make test`. */
#include <stdio.h>
#include <string.h>
#include "page.h"

static int g_fail;
static void check(const char *n, int c) { printf("%-44s %s\n", n, c ? "ok" : "FAIL"); if (!c) g_fail = 1; }

#define TMP "/tmp/pp_page_test.dat"

int main(void)
{
    pp_nodemsg m;
    pp_u8 rec[PP_NMSG_REC];
    pp_u32 cur;
    pp_u32 s1, s2, s3, s4;

    remove(TMP);

    /* ---- create fresh ---- */
    check("open fresh", pg_open(TMP) == 0);
    check("fresh count 0", pg_count() == 0);
    check("fresh seq 0", pg_seq() == 0);

    /* ---- send several; assert monotonic seq starting at 1 ---- */
    s1 = pg_send(2, 1, "alice", PG_CHAT, "hi node 2",  100);
    s2 = pg_send(3, 1, "alice", PG_PAGE, "you there?", 101);
    s3 = pg_send(2, 4, "bob",   PG_CHAT, "yo from 4",  102);
    s4 = pg_send(PG_SYSOP, 2, "carol", PG_PAGE, "sysop please", 103);
    check("seq #1 == 1", s1 == 1);
    check("seq #2 == 2", s2 == 2);
    check("seq #3 == 3", s3 == 3);
    check("seq #4 == 4", s4 == 4);
    check("monotonic", s1 < s2 && s2 < s3 && s3 < s4);
    check("count 4", pg_count() == 4);
    check("seq high 4", pg_seq() == 4);

    /* ---- poll for node 2: should get seq 1 then seq 3 (skipping 2 and 4) ---- */
    cur = 0;
    check("poll n2 hit #1", pg_poll(2, &cur, &m) == 1);
    check("poll n2 seq1", m.seq == 1 && cur == 1);
    check("poll n2 to", m.to_node == 2);
    check("poll n2 from", m.from_node == 1);
    check("poll n2 kind", m.kind == PG_CHAT);
    check("poll n2 handle", strcmp(m.from_handle, "alice") == 0);
    check("poll n2 text", strcmp(m.text, "hi node 2") == 0);
    check("poll n2 when", m.when == 100);

    check("poll n2 hit #2", pg_poll(2, &cur, &m) == 1);
    check("poll n2 seq3", m.seq == 3 && cur == 3);
    check("poll n2 from4", m.from_node == 4);
    check("poll n2 text2", strcmp(m.text, "yo from 4") == 0);

    /* drained for node 2 */
    check("poll n2 drained", pg_poll(2, &cur, &m) == 0);
    check("cursor untouched", cur == 3);

    /* ---- poll for node 3: only seq 2 ---- */
    cur = 0;
    check("poll n3 hit", pg_poll(3, &cur, &m) == 1);
    check("poll n3 seq2", m.seq == 2 && cur == 2 && m.kind == PG_PAGE);
    check("poll n3 drained", pg_poll(3, &cur, &m) == 0);

    /* ---- poll for sysop (node 0): only seq 4 ---- */
    cur = 0;
    check("poll sysop hit", pg_poll(PG_SYSOP, &cur, &m) == 1);
    check("poll sysop seq4", m.seq == 4 && m.to_node == PG_SYSOP);
    check("poll sysop drained", pg_poll(PG_SYSOP, &cur, &m) == 0);

    /* ---- cursor at high seq sees nothing (logon skip-backlog behavior) ---- */
    cur = pg_seq();
    check("poll past-end none", pg_poll(2, &cur, &m) == 0);

    /* ---- persistence across reopen ---- */
    pg_close();
    check("reopen", pg_open(TMP) == 0 && pg_count() == 4 && pg_seq() == 4);
    s1 = pg_send(2, 1, "alice", PG_END, "bye", 104);
    check("seq continues 5", s1 == 5 && pg_count() == 5);
    pg_close();

    /* ---- pack/unpack round-trip at documented offsets ---- */
    memset(&m, 0, sizeof m);
    m.seq = 0x11223344UL;
    m.to_node = 0x0102;
    m.from_node = 0x0304;
    m.kind = PG_ACK;
    strcpy(m.from_handle, "handle-xyz");
    strcpy(m.text, "the quick brown fox");
    m.when = 0x55667788UL;
    pg_pack(&m, rec);

    /* seq u32 @0 little-endian */
    check("off seq", rec[0] == 0x44 && rec[1] == 0x33 && rec[2] == 0x22 && rec[3] == 0x11);
    /* to_node u16 @4 */
    check("off to_node", rec[4] == 0x02 && rec[5] == 0x01);
    /* from_node u16 @6 */
    check("off from_node", rec[6] == 0x04 && rec[7] == 0x03);
    /* kind u8 @8 */
    check("off kind", rec[8] == PG_ACK);
    /* from_handle[32] @9 */
    check("off handle", memcmp(rec + 9, "handle-xyz", 11) == 0);
    /* text[80] @41 */
    check("off text", memcmp(rec + 41, "the quick brown fox", 20) == 0);
    /* when u32 @121 */
    check("off when", rec[121] == 0x88 && rec[122] == 0x77 && rec[123] == 0x66 && rec[124] == 0x55);
    /* padding @125..127 zero */
    check("off pad", rec[125] == 0 && rec[126] == 0 && rec[127] == 0);

    memset(&m, 0, sizeof m);
    pg_unpack(rec, &m);
    check("rt seq", m.seq == 0x11223344UL);
    check("rt to_node", m.to_node == 0x0102);
    check("rt from_node", m.from_node == 0x0304);
    check("rt kind", m.kind == PG_ACK);
    check("rt handle", strcmp(m.from_handle, "handle-xyz") == 0);
    check("rt text", strcmp(m.text, "the quick brown fox") == 0);
    check("rt when", m.when == 0x55667788UL);

    remove(TMP);
    printf("\n%s\n", g_fail ? "TESTS FAILED" : "all page tests passed");
    return g_fail;
}
