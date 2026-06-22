/*
 * lbmenu.c -- lightbar menu engine. See lbmenu.h.
 */
#include <stdio.h>
#include <string.h>
#include "lbmenu.h"
#include "render.h"
#include "ppio.h"

#define LB_SEL    "\x1b[1;37;41m"   /* selected: bright white on red */
#define LB_NORMAL "\x1b[0;37;40m"   /* normal: grey on black */

static int lower(int c) { return (c >= 'A' && c <= 'Z') ? c + 32 : c; }
static int lb_abs(int v) { return v < 0 ? -v : v; }

/* Find the option nearest to opts[cur] in screen direction dir ('U' 'D' 'L'
 * 'R'), so arrow keys move the bar across a 2-D grid, not just a flat list.
 * Picks the closest along the travel axis, ties broken by the other axis.
 * Returns -1 if nothing lies that way (edge of the menu -> bar stays put). */
static int lb_neighbor(const lb_opt *o, int n, int cur, int dir)
{
    int best = -1, bprim = 0, bsec = 0, i;
    int crow = o[cur].row, ccol = o[cur].col;
    for (i = 0; i < n; i++) {
        int ok, rd, cd, prim, sec;
        if (i == cur) continue;
        rd = o[i].row - crow;
        cd = o[i].col - ccol;
        switch (dir) {
            case 'U': ok = (rd < 0); break;
            case 'D': ok = (rd > 0); break;
            case 'L': ok = (cd < 0); break;
            default:  ok = (cd > 0); break;     /* 'R' */
        }
        if (!ok) continue;
        if (dir == 'U' || dir == 'D') { prim = lb_abs(rd); sec = lb_abs(cd); }
        else                          { prim = lb_abs(cd); sec = lb_abs(rd); }
        if (best < 0 || prim < bprim || (prim == bprim && sec < bsec)) {
            best = i; bprim = prim; bsec = sec;
        }
    }
    return best;
}

/* Repaint one option in its normal or selected rendition, at its position. */
static void draw_opt(const lb_opt *o, int selected)
{
    char seq[24];
    sprintf(seq, "\x1b[%d;%dH", o->row, o->col);
    io_puts(seq);
    io_puts(selected ? LB_SEL : LB_NORMAL);
    io_puts(o->label);
    io_puts("\x1b[0m");
}

int lbmenu_run(pp_ctx *ctx, const lb_opt *opts, int n, int start)
{
    int sel = (start < 0 || start >= n) ? 0 : start;
    int i;
    (void)ctx;
    if (n <= 0) return -1;

    io_puts("\x1b[?25l");                     /* hide cursor while the bar moves */
    for (i = 0; i < n; i++) draw_opt(&opts[i], i == sel);

    for (;;) {
        int c = io_getch();
        if (c < 0) { io_puts("\x1b[?25h"); return -1; }

        if (c == '\r' || c == '\n')
            break;

        if (c == 27) {                        /* escape seq -> arrow keys */
            int d = io_getch();
            if (d == '[' || d == 'O') {
                int e = io_getch(), ni = -1;
                if      (e == 'A') ni = lb_neighbor(opts, n, sel, 'U');
                else if (e == 'B') ni = lb_neighbor(opts, n, sel, 'D');
                else if (e == 'C') ni = lb_neighbor(opts, n, sel, 'R');
                else if (e == 'D') ni = lb_neighbor(opts, n, sel, 'L');
                if (ni >= 0 && ni != sel) {
                    draw_opt(&opts[sel], 0); sel = ni; draw_opt(&opts[sel], 1);
                }
            }
            continue;
        }

        {                                      /* hotkey selects + confirms */
            int lc = lower(c), k;
            for (k = 0; k < n; k++) {
                if (lower(opts[k].key) == lc) {
                    if (k != sel) { draw_opt(&opts[sel], 0); draw_opt(&opts[k], 1); }
                    io_puts("\x1b[?25h");
                    return k;
                }
            }
        }
    }

    io_puts("\x1b[?25h");
    return sel;
}

/* collector for lbmenu_screen: gather |{...} markers as the screen renders.
 * 8 is plenty for a standalone screen (the login matrix has 3); keeping it
 * small matters for the 16-bit DGROUP budget. */
static lb_opt g_scr[8];
static int    g_nscr;
static void scr_collect(void *u, int row, int col, int key, const char *label)
{
    (void)u;
    if (g_nscr < (int)(sizeof g_scr / sizeof g_scr[0])) {
        lb_opt *o = &g_scr[g_nscr++];
        o->row = row; o->col = col; o->key = key;
        strncpy(o->label, label, LB_LABEL - 1);
        o->label[LB_LABEL - 1] = '\0';
    }
}

int lbmenu_screen(pp_ctx *ctx, const char *path)
{
    FILE *f = fopen(path, "rb");
    size_t n;
    int sel;
    if (f == (FILE *)0) return -1;

    g_nscr = 0;
    ctx->bar = scr_collect; ctx->bar_user = (void *)0;
    n = strlen(path);
    if (n >= 3 && path[n - 3] == '.' &&
        (path[n - 2] | 0x20) == 'p' && (path[n - 1] | 0x20) == 'p')
        render_tpl(f, ctx);
    else
        render_raw(f);
    fclose(f);
    ctx->bar = (pp_bar_fn)0;

    if (g_nscr == 0) return -1;
    sel = lbmenu_run(ctx, g_scr, g_nscr, 0);
    if (sel < 0) return -1;
    return g_scr[sel].key;
}
