#ifndef PIGPEN_IO_H
#define PIGPEN_IO_H

/*
 * IO backend interface (DESIGN.md: getch/putch/idle/carrier-check).
 * Named ppio.h, not io.h, so it never shadows the C library's <io.h>
 * (which the DOS backend includes for setmode).
 * The engine talks only to these calls; io_local.c is the phase-1 console
 * backend and io_mtcp.c (phase 2) will be a drop-in telnet implementation.
 * All output is raw bytes: CP437 glyphs and ANSI escapes pass through
 * untouched. The board owns the bytes; this layer is a dumb pipe.
 */
#include "pigtypes.h"

#ifdef __cplusplus
extern "C" {            /* io_mtcp.cpp implements these for the C core to call */
#endif

int  io_init(void);                       /* one-time setup (open stack / console); 0 ok */
void io_shutdown(void);

/* Session lifecycle. io_init/io_shutdown bracket the whole program; these
 * bracket one caller. The telnet backend listens+accepts here and serves
 * callers in a loop; the local console runs exactly one session. */
int  io_session_begin(void);              /* block for next caller; 0 = serve, -1 = stop */
void io_session_end(void);                /* hang up the current caller */

void io_putc(pp_u8 ch);                    /* one raw byte */
void io_puts(const char *s);               /* NUL-terminated raw bytes */
void io_write(const pp_u8 *buf, pp_u16 len);

int  io_getch(void);                       /* blocking; key 0..255, or -1 if carrier lost */
void io_idle(void);                        /* yield (no-op on local console) */
int  io_carrier(void);                     /* 1 = connected; local console is always 1 */

#ifdef __cplusplus
}
#endif

#endif /* PIGPEN_IO_H */
