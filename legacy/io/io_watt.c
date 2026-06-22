/*
 * io_watt.c -- telnet-over-Watt-32 backend (VX/32; Open Watcom protected mode).
 *
 * The protected-mode answer to the real-mode mTCP dead end. mTCP is real-mode
 * only and overflows the 64k DGROUP once linked with the engine, so the flat
 * 32-bit board (wcl386 -mf, DOS/4GW) talks to a Watt-32 TCP/IP stack instead.
 * Watt-32 runs in flat protected mode and bridges the real-mode packet-driver
 * callbacks via DPMI; its license is permissive (redistributable, unlike
 * mTCP's GPLv3), so a free Vendetta/X release can link it without copyleft.
 *
 * This implements the same ppio.h the local console and io_mtcp implement, and
 * wraps the same portable telnet codec (core/telnet.c). The strict-C89 engine
 * never sees Watt-32 -- swapping io_local/io_mtcp/io_watt is a link-time choice,
 * exactly the DESIGN.md firewall. Plain C (Watt-32 is C), so no extern "C".
 *
 * Build (DOS, flat model to match the 32-bit engine): compile with wcc386 -mf
 * and -I<watt32>/inc, link against the flat Watt-32 library (wattcpwf.lib).
 * See io/vxwatt.lnk, mkwatt.bat, and DESIGN.md.
 */

#include <string.h>
#include <time.h>

#include <tcp.h>            /* Watt-32: sock_init, tcp_listen, tcp_tick, ... */

#include "ppio.h"
#include "telnet.h"

#define LISTEN_PORT  23
#define IO_BUFSZ     1024
#define IO_IDLE_MAX  90     /* seconds with no input at a prompt -> drop the caller */
#define IO_SEND_TMO  20     /* seconds of zero send progress -> connection is dead */

static tcp_Socket g_sock;
static telnet_t   g_tn;
static int        g_dead;   /* current caller's connection is wedged/dead */
static int        g_open;   /* g_sock currently holds a live/closing connection */

static pp_u8 g_raw[IO_BUFSZ];        /* raw bytes off the socket */
static pp_u8 g_clean[IO_BUFSZ];      /* telnet-decoded user bytes */
static int   g_clean_len;
static int   g_clean_pos;
static pp_u8 g_enc[IO_BUFSZ * 2];    /* telnet-encoded outbound scratch */

/* Drive the stack. tcp_tick services the packet driver, ACKs, retransmits and
 * the FSM; it must run often. Returns 0 once our socket has fully closed. */
static int wstack(void)
{
    return tcp_tick(&g_sock);
}

/* Push raw wire bytes (already IAC-correct), blocking until all are queued.
 * sock_write buffers into the TCP send window; if it makes no progress for
 * IO_SEND_TMO seconds the peer is gone, so we mark the connection dead and bail
 * rather than wedge the node on a caller who dropped mid-output. */
static void wire_raw(const pp_u8 *buf, int len)
{
    int off = 0;
    time_t start = time((time_t *)0);
    while (off < len) {
        int rc;
        if (!wstack() || g_dead) { g_dead = 1; return; }
        rc = sock_write(&g_sock, (const char *)(buf + off), len - off);
        if (rc > 0) { off += rc; start = time((time_t *)0); }
        else if (time((time_t *)0) - start >= IO_SEND_TMO) { g_dead = 1; return; }
    }
}

/* Telnet-encode user bytes, then send. */
static void wire_user(const pp_u8 *buf, int len)
{
    while (len > 0) {
        int chunk = (len > IO_BUFSZ) ? IO_BUFSZ : len;
        int n = telnet_send(buf, chunk, g_enc);
        wire_raw(g_enc, n);
        buf += chunk;
        len -= chunk;
    }
}

/* Output buffering. The renderer emits a byte at a time; one TCP write per byte
 * makes a full screen crawl, so accumulate and flush in big chunks -- on a full
 * buffer, before we wait for input, and on idle/hang-up. */
