/*
 * render_dump.c -- host-only dev tool (not part of the board). Renders a
 * screen file through the REAL board renderer and writes the resulting raw
 * ANSI+CP437 byte stream to stdout, so tools/ansi2png.py can turn it into a
 * PNG that is byte-for-byte what a caller would actually see. Pipe codes have
 * exactly one implementation (core/render.c) -- this shares it, no drift.
 *
 *   render_dump art/lbdemo.pp            -> static screen
 *   render_dump art/lbdemo.pp 1          -> with lightbar option #1 highlighted
 *
 * Build:  cc -std=c89 -Icore -Iio tools/render_dump.c core/render.c -o render_dump
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include "render.h"
#include "lbmenu.h"
#include "ppio.h"

/* ppio backend: emit straight to stdout */
void io_putc(pp_u8 c)                    { putchar((int)c); }
void io_puts(const char *s)              { fputs(s, stdout); }
void io_write(const pp_u8 *b, pp_u16 n)  { fwrite(b, 1, (size_t)n, stdout); }

/* collect |{...} lightbar markers so we can paint a selected bar for the shot */
static lb_opt g_opt[64];
static int    g_nopt;
static void bar_cb(void *u, int row, int col, int key, const char *label)
{
    (void)u;
    if (g_nopt < 64) {
        g_opt[g_nopt].row = row; g_opt[g_nopt].col = col; g_opt[g_nopt].key = key;
        strncpy(g_opt[g_nopt].label, label, LB_LABEL - 1);
        g_opt[g_nopt].label[LB_LABEL - 1] = '\0';
        g_nopt++;
    }
}

int main(int argc, char **argv)
{
    const char *path;
    int sel = -1;
    size_t n;
    pp_ctx ctx;
    FILE *f;

    if (argc < 2) {
        fprintf(stderr, "usage: render_dump <screen.pp|.ans> [selected-index]\n");
        return 2;
    }
    path = argv[1];
    if (argc >= 3) sel = atoi(argv[2]);

    pp_ctx_init(&ctx);
    ctx.handle   = "SYSOP";
    ctx.board    = "Vendetta/X";
    ctx.version  = "0.1";
    ctx.location = "Calgary, AB";
    ctx.callnum  = "1,337";
    if (sel >= 0) { ctx.bar = bar_cb; ctx.bar_user = (void *)0; }

    f = fopen(path, "rb");
    if (f == (FILE *)0) { fprintf(stderr, "render_dump: cannot open %s\n", path); return 1; }

    n = strlen(path);
    if (n >= 3 && path[n - 3] == '.' &&
        tolower((unsigned char)path[n - 2]) == 'p' &&
        tolower((unsigned char)path[n - 1]) == 'p')
        render_tpl(f, &ctx);
    else
        render_raw(f);
    fclose(f);

    /* paint the chosen option highlighted, the way lbmenu_run would */
    if (sel >= 0 && sel < g_nopt)
        printf("\x1b[%d;%dH\x1b[1;37;41m%s\x1b[0m",
               g_opt[sel].row, g_opt[sel].col, g_opt[sel].label);

    return 0;
}
