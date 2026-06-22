/*
 * frame.c -- CP437 box-drawing frames. See frame.h.
 *
 * Each cell is placed with an absolute ANSI cursor move (CUP, ESC[r;cH) so a
 * frame draws correctly wherever it lands, independent of prior output. Glyphs
 * are raw CP437 bytes emitted through the io backend.
 */
#include <stdio.h>
#include <string.h>
#include "frame.h"
#include "ppio.h"

/* CP437 line-draw glyphs: { TL, TR, BL, BR, horizontal, vertical }. */
static const unsigned char GLYPH[2][6] = {
    { 0xDA, 0xBF, 0xC0, 0xD9, 0xC4, 0xB3 },   /* single */
    { 0xC9, 0xBB, 0xC8, 0xBC, 0xCD, 0xBA }    /* double */
};

/* Move the cursor to (row, col), 1-based (ANSI CUP). */
static void goxy(int row, int col)
{
    char buf[16];
    sprintf(buf, "\x1b[%d;%dH", row, col);
    io_puts(buf);
}

static void hbar(const unsigned char *g, int n)
{
    int i;
    for (i = 0; i < n; i++) io_putc(g[4]);
}

void frame_box(int row, int col, int w, int h, int style)
{
    const unsigned char *g = GLYPH[style ? 1 : 0];
    int k;

    if (w < 2 || h < 2) return;

    goxy(row, col);            io_putc(g[0]); hbar(g, w - 2); io_putc(g[1]);
    for (k = 1; k < h - 1; k++) {
        goxy(row + k, col);          io_putc(g[5]);
        goxy(row + k, col + w - 1);  io_putc(g[5]);
    }
    goxy(row + h - 1, col);    io_putc(g[2]); hbar(g, w - 2); io_putc(g[3]);
}

void frame_titled(int row, int col, int w, int h, int style, const char *title)
{
    frame_box(row, col, w, h, style);

    if (title && *title) {
        int inner = w - 2;                 /* span between the corners */
        int len = (int)strlen(title);
        int off, i;
        if (len > inner) len = inner;      /* truncate to fit */
        off = (inner - len) / 2;
        goxy(row, col + 1 + off);
        for (i = 0; i < len; i++) io_putc((unsigned char)title[i]);
    }
}
