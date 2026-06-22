#ifndef PIGPEN_TELNET_H
#define PIGPEN_TELNET_H

/*
 * Server-side telnet codec (DESIGN.md phase-2 scope): option negotiation
 * (WILL ECHO / WILL SGA / WILL/DO BINARY), IAC stripping, CR LF and CR NUL
 * collapsed to a single CR, and outbound 0xFF doubled so CP437 byte 255 is
 * never mistaken for IAC. No NAWS; we assume 80x25 -- it's a BBS.
 *
 * This module is pure byte-stream logic with no transport dependency, so it
 * builds and is unit-tested on the host; io_mtcp wraps it around an mTCP
 * socket. The board is the echoer (WILL ECHO) and runs in character-at-a-time
 * mode (WILL SGA) so hotkeys work without the client buffering a line.
 */
#include "pigtypes.h"

#ifdef __cplusplus
extern "C" {            /* called from the C++ io_mtcp backend */
#endif

/* Telnet command bytes (RFC 854) */
#define TN_SE    240
#define TN_NOP   241
#define TN_SB    250
#define TN_WILL  251
#define TN_WONT  252
#define TN_DO    253
#define TN_DONT  254
#define TN_IAC   255

/* Options we care about */
#define TN_OPT_BINARY 0
#define TN_OPT_ECHO   1
#define TN_OPT_SGA    3

typedef struct {
    int parse;                 /* parser state machine */
    int cmd;                   /* pending WILL/WONT/DO/DONT verb */
    int last_cr;               /* previous data byte was CR (collapse LF/NUL) */
    pp_u8 my[256];             /* options the server has enabled (WILL) */
    pp_u8 his[256];            /* options the client has enabled (its WILL) */
} telnet_t;

/* Reset state. Does not emit; call telnet_greeting for the opening offer. */
void telnet_init(telnet_t *t);

/* Write the server's opening negotiation (WILL ECHO/SGA/BINARY, DO BINARY).
 * out must hold at least 12 bytes; returns the count written. */
int telnet_greeting(telnet_t *t, pp_u8 *out);

/* Process inbound wire bytes. Cleaned user bytes go to user[] (needs >= inlen);
 * any negotiation responses go to reply[] (needs >= 3*inlen, worst case).
 * Returns the user byte count; *reply_len receives the reply byte count. */
int telnet_recv(telnet_t *t, const pp_u8 *in, int inlen,
                pp_u8 *user, pp_u8 *reply, int *reply_len);

/* Encode outbound user bytes for the wire, doubling any 0xFF (IAC).
 * out must hold at least 2*inlen; returns the count written. */
int telnet_send(const pp_u8 *in, int inlen, pp_u8 *out);

#ifdef __cplusplus
}
#endif

#endif /* PIGPEN_TELNET_H */
