/*
 * io_local.c -- local console backend.
 *
 * Output goes to stdout as raw bytes (CP437 + ANSI), unbuffered, with no
 * newline translation so art renders byte-exact. On the modern host this
 * is the test bench; in DOSBox the same bytes drive ANSI.SYS / the built-in
 * ANSI console. Input is read one key at a time with echo off.
 */
#include <stdio.h>
#include <string.h>
#include "ppio.h"

#if defined(__WATCOMC__)   /* any Watcom DOS target (16- or 32-bit), not the host */
#  include <conio.h>      /* getch */
#  include <io.h>         /* setmode */
#  include <fcntl.h>      /* O_BINARY */
#else
#  include <unistd.h>     /* isatty */
#  include <termios.h>
static struct termios g_saved_tty;
static int g_raw = 0;
#endif

int io_init(void)
{
    setvbuf(stdout, (char *)0, _IONBF, 0);   /* unbuffered: bytes hit the wire now */
#if defined(__WATCOMC__)   /* any Watcom DOS target (16- or 32-bit), not the host */
    setmode(fileno(stdout), O_BINARY);       /* no \n -> \r\n mangling */
#else
    if (isatty(0)) {
        struct termios raw;
        if (tcgetattr(0, &g_saved_tty) == 0) {
            raw = g_saved_tty;
            raw.c_lflag &= ~(tcflag_t)(ICANON | ECHO);
            raw.c_cc[VMIN] = 1;
            raw.c_cc[VTIME] = 0;
            if (tcsetattr(0, TCSANOW, &raw) == 0)
                g_raw = 1;
        }
    }
#endif
    return 0;
}

void io_shutdown(void)
{
#if !defined(__WATCOMC__)
    if (g_raw) {
        tcsetattr(0, TCSANOW, &g_saved_tty);
        g_raw = 0;
    }
#endif
}

/* The local console serves exactly one session (the sysop at the keyboard). */
static int g_session_done;

int io_session_begin(void)
{
    if (g_session_done) return -1;
    g_session_done = 1;
    return 0;
}

void io_session_end(void)
{
}

void io_putc(pp_u8 ch)
{
    putc((int)ch, stdout);
}

void io_puts(const char *s)
{
    fwrite(s, 1, strlen(s), stdout);
}

void io_write(const pp_u8 *buf, pp_u16 len)
{
    fwrite(buf, 1, (size_t)len, stdout);
}

int io_getch(void)
{
#if defined(__WATCOMC__)   /* any Watcom DOS target (16- or 32-bit), not the host */
    return getch() & 0xFF;
#else
    int c = getchar();
    return (c == EOF) ? -1 : c;
#endif
}

void io_idle(void)
{
    /* single local caller: nothing to yield to yet */
}

int io_carrier(void)
{
    return 1;
}
