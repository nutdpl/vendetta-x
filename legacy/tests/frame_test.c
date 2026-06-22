/*
 * frame_test.c -- host unit tests for CP437 box frames. `make test`.
 * Supplies a capturing io backend and asserts the exact byte stream (cursor
 * moves + line-draw glyphs) for small boxes.
 */
#include <stdio.h>
#include <string.h>
#include "frame.h"
#include "pigtypes.h"

static char g_buf[4096];
static int  g_len;
void io_putc(pp_u8 c)        { if (g_len < (int)sizeof(g_buf) - 1) g_buf[g_len++] = (char)c; }
void io_puts(const char *s)  { while (*s) io_putc((pp_u8)*s++); }
static void reset(void)      { g_len = 0; }
static const char *out(void) { g_buf[g_len] = '\0'; return g_buf; }

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

int main(void)
{
    /* 3x3 single box at (1,1): top row, the two side rows, bottom row.
     * single glyphs: TL=DA H=C4 TR=BF V=B3 BL=C0 BR=D9 */
    reset();
    frame_box(1, 1, 3, 3, FRAME_SINGLE);
    check("3x3 single box",
          strcmp(out(),
            "\x1b[1;1H\xDA\xC4\xBF"
            "\x1b[2;1H\xB3" "\x1b[2;3H\xB3"
            "\x1b[3;1H\xC0\xC4\xD9") == 0);

    /* double-line corners at an offset (2,5), width 4 height 2.
     * double glyphs: TL=C9 H=CD TR=BB BL=C8 BR=BC (no side rows: h-2==0) */
    reset();
    frame_box(2, 5, 4, 2, FRAME_DOUBLE);
    check("4x2 double box",
          strcmp(out(),
            "\x1b[2;5H\xC9\xCD\xCD\xBB"
            "\x1b[3;5H\xC8\xCD\xCD\xBC") == 0);

    /* degenerate sizes draw nothing */
    reset(); frame_box(1, 1, 1, 5, FRAME_SINGLE);
    check("w<2 draws nothing", out()[0] == '\0');

    /* titled box: title centered on the top border. inner span = w-2 = 8,
     * title "Hi" len 2 -> offset (8-2)/2 = 3 -> placed at col 1+1+3 = 5. */
    reset();
    frame_titled(1, 1, 10, 3, FRAME_SINGLE, "Hi");
    check("titled box places centered title",
          strstr(out(), "\x1b[1;5HHi") != (char *)0);

    printf("\nframe_test: %s\n", g_fail ? "FAILED" : "all passed");
    return g_fail;
}
