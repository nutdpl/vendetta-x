/*
 * io_mtcp.cpp -- telnet-over-mTCP console backend (phase 2; Watcom/DOS only).
 *
 * Implements the ppio.h interface on top of an mTCP TcpSocket and the
 * portable telnet codec (core/telnet.c). mTCP is C++ and GPLv3, so it stays
 * an EXTERNAL dependency behind this backend boundary -- DESIGN.md's
 * architectural firewall. The strict-C89 core never sees it; swapping
 * io_local for io_mtcp is a link-time choice, not a rewrite.
 *
 * The engine emits raw CP437 + ANSI. Outbound we telnet-encode (double any
 * 0xFF so CP437 byte 255 is not read as IAC); inbound we telnet-decode (strip
 * IAC, answer option negotiation, collapse CR LF / CR NUL to CR).
 *
 * Build (DOS, large model to match the core): compile this with Open Watcom
 * C++ (wpp) and -DCFG_H="vxmtcp.cfg", link against the mTCP TCPLIB objects
 * built in the same memory model. See io/vxmtcp.cfg and DESIGN.md.
 */

#include <string.h>
#include <time.h>

#include "types.h"          // mTCP TCPINC
#include "utils.h"
#include "packet.h"
#include "arp.h"
#include "tcp.h"
#include "tcpsockm.h"

extern "C" {
#include "ppio.h"
#include "telnet.h"
}

#define LISTEN_PORT  23
#define IO_BUFSZ     1024
#define IO_IDLE_MAX  90     /* seconds with no input at a prompt -> drop the caller */
#define IO_SEND_TMO  20     /* seconds of zero send progress -> connection is dead */

static TcpSocket *g_sock;
static telnet_t   g_tn;
static int        g_dead;   /* current caller's connection is wedged/dead */

static pp_u8 g_raw[IO_BUFSZ];        // raw bytes off the socket
static pp_u8 g_clean[IO_BUFSZ];      // telnet-decoded user bytes
static int   g_clean_len;
static int   g_clean_pos;
static pp_u8 g_enc[IO_BUFSZ * 2];    // telnet-encoded outbound scratch

/* Service the stack. Must run often; this is the single point that does so. */
static void mtcp_poll(void)
{
    PACKET_PROCESS_SINGLE;
    Arp::driveArp();
    Tcp::drivePackets();
}

/* Push raw wire bytes (already IAC-correct), blocking until all are queued.
 * If send makes no progress for IO_SEND_TMO seconds the peer is gone (and not
 * surfacing a FIN, e.g. over slirp): mark the connection dead and bail, so a
 * caller who drops mid-output can't wedge the node forever. */
static void wire_raw(const pp_u8 *buf, int len)
{
    int off = 0;
    time_t start = time((time_t *)0);
    while (off < len) {
        int16_t rc;
        mtcp_poll();
        if (g_dead || !g_sock || g_sock->isRemoteClosed()) return;
        rc = g_sock->send((uint8_t *)(buf + off), (uint16_t)(len - off));
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

/* Output buffering. The renderer emits a byte at a time; sending each byte as
 * its own TCP write (with a poll between) makes a full screen crawl. So we
 * accumulate output and flush it in big chunks -- on a full buffer, before we
 * wait for input, and on idle/hang-up. */
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
    /* Bring the stack up ONCE. initStack/endStack must not be re-run per
     * caller (the second init after an endStack does not recover), so the
     * serve loop listens/accepts in io_session_begin, not here. */
    if (Utils::parseEnv() != 0) return 1;
    if (Utils::initStack(2, TCP_SOCKET_RING_SIZE, 0, 0)) return 1;
    return 0;
}

int io_session_begin(void)
{
    pp_u8 greet[16];
    TcpSocket *lsock = TcpSocketMgr::getSocket();
    if (lsock == 0) return -1;
    lsock->listen(LISTEN_PORT, IO_BUFSZ);

    for (;;) {                                   /* block until a caller connects */
        mtcp_poll();
        g_sock = TcpSocketMgr::accept();
        if (g_sock != 0) {
            lsock->close();
            TcpSocketMgr::freeSocket(lsock);
            break;
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
    out_flush();                 /* push any pending output (e.g. the goodbye) */
    if (g_sock != 0) {
        g_sock->close();
        TcpSocketMgr::freeSocket(g_sock);
        g_sock = 0;
    }
}

void io_shutdown(void)
{
    Utils::endStack();
}

void io_putc(pp_u8 ch)                       { out_byte(ch); }
void io_puts(const char *s)                  { while (*s) out_byte((pp_u8)*s++); }
void io_write(const pp_u8 *buf, pp_u16 len)  { pp_u16 i; for (i = 0; i < len; i++) out_byte(buf[i]); }

int io_getch(void)
{
    /* A caller who has gone away must not wedge the node. A graceful FIN flips
     * the socket to CLOSE_WAIT (isRemoteClosed) -- prompt on real hardware, but
     * over slirp the close often isn't delivered and mTCP's send ring even
     * accepts data to the dead peer. The reliable signal is silence: a live
     * caller eventually types, a dead one never does. So drop the caller after
     * IO_IDLE_MAX seconds with no input at a prompt. (g_dead, set by a stuck
     * send, ends it sooner.) */
    time_t last = time((time_t *)0);

    out_flush();                 /* the caller must see the prompt before we wait */

    for (;;) {
        if (g_clean_pos < g_clean_len)
            return g_clean[g_clean_pos++];

        mtcp_poll();
        if (g_dead || g_sock == 0 || g_sock->isRemoteClosed())
            return -1;

        {
            int16_t rc = g_sock->recv(g_raw, IO_BUFSZ);
            if (rc > 0) {
                pp_u8 reply[IO_BUFSZ];           /* reply_len <= inlen */
                int rlen = 0;
                g_clean_len = telnet_recv(&g_tn, g_raw, rc, g_clean, reply, &rlen);
                g_clean_pos = 0;
                if (rlen > 0) wire_raw(reply, rlen);   /* negotiation answers (raw IAC) */
                last = time((time_t *)0);
            } else if (rc < 0) {
                g_dead = 1;
                return -1;
            } else if (time((time_t *)0) - last >= IO_IDLE_MAX) {
                g_dead = 1;                       /* inactivity: mark dead so the session
                                                    unwinds at once, not prompt by prompt */
                return -1;
            }
        }
    }
}

void io_idle(void) { out_flush(); mtcp_poll(); }

int io_carrier(void) { return (g_sock != 0 && !g_dead && !g_sock->isRemoteClosed()) ? 1 : 0; }
