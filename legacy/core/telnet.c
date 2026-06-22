/*
 * telnet.c -- server-side telnet codec. Pure logic, no transport (host-tested).
 *
 * Negotiation is loop-safe: a reply is emitted only when an option's state
 * actually changes (RFC 854). The server offers ECHO, SGA and BINARY and
 * agrees to BINARY in both directions; every other option is refused once.
 */
#include "telnet.h"

/* parser states */
enum { P_DATA = 0, P_IAC, P_OPT, P_SB, P_SB_IAC };

void telnet_init(telnet_t *t)
{
    int i;
    t->parse = P_DATA;
    t->cmd = 0;
    t->last_cr = 0;
    for (i = 0; i < 256; i++) { t->my[i] = 0; t->his[i] = 0; }
}

static int want_will(int opt)
{
    return opt == TN_OPT_ECHO || opt == TN_OPT_SGA || opt == TN_OPT_BINARY;
}

static int emit_cmd(pp_u8 *r, int n, int verb, int opt)
{
    r[n++] = TN_IAC;
    r[n++] = (pp_u8)verb;
    r[n++] = (pp_u8)opt;
    return n;
}

int telnet_greeting(telnet_t *t, pp_u8 *out)
{
    int n = 0;
    t->my[TN_OPT_ECHO]   = 1; n = emit_cmd(out, n, TN_WILL, TN_OPT_ECHO);
    t->my[TN_OPT_SGA]    = 1; n = emit_cmd(out, n, TN_WILL, TN_OPT_SGA);
    t->my[TN_OPT_BINARY] = 1; n = emit_cmd(out, n, TN_WILL, TN_OPT_BINARY);
    n = emit_cmd(out, n, TN_DO, TN_OPT_BINARY);   /* ask client for 8-bit too */
    return n;
}

/* React to a negotiation verb; append any state-changing reply to reply[]. */
static int negotiate(telnet_t *t, int verb, int opt, pp_u8 *reply, int n)
{
    switch (verb) {
    case TN_DO:                                      /* my[opt]: 0=off 1=will 2=wont */
        if (want_will(opt)) {
            if (t->my[opt] != 1) { t->my[opt] = 1; n = emit_cmd(reply, n, TN_WILL, opt); }
        } else {
            if (t->my[opt] != 2) { t->my[opt] = 2; n = emit_cmd(reply, n, TN_WONT, opt); }
        }
        break;
    case TN_DONT:
        if (t->my[opt] == 1) { t->my[opt] = 0; n = emit_cmd(reply, n, TN_WONT, opt); }
        break;
    case TN_WILL:
        if (opt == TN_OPT_BINARY) {
            if (!t->his[opt]) { t->his[opt] = 1; n = emit_cmd(reply, n, TN_DO, opt); }
        } else {
            if (t->his[opt] != 2) { t->his[opt] = 2; n = emit_cmd(reply, n, TN_DONT, opt); }
        }
        break;
    case TN_WONT:
        if (t->his[opt] == 1) { t->his[opt] = 0; n = emit_cmd(reply, n, TN_DONT, opt); }
        break;
    default:
        break;
    }
    return n;
}

int telnet_recv(telnet_t *t, const pp_u8 *in, int inlen,
                pp_u8 *user, pp_u8 *reply, int *reply_len)
{
    int i, u = 0, r = 0;

    for (i = 0; i < inlen; i++) {
        int c = in[i];

        switch (t->parse) {
        case P_DATA:
            if (c == TN_IAC) { t->parse = P_IAC; break; }
            /* CR LF / CR NUL -> single CR */
            if (t->last_cr) {
                t->last_cr = 0;
                if (c == '\n' || c == 0) break;       /* swallow the pair byte */
            }
            if (c == '\r') { t->last_cr = 1; user[u++] = '\r'; break; }
            user[u++] = (pp_u8)c;
            break;

        case P_IAC:
            if (c == TN_IAC) { user[u++] = TN_IAC; t->parse = P_DATA; }  /* escaped 0xFF */
            else if (c == TN_WILL || c == TN_WONT || c == TN_DO || c == TN_DONT) {
                t->cmd = c; t->parse = P_OPT;
            } else if (c == TN_SB) {
                t->parse = P_SB;
            } else {
                t->parse = P_DATA;                    /* NOP/GA/etc: ignore */
            }
            break;

        case P_OPT:
            r = negotiate(t, t->cmd, c, reply, r);
            t->parse = P_DATA;
            break;

        case P_SB:                                    /* skip subnegotiation body */
            if (c == TN_IAC) t->parse = P_SB_IAC;
            break;

        case P_SB_IAC:
            t->parse = (c == TN_SE) ? P_DATA : P_SB;
            break;

        default:
            t->parse = P_DATA;
            break;
        }
    }

    *reply_len = r;
    return u;
}

int telnet_send(const pp_u8 *in, int inlen, pp_u8 *out)
{
    int i, n = 0;
    for (i = 0; i < inlen; i++) {
        out[n++] = in[i];
        if (in[i] == TN_IAC) out[n++] = TN_IAC;       /* double 0xFF */
    }
    return n;
}
