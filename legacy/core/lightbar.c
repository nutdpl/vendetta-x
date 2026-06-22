/*
 * lightbar.c -- inline horizontal lightbar. Positions itself with ANSI cursor
 * save/restore (ESC[s / ESC[u) so the caller just prints a prompt and calls it.
 */
#include "lightbar.h"
#include "ppio.h"

#define SEL_ON  "\x1b[1;37;41m"   /* highlighted: bright white on red */
#define SEL_OFF "\x1b[0;37;40m"   /* normal: grey */

static void draw(const char *const *opts, int n, int sel)
{
    int i;
    io_puts("\x1b[u");                       /* back to the saved spot */
    for (i = 0; i < n; i++) {
        io_puts(i == sel ? SEL_ON : SEL_OFF);
        io_puts("  ");
        io_puts(opts[i]);
        io_puts("  \x1b[0m ");
    }
}

static int lower(int c) { return (c >= 'A' && c <= 'Z') ? c + 32 : c; }

int lightbar(pp_ctx *ctx, const char *const *opts, int n, int start)
{
    int sel = (start < 0 || start >= n) ? 0 : start;
    (void)ctx;

    io_puts("\x1b[?25l\x1b[s");               /* hide cursor, save position */
    draw(opts, n, sel);

    for (;;) {
        int c = io_getch();
        if (c < 0) { io_puts("\x1b[?25h"); return -1; }

        if (c == '\r' || c == '\n')
            break;

        if (c == 27) {                        /* escape seq -> arrow keys */
            int d = io_getch();
            if (d == '[' || d == 'O') {
                int e = io_getch();
                if (e == 'D' || e == 'A') { sel = (sel > 0) ? sel - 1 : n - 1; draw(opts, n, sel); }
                else if (e == 'C' || e == 'B') { sel = (sel < n - 1) ? sel + 1 : 0; draw(opts, n, sel); }
            }
            continue;
        }

        {                                      /* first-letter shortcut selects + confirms */
            int lc = lower(c), i;
            for (i = 0; i < n; i++) {
                if (lower((int)opts[i][0]) == lc) {
                    sel = i;
                    draw(opts, n, sel);
                    io_puts("\x1b[?25h");
                    return sel;
                }
            }
        }
    }

    io_puts("\x1b[?25h");
    return sel;
}

/* ---- vertical lightbar: one option per line, the backbone of arrow-key
 * menus and area/file pickers. Anchors at the current cursor position (saved/
 * restored) and paints downward. Up/Down move (wrapping), Enter picks, an
 * option's first letter selects it outright. Returns the index, -1 on hangup. */

static void vdraw(const char *const *opts, int n, int sel)
{
    int i;
    io_puts("\x1b[u");                       /* back to the anchor (list top) */
    for (i = 0; i < n; i++) {
        if (i > 0) io_puts("\r\n");
        io_puts(i == sel ? SEL_ON : SEL_OFF);
        io_puts(" ");
        io_puts(opts[i]);
        io_puts(" \x1b[0m");
    }
}

int lightbar_vert(pp_ctx *ctx, const char *const *opts, int n, int start)
{
    int sel = (start < 0 || start >= n) ? 0 : start;
    (void)ctx;
    if (n <= 0) return -1;

    io_puts("\x1b[?25l\x1b[s");               /* hide cursor, anchor the list */
    vdraw(opts, n, sel);

    for (;;) {
        int c = io_getch();
        if (c < 0) { io_puts("\x1b[?25h"); return -1; }

        if (c == '\r' || c == '\n')
            break;

        if (c == 27) {                        /* escape seq -> arrow keys */
            int d = io_getch();
            if (d == '[' || d == 'O') {
                int e = io_getch();
                if (e == 'A') { sel = (sel > 0) ? sel - 1 : n - 1; vdraw(opts, n, sel); }
                else if (e == 'B') { sel = (sel < n - 1) ? sel + 1 : 0; vdraw(opts, n, sel); }
            }
            continue;
        }

        {                                      /* first-letter shortcut */
            int lc = lower(c), i;
            for (i = 0; i < n; i++) {
                if (lower((int)opts[i][0]) == lc) {
                    sel = i;
                    vdraw(opts, n, sel);
                    io_puts("\x1b[?25h");
                    return sel;
                }
            }
        }
    }

    io_puts("\x1b[?25h");
    return sel;
}
