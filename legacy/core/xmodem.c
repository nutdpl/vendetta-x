/*
 * xmodem.c -- XMODEM / XMODEM-1K / YMODEM codec, transport-abstracted.
 *
 * Framing (published de-facto standard; clean-room from spec):
 *   block = [SOH|STX] seq ~seq data[128|1024] trailer
 *     SOH (0x01) marks a 128-byte data block, STX (0x02) a 1024-byte block.
 *     seq   = blocknum & 0xFF, starting at 1 for the first data block.
 *     ~seq  = 255 - seq.
 *     trailer = 1 arithmetic checksum byte (sum of data, mod 256), OR
 *               2 CRC-16 bytes (CCITT poly 0x1021, init 0, big-endian).
 *   End of transfer: EOT (0x04), acknowledged by ACK.
 *   Abort: CAN (0x18) sent twice.
 *
 * The receiver picks the mode: it sends 'C' (0x43) to request CRC, or NAK
 * (0x15) for plain checksum, and keeps poking until the sender's first block
 * arrives. allow_1k uses 1024-byte STX blocks but only in CRC mode (the
 * 1-byte checksum is too weak for 1K blocks, per the XMODEM-1K convention).
 *
 * The final data block is padded to a full 128/1024 bytes with 0x1A (^Z).
 * XMODEM carries no length, so on receive the trailing ^Z padding remains in
 * the output buffer -- callers that need an exact length must strip it (this
 * is correct/expected XMODEM behaviour and is documented at *outlen below).
 *
 * All I/O goes through the caller's get/put callbacks; no transport is
 * assumed, so the same code drives a telnet socket and an in-memory pipe.
 */
#include "xmodem.h"

#define XM_CTRLZ   0x1A
#define XM_MAX_RETRY 10
#define XM_RECV_TMO  3000   /* ms; meaningful only for a real transport */

/* ---- pure frame helpers ------------------------------------------------- */

pp_u16 xm_crc16(const pp_u8 *data, int len)
{
    pp_u16 crc;
    int    i;
    int    b;

    crc = 0;
    for (i = 0; i < len; i++) {
        crc = (pp_u16)(crc ^ ((pp_u16)data[i] << 8));
        for (b = 0; b < 8; b++) {
            if (crc & 0x8000) {
                crc = (pp_u16)((crc << 1) ^ 0x1021);
            } else {
                crc = (pp_u16)(crc << 1);
            }
        }
    }
    return crc;
}

int xm_build_block(int use_crc, int blocknum, const pp_u8 *data, int blocklen, pp_u8 *out)
{
    int   n;
    pp_u8 seq;

    n = 0;
    out[n++] = (pp_u8)((blocklen == 1024) ? XM_STX : XM_SOH);
    seq = (pp_u8)(blocknum & 0xFF);
    out[n++] = seq;
    out[n++] = (pp_u8)(255 - seq);
    {
        int i;
        for (i = 0; i < blocklen; i++) out[n++] = data[i];
    }
    if (use_crc) {
        pp_u16 crc = xm_crc16(data, blocklen);
        out[n++] = (pp_u8)((crc >> 8) & 0xFF);   /* big-endian */
        out[n++] = (pp_u8)(crc & 0xFF);
    } else {
        pp_u8 sum = 0;
        int   i;
        for (i = 0; i < blocklen; i++) sum = (pp_u8)(sum + data[i]);
        out[n++] = sum;
    }
    return n;
}

int xm_check_block(int use_crc, const pp_u8 *body, int blocklen,
                   int *blocknum, pp_u8 *data_out)
{
    pp_u8       seq;
    pp_u8       nseq;
    const pp_u8 *data;
    int          i;

    seq  = body[0];
    nseq = body[1];
    if ((pp_u8)(seq + nseq) != 0xFF) return 0;   /* seq + ~seq must == 255 */

    data = body + 2;
    if (use_crc) {
        pp_u16 want;
        pp_u16 got;
        want = xm_crc16(data, blocklen);
        got  = (pp_u16)(((pp_u16)data[blocklen] << 8) | data[blocklen + 1]);
        if (want != got) return 0;
    } else {
        pp_u8 sum = 0;
        for (i = 0; i < blocklen; i++) sum = (pp_u8)(sum + data[i]);
        if (sum != data[blocklen]) return 0;
    }

    for (i = 0; i < blocklen; i++) data_out[i] = data[i];
    *blocknum = seq;
    return 1;
}

/* ---- send --------------------------------------------------------------- */

static void send_cancel(xm_put_fn put, void *io)
{
    put(io, (pp_u8)XM_CAN);
    put(io, (pp_u8)XM_CAN);
    put(io, (pp_u8)XM_CAN);
}

