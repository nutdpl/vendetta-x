/*
 * editor.c -- full-screen message editor with a custom ANSI header.
 *
 * Layout (80x24): the header template owns rows 1..ED_TOP-1; the body region
 * is ED_LINES rows starting at ED_TOP, ED_W columns wide from ED_LEFT; the
 * help line is row ED_STAT. Word-wrap happens on type. The cursor stays at the
 * end of the text (type / backspace / Enter) -- no mid-line editing yet, which
 * keeps the redraw model simple and correct; arrow navigation is a future add.
 */
#include <stdio.h>
#include <string.h>
#include "editor.h"
#include "ppio.h"
#include "strtab.h"

#define ED_TOP    9      /* first body row (header template fills rows 1..8) */
#define ED_LEFT   3      /* first body column */
#define ED_LINES  14     /* body rows: 9..22 */
#define ED_W      74     /* usable columns per line */
#define ED_STAT   24     /* help line row */

static char        g_line[ED_LINES][ED_W + 1];
static int         g_len[ED_LINES];
static int         g_nl, g_cl, g_cc;        /* line count, cursor line, cursor col */

/* header tokens, resolved while the frame renders */
static const char *g_from, *g_to, *g_subj;
static const char *ed_lookup(void *u, int a, int b)
{
    (void)u;
    if (a == 'M' && b == 'F') return g_from ? g_from : "";
    if (a == 'M' && b == 'T') return g_to   ? g_to   : "";
    if (a == 'M' && b == 'S') return g_subj ? g_subj : "";
    return (const char *)0;
}

static void gotoxy(int row, int col)
{
    char b[16];
    sprintf(b, "\x1b[%d;%dH", row, col);
    io_puts(b);
}

/* redraw one body line and clear to end of line, then leave cursor at its end */
static void draw_line(int li)
{
    gotoxy(ED_TOP + li, ED_LEFT);
    io_write((const pp_u8 *)g_line[li], (pp_u16)g_len[li]);
    io_puts("\x1b[K");
}

static void place_cursor(void)
{
    gotoxy(ED_TOP + g_cl, ED_LEFT + g_cc);
}

/* The current line just hit the right margin -- move its trailing word (the
 * text after the last space) down to a fresh line, so words don't get split. */
static void wrap_current(void)
{
    char *L = g_line[g_cl];
    int sp = -1, i;

    if (g_nl >= ED_LINES) return;            /* no room: leave it hard-filled */

    for (i = g_len[g_cl] - 1; i > 0; i--)
        if (L[i] == ' ') { sp = i; break; }

    g_cl = g_nl;
    g_nl++;

    if (sp <= 0) {                            /* one long word: hard break */
        g_len[g_cl] = 0;
        g_line[g_cl][0] = '\0';
        g_cc = 0;
    } else {
        int wstart = sp + 1;
        int wlen = g_len[g_cl - 1] - wstart;
        memcpy(g_line[g_cl], L + wstart, (size_t)wlen);
        g_line[g_cl][wlen] = '\0';
        g_len[g_cl] = wlen;
        L[sp] = '\0';                         /* drop the break space */
        g_len[g_cl - 1] = sp;
        g_cc = wlen;
        draw_line(g_cl - 1);                  /* the shortened previous line */
    }
    draw_line(g_cl);
    place_cursor();
}

static void type_char(int c)
{
    if (g_len[g_cl] >= ED_W) {                /* full line, no space made room */
        if (g_nl >= ED_LINES) return;        /* editor full */
        wrap_current();
    }
    g_line[g_cl][g_len[g_cl]] = (char)c;
    g_len[g_cl]++;
    g_line[g_cl][g_len[g_cl]] = '\0';
    g_cc++;
    io_putc((pp_u8)c);                        /* echo at the cursor */
    if (g_len[g_cl] >= ED_W)
        wrap_current();
}

static void do_enter(void)
{
    if (g_nl >= ED_LINES) return;
    g_cl = g_nl;
    g_nl++;
    g_len[g_cl] = 0;
    g_line[g_cl][0] = '\0';
    g_cc = 0;
    place_cursor();
}

static void do_backspace(void)
{
    if (g_cc > 0) {
        g_cc--;
        g_len[g_cl]--;
        g_line[g_cl][g_len[g_cl]] = '\0';
        io_puts("\b \b");
    } else if (g_cl > 0 && g_len[g_cl] == 0) {
        g_nl--;
        g_cl--;
        g_cc = g_len[g_cl];
        place_cursor();
    }
}

int editor_run(pp_ctx *ctx, const char *hdr_template,
               const char *from, const char *to, const char *subj,
               char *out, int outsz)
{
    pp_lookup_fn save_lu = ctx->lookup;
    void        *save_u  = ctx->user;
    FILE        *f;
    int          i, saved = 0;

    /* clear + draw the custom ANSI header frame with live tokens */
    io_puts("\x1b[0m\x1b[2J\x1b[1;1H");
    g_from = from; g_to = to; g_subj = subj;
    ctx->lookup = ed_lookup;
    ctx->user = (void *)0;
    f = fopen(hdr_template, "rb");
    if (f != (FILE *)0) { render_tpl(f, ctx); fclose(f); }
    ctx->lookup = save_lu;
    ctx->user = save_u;

    /* help line */
    gotoxy(ED_STAT, 1);
    render_strn(pps_get(PPS_EDIT_HELP), ctx);

    /* empty body, cursor at top-left of the region */
    for (i = 0; i < ED_LINES; i++) { g_line[i][0] = '\0'; g_len[i] = 0; }
    g_nl = 1; g_cl = 0; g_cc = 0;
    place_cursor();

    for (;;) {
        int c = io_getch();
        if (c < 0) { saved = 0; break; }
        if (c == 26) { saved = 1; break; }          /* ^Z save */
        if (c == 24) { saved = 0; break; }          /* ^X abort */
        if (c == 27) {                               /* swallow an escape seq (arrows) */
            int d = io_getch();
            if (d == '[' || d == 'O') io_getch();
            continue;
        }
        if (c == '\r' || c == '\n') { do_enter(); continue; }
        if (c == 8 || c == 127)     { do_backspace(); continue; }
        if (c >= 32)                { type_char(c); }
    }

    if (saved) {
        int o = 0;
        for (i = 0; i < g_nl; i++) {
            int ll = g_len[i];
            if (o + ll + 2 >= outsz) break;
            memcpy(out + o, g_line[i], (size_t)ll); o += ll;
            out[o++] = '\r'; out[o++] = '\n';
        }
        out[o] = '\0';
        /* trim to a non-empty message: if only blank lines, treat as abort */
        for (i = 0; i < o; i++)
            if (out[i] != '\r' && out[i] != '\n' && out[i] != ' ') return 1;
        return 0;
    }
    return 0;
}