#define OUTBUF 2048
static pp_u8 g_out[OUTBUF];
static int   g_out_n;

static void out_flush(void)
{
    if (g_out_n > 0) { wire_user(g_out, g_out_n); g_out_n = 0; }
}

static void out_byte(pp_u8 c)
{
    if (g_out_n >= OUTBUF) out_flush();
    g_out[g_out_n++] = c;
}

int io_init(void)
{
    /* Bring the stack up ONCE (reads WATTCP.CFG / does BOOTP-DHCP per config).
     * Nonzero is a boot failure; sock_init_err(rc) has the text. */
    int rc = sock_init();
    if (rc != 0) return 1;
    return 0;
}

int io_session_begin(void)
{
    pp_u8 greet[16];

    /* Passive open: listen on :23 for any peer, no inactivity timeout (we run
     * our own IO_IDLE_MAX). Re-arm if the half-open socket is reset while we
     * wait, so a stray RST can't end the serve loop. */
    if (tcp_listen(&g_sock, LISTEN_PORT, 0UL, 0, (ProtoHandler)0, 0) < 0)
        return -1;
    g_open = 1;

    while (!sock_established(&g_sock)) {
        if (!wstack()) {                              /* listener died -> re-arm */
            if (tcp_listen(&g_sock, LISTEN_PORT, 0UL, 0, (ProtoHandler)0, 0) < 0)
                return -1;
        }
    }

    telnet_init(&g_tn);
    g_clean_len = g_clean_pos = 0;
    g_out_n = 0;
    g_dead = 0;
    wire_raw(greet, telnet_greeting(&g_tn, greet));   /* opening negotiation (raw IAC) */
    return 0;
}

void io_session_end(void)
{
    int spins = 0;
    out_flush();                 /* push any pending output (e.g. the goodbye) */
    if (g_open) {
        sock_close(&g_sock);
        /* let the FIN handshake drain so the client sees a clean disconnect */
        while (wstack() && spins++ < 500)
            ;
        g_open = 0;
    }
}

void io_shutdown(void)
{
    /* Watt-32 unhooks the packet driver at exit via its own atexit handler. */
}

void io_putc(pp_u8 ch)                       { out_byte(ch); }
void io_puts(const char *s)                  { while (*s) out_byte((pp_u8)*s++); }
void io_write(const pp_u8 *buf, pp_u16 len)  { pp_u16 i; for (i = 0; i < len; i++) out_byte(buf[i]); }

int io_getch(void)
{
    /* A caller who has gone away must not wedge the node. tcp_tick returning 0
     * is a clean carrier loss; otherwise the reliable signal is silence -- a
     * live caller eventually types, a dead one never does -- so drop after
     * IO_IDLE_MAX seconds with no input at a prompt. (g_dead, set by a stuck
     * send, ends it sooner.) */
    time_t last = time((time_t *)0);

    out_flush();                 /* the caller must see the prompt before we wait */

    for (;;) {
        if (g_clean_pos < g_clean_len)
            return g_clean[g_clean_pos++];

        if (g_dead || !wstack())
            return -1;

        if (sock_dataready(&g_sock)) {
            int rc = sock_fastread(&g_sock, (char *)g_raw, IO_BUFSZ);
            if (rc > 0) {
                pp_u8 reply[IO_BUFSZ];           /* reply_len <= inlen */
                int rlen = 0;
                g_clean_len = telnet_recv(&g_tn, g_raw, rc, g_clean, reply, &rlen);
                g_clean_pos = 0;
                if (rlen > 0) wire_raw(reply, rlen);   /* negotiation answers (raw IAC) */
                last = time((time_t *)0);
            }
        } else if (time((time_t *)0) - last >= IO_IDLE_MAX) {
            g_dead = 1;                          /* inactivity: unwind the session at once */
            return -1;
        }
    }
}

void io_idle(void) { out_flush(); wstack(); }

int io_carrier(void) { return (!g_dead && wstack() && sock_established(&g_sock)) ? 1 : 0; }
