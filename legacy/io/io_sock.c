/*
 * io_sock.c -- telnet-over-BSD-sockets backend (host + Win32/Windows 98).
 *
 * The portable network answer. Where io_watt.c talks to a DOS packet driver
 * through Watt-32, this talks to a real OS TCP/IP stack: BSD sockets on a
 * POSIX host, Winsock on Windows (98/ME/2000/XP and up). Same ppio.h the
 * console and Watt-32 backends implement, same portable telnet codec
 * (core/telnet.c) -- swapping io_local/io_watt/io_sock is a link-time choice,
 * exactly the DESIGN.md firewall.
 *
 * This is the backend that needs no DOS, no packet driver, no DPMI -- so it
 * runs (and is testable) on the dev host, and drops onto Windows 98 unchanged
 * for the "Windows BBS with telnet" path. Single caller per process for now
 * (a thread/accept loop is the multinode step).
 *
 * Listen port: $VENDX_PORT, else 2323 (non-privileged, for host testing; a
 * production box would use 23).
 *
 * Build (host): cc -std=c89 ... io/io_sock.c   (link in place of io_local.o)
 * Build (Win32): cl / wcl386 -bt=nt ... -l ws2_32
 */

#if !defined(_WIN32)
#  ifndef _DEFAULT_SOURCE
#    define _DEFAULT_SOURCE 1     /* expose BSD socket APIs under -std=c89 */
#  endif
#endif

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#if defined(_WIN32)
#  include <winsock2.h>
   typedef SOCKET sock_t;
#  define BADSOCK   INVALID_SOCKET
#  define CLOSESOCK closesocket
   typedef int socklen_t;
#else
#  include <sys/types.h>
#  include <sys/socket.h>
#  include <netinet/in.h>
#  include <unistd.h>
   typedef int sock_t;
#  define BADSOCK   (-1)
#  define CLOSESOCK close
#endif

#include "ppio.h"
#include "telnet.h"

#define DEFAULT_PORT 2323
#define IO_BUFSZ     1024
#define OUTBUF       2048

static sock_t   g_listen = BADSOCK;   /* the passive listening socket */
static sock_t   g_conn   = BADSOCK;   /* the current caller's connection */
static telnet_t g_tn;
static int      g_dead;

static pp_u8 g_raw[IO_BUFSZ];         /* raw bytes off the socket */
static pp_u8 g_clean[IO_BUFSZ];       /* telnet-decoded user bytes */
static int   g_clean_len, g_clean_pos;
static pp_u8 g_enc[IO_BUFSZ * 2];     /* telnet-encoded outbound scratch */

static pp_u8 g_out[OUTBUF];           /* output accumulator (one write per byte crawls) */
static int   g_out_n;

/* Push raw wire bytes (already IAC-correct), blocking until all are sent. */
static void wire_raw(const pp_u8 *buf, int len)
{
    int off = 0;
    if (g_conn == BADSOCK) return;
    while (off < len) {
        int rc = (int)send(g_conn, (const char *)(buf + off), len - off, 0);
        if (rc <= 0) { g_dead = 1; return; }
        off += rc;
    }
}

/* Telnet-encode user bytes (CR->CRLF, IAC-escape), then send. */
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
    struct sockaddr_in a;
    int one = 1, port;
    const char *e = getenv("VENDX_PORT");
    port = (e && atoi(e) > 0) ? atoi(e) : DEFAULT_PORT;

#if defined(_WIN32)
    {
        WSADATA w;
        if (WSAStartup(MAKEWORD(2, 2), &w) != 0) return 1;
    }
#endif
    g_listen = socket(AF_INET, SOCK_STREAM, 0);
    if (g_listen == BADSOCK) return 1;
    setsockopt(g_listen, SOL_SOCKET, SO_REUSEADDR, (const char *)&one, sizeof one);

    memset(&a, 0, sizeof a);
    a.sin_family = AF_INET;
    a.sin_addr.s_addr = htonl(INADDR_ANY);
    a.sin_port = htons((unsigned short)port);
    if (bind(g_listen, (struct sockaddr *)&a, sizeof a) != 0) return 1;
    if (listen(g_listen, 1) != 0) return 1;
    fprintf(stderr, "vendetta/x: telnet listening on port %d\n", port);
    return 0;
}

int io_session_begin(void)
{
    pp_u8 greet[16];
    g_conn = accept(g_listen, (struct sockaddr *)0, (socklen_t *)0);
    if (g_conn == BADSOCK) return -1;

    telnet_init(&g_tn);
    g_clean_len = g_clean_pos = 0;
    g_out_n = 0;
    g_dead = 0;
    wire_raw(greet, telnet_greeting(&g_tn, greet));   /* opening IAC negotiation */
    return 0;
}

void io_session_end(void)
{
    out_flush();
    if (g_conn != BADSOCK) { CLOSESOCK(g_conn); g_conn = BADSOCK; }
}

void io_shutdown(void)
{
    if (g_listen != BADSOCK) { CLOSESOCK(g_listen); g_listen = BADSOCK; }
#if defined(_WIN32)
    WSACleanup();
#endif
}

void io_putc(pp_u8 ch)                       { out_byte(ch); }
void io_puts(const char *s)                  { while (*s) out_byte((pp_u8)*s++); }
void io_write(const pp_u8 *buf, pp_u16 len)  { pp_u16 i; for (i = 0; i < len; i++) out_byte(buf[i]); }

int io_getch(void)
{
    out_flush();                 /* the caller must see the prompt before we block */
    for (;;) {
        int rc;
        pp_u8 reply[IO_BUFSZ];
        int rlen = 0;

        if (g_clean_pos < g_clean_len)
            return g_clean[g_clean_pos++];
        if (g_dead || g_conn == BADSOCK)
            return -1;

        rc = (int)recv(g_conn, (char *)g_raw, IO_BUFSZ, 0);
        if (rc <= 0) { g_dead = 1; return -1; }     /* 0 = peer closed, <0 = error */

        g_clean_len = telnet_recv(&g_tn, g_raw, rc, g_clean, reply, &rlen);
        g_clean_pos = 0;
        if (rlen > 0) wire_raw(reply, rlen);        /* negotiation answers (raw IAC) */
    }
}

void io_idle(void) { out_flush(); }

int io_carrier(void) { return (!g_dead && g_conn != BADSOCK) ? 1 : 0; }