int xm_send(xm_get_fn get, xm_put_fn put, void *io,
            const pp_u8 *data, long len, int allow_1k)
{
    int  use_crc;
    int  c;
    int  tries;
    long pos;
    int  blocknum;
    pp_u8 block[3 + 1024 + 2];
    pp_u8 chunk[1024];

    /* Wait for the receiver to announce its mode: 'C' = CRC, NAK = checksum. */
    use_crc = -1;
    for (tries = 0; tries < XM_MAX_RETRY; tries++) {
        c = get(io, XM_RECV_TMO);
        if (c < 0) continue;                 /* timeout: keep waiting */
        if (c == XM_C)   { use_crc = 1; break; }
        if (c == XM_NAK) { use_crc = 0; break; }
        if (c == XM_CAN) { return XM_ERR_CANCEL; }
    }
    if (use_crc < 0) return XM_ERR_RETRY;

    blocknum = 1;
    pos = 0;
    while (pos < len) {
        int  blocklen;
        int  i;
        long remain;
        int  nbuilt;

        remain = len - pos;
        /* 1K blocks only when allowed AND in CRC mode and enough data left. */
        if (allow_1k && use_crc && remain >= 1024) {
            blocklen = 1024;
        } else {
            blocklen = 128;
        }
        for (i = 0; i < blocklen; i++) {
            chunk[i] = (pos + i < len) ? data[pos + i] : (pp_u8)XM_CTRLZ;
        }
        nbuilt = xm_build_block(use_crc, blocknum, chunk, blocklen, block);

        for (tries = 0; tries < XM_MAX_RETRY; tries++) {
            for (i = 0; i < nbuilt; i++) put(io, block[i]);
            c = get(io, XM_RECV_TMO);
            if (c < 0)        continue;               /* timeout: resend */
            if (c == XM_ACK)  break;
            if (c == XM_CAN)  { return XM_ERR_CANCEL; }
            /* NAK or anything else: resend */
        }
        if (tries >= XM_MAX_RETRY) { send_cancel(put, io); return XM_ERR_RETRY; }

        pos += blocklen;
        blocknum++;
    }

    /* End of transmission: send EOT, expect ACK (NAK -> resend EOT). */
    for (tries = 0; tries < XM_MAX_RETRY; tries++) {
        put(io, (pp_u8)XM_EOT);
        c = get(io, XM_RECV_TMO);
        if (c < 0)       continue;
        if (c == XM_ACK) return XM_OK;
        if (c == XM_CAN) return XM_ERR_CANCEL;
    }
    return XM_ERR_RETRY;
}

/* ---- receive ------------------------------------------------------------ */

int xm_recv(xm_get_fn get, xm_put_fn put, void *io,
            pp_u8 *buf, long maxlen, long *outlen)
{
    int   use_crc;
    int   c;
    int   tries;
    int   expect;        /* next expected seq, 0..255 wrap */
    long  pos;
    pp_u8 body[2 + 1024 + 2];
    pp_u8 data[1024];

    *outlen = 0;
    pos = 0;

    /* Kick the sender: try 'C' (CRC) a few times, then fall back to NAK. */
    use_crc = 1;
    for (tries = 0; tries < XM_MAX_RETRY; tries++) {
        if (tries < 4) {
            put(io, (pp_u8)(use_crc ? XM_C : XM_NAK));
        } else {
            use_crc = 0;
            put(io, (pp_u8)XM_NAK);
        }
        c = get(io, XM_RECV_TMO);
        if (c == XM_SOH || c == XM_STX) goto have_header;
        if (c == XM_EOT) {                       /* empty transfer */
            put(io, (pp_u8)XM_ACK);
            *outlen = 0;
            return XM_OK;
        }
        if (c == XM_CAN) return XM_ERR_CANCEL;
    }
    return XM_ERR_RETRY;

have_header:
    expect = 1;
    for (;;) {
        int blocklen;
        int blocknum;
        int trailer;
        int need;
        int i;
        int ok;

        /* c holds the current header byte (SOH/STX) at this point. */
        if (c == XM_EOT) {
            put(io, (pp_u8)XM_ACK);
            *outlen = pos;
            return XM_OK;
        }
        if (c == XM_CAN) return XM_ERR_CANCEL;
        if (c != XM_SOH && c != XM_STX) {
            /* noise: ask for a resend and read the next header */
            put(io, (pp_u8)XM_NAK);
            c = get(io, XM_RECV_TMO);
            continue;
        }

        blocklen = (c == XM_STX) ? 1024 : 128;
        trailer  = use_crc ? 2 : 1;
        need     = 2 + blocklen + trailer;       /* seq, ~seq, data, trailer */

        ok = 1;
        for (i = 0; i < need; i++) {
            int b = get(io, XM_RECV_TMO);
            if (b < 0) { ok = 0; break; }        /* short block / timeout */
            body[i] = (pp_u8)b;
        }
        if (!ok) {
            put(io, (pp_u8)XM_NAK);
            c = get(io, XM_RECV_TMO);
            continue;
        }

        if (!xm_check_block(use_crc, body, blocklen, &blocknum, data)) {
            put(io, (pp_u8)XM_NAK);
            c = get(io, XM_RECV_TMO);
            continue;
        }

        if (blocknum == expect) {
            /* New, valid block. Refuse it only if it would overflow. */
            if (pos + blocklen > maxlen) {
                send_cancel(put, io);
                return XM_ERR_IO;
            }
            for (i = 0; i < blocklen; i++) buf[pos + i] = data[i];
            pos += blocklen;
            expect = (expect + 1) & 0xFF;
            put(io, (pp_u8)XM_ACK);
        } else if (blocknum == ((expect - 1) & 0xFF)) {
            /* Sender missed our ACK and resent the previous block: re-ACK. */
            put(io, (pp_u8)XM_ACK);
        } else {
            /* Wrong sequence entirely: protocol desync, abort. */
            send_cancel(put, io);
            return XM_ERR_IO;
        }

        c = get(io, XM_RECV_TMO);                /* next header byte */
    }
}
