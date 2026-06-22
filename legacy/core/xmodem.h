#ifndef PIGPEN_XMODEM_H
#define PIGPEN_XMODEM_H

/*
 * XMODEM / XMODEM-1K / YMODEM file-transfer codec.
 *
 * The protocol logic is kept INDEPENDENT of the transport: the send/receive
 * drivers move bytes only through caller-supplied get/put callbacks, so the
 * same code drives a telnet socket on the real board and an in-memory pipe in
 * the unit test. The pure frame helpers (CRC, block build/parse) are exposed
 * separately so they can be tested directly against known vectors.
 *
 * Clean-room: the XMODEM/YMODEM framing is a published de-facto standard;
 * implement it from the spec, copy no code.
 */
#include "pigtypes.h"

/* control bytes */
#define XM_SOH  0x01   /* 128-byte data block header */
#define XM_STX  0x02   /* 1024-byte data block header (XMODEM-1K / YMODEM) */
#define XM_EOT  0x04   /* end of transmission */
#define XM_ACK  0x06
#define XM_NAK  0x15
#define XM_CAN  0x18   /* cancel (sent twice) */
#define XM_C    0x43   /* 'C' -- receiver requests CRC mode */

#define XM_OK        0    /* transfer completed */
#define XM_ERR_IO   -1    /* get/put callback signalled carrier loss */
#define XM_ERR_CANCEL -2  /* peer cancelled */
#define XM_ERR_RETRY -3   /* too many retries / timeout */

/* Transport callbacks. get returns 0..255, or <0 on timeout/carrier loss
 * (the driver treats <0 as a retry trigger / abort). `io` is passed through. */
typedef int  (*xm_get_fn)(void *io, int timeout_ms);
typedef void (*xm_put_fn)(void *io, pp_u8 byte);

/* ---- pure frame helpers (unit-testable without any transport) ---------- */

/* CCITT CRC-16 (poly 0x1021, init 0) over len bytes. */
pp_u16 xm_crc16(const pp_u8 *data, int len);

/* Build one block into `out`. blocklen is 128 or 1024; out holds
 * 3 + blocklen + (crc?2:1) bytes (header, seq, ~seq, data, checksum/CRC).
 * Returns the total byte count written to out. */
int xm_build_block(int use_crc, int blocknum, const pp_u8 *data, int blocklen, pp_u8 *out);

/* Validate a received block body (the bytes AFTER the SOH/STX header byte):
 * seq, ~seq, data[blocklen], checksum-or-crc. Returns 1 if intact (and copies
 * blocklen data bytes to data_out, sets *blocknum), 0 if corrupt. */
int xm_check_block(int use_crc, const pp_u8 *body, int blocklen,
                   int *blocknum, pp_u8 *data_out);

/* ---- transport drivers ------------------------------------------------- */

/* Send len bytes. allow_1k uses 1024-byte blocks when the receiver asks for
 * CRC mode. Returns XM_OK or an XM_ERR_*. */
int xm_send(xm_get_fn get, xm_put_fn put, void *io,
            const pp_u8 *data, long len, int allow_1k);

/* Receive into buf (<= maxlen). On success sets *outlen and returns XM_OK. */
int xm_recv(xm_get_fn get, xm_put_fn put, void *io,
            pp_u8 *buf, long maxlen, long *outlen);

#endif /* PIGPEN_XMODEM_H */
