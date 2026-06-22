/*
 * render_test.c -- host unit tests for the template engine. `make test`.
 *
 * render.c emits through the io backend (io_putc/io_puts); the test supplies
 * its own capturing backend so a template's exact byte output can be asserted
 * without a console. Covers existing behavior (passthrough, LF->CRLF, color,
 * data tokens, width/justify) plus the new cursor/screen-control codes.
 */
#include <stdio.h>
#include <string.h>
#include "render.h"
#include "pigtypes.h"

/* ---- capturing io backend (only the calls render.c actually makes) ------- */
static char g_buf[4096];
static int  g_len;

void io_putc(pp_u8 c)            { if (g_len < (int)sizeof(g_buf) - 1) g_buf[g_len++] = (char)c; }
void io_puts(const char *s)     { while (*s) io_putc((pp_u8)*s++); }

static void reset(void) { g_len = 0; g_buf[0] = '\0'; }
static const char *out(void)    { g_buf[g_len] = '\0'; return g_buf; }

/* ---- test harness -------------------------------------------------------- */
static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

/* Render a string template with a fresh default ctx (handle = "Dan"). */
static const char *r(const char *tpl)
{
    pp_ctx c;
    pp_ctx_init(&c);
    c.handle = "Dan";
    reset();
    render_strn(tpl, &c);
    return out();
}

/* lightbar-marker registrar: capture each |{...} option the renderer reports */
static struct { int row, col, key; char label[32]; } g_bar[8];
static int g_nbar;
static void bar_cb(void *u, int row, int col, int key, const char *label)
{
    (void)u;
    if (g_nbar < 8) {
        g_bar[g_nbar].row = row; g_bar[g_nbar].col = col; g_bar[g_nbar].key = key;
        strncpy(g_bar[g_nbar].label, label, sizeof g_bar[0].label - 1);
        g_bar[g_nbar].label[sizeof g_bar[0].label - 1] = '\0';
        g_nbar++;
    }
}

int main(void)
{
    /* 1. plain passthrough + lone LF -> CRLF */
    check("passthrough + LF->CRLF", strcmp(r("hi\n"), "hi\r\n") == 0);

    /* 2. existing: foreground color |15 -> bold white on black */
    check("color |15", strcmp(r("|15"), "\x1b[1;37;40m") == 0);

    /* 3. existing: data token |UH */
    check("token |UH", strcmp(r("|UH"), "Dan") == 0);

    /* 4. existing: width modifier (right-justify to 5) */
    check("token |UH\\>5", strcmp(r("|UH\\>5"), "  Dan") == 0);

    /* 5. NEW: cursor to column ## -> ANSI CHA */
    check("|[X10 -> ESC[10G", strcmp(r("|[X10"), "\x1b[10G") == 0);

    /* 6. NEW: cursor to row ## -> ANSI VPA */
    check("|[Y5 -> ESC[5d", strcmp(r("|[Y5"), "\x1b[5d") == 0);

    /* 7. NEW: |[X with no digits defaults to column 1 */
    check("|[X -> ESC[1G (default)", strcmp(r("|[X"), "\x1b[1G") == 0);

    /* 8. NEW: clear screen + home */
    check("|CL -> ESC[2J ESC[H", strcmp(r("|CL"), "\x1b[2J\x1b[H") == 0);

    /* 9. NEW: clear to end of line */
    check("|CE -> ESC[K", strcmp(r("|CE"), "\x1b[K") == 0);

    /* 10. control code then LF still normalizes to CRLF */
    check("|CE then LF -> CRLF", strcmp(r("|CE\n"), "\x1b[K\r\n") == 0);

    /* 11. unknown two-char code is emitted literally (no char-eating) */
    check("unknown |ZZ literal", strcmp(r("|ZZ"), "|ZZ") == 0);

    /* 12. positioning composes with text (a one-line form field) */
    check("compose |[Y2|[X5 text",
          strcmp(r("|[Y2|[X5Name:"), "\x1b[2d\x1b[5GName:") == 0);

    /* 13. unterminated lightbar marker emits literally (no char-eating) */
    check("malformed |{ literal", strcmp(r("|{2,5,F,Files"), "|{2,5,F,Files") == 0);

    /* 14. lightbar markers: draw each label in place AND register it */
    {
        pp_ctx c;
        pp_ctx_init(&c);
        c.bar = bar_cb;
        g_nbar = 0;
        reset();
        render_strn("hdr|{2,5,F,Files}mid|{3,5,M,Mail}", &c);
        check("markers draw labels at positions",
              strcmp(out(), "hdr\x1b[2;5HFilesmid\x1b[3;5HMail") == 0);
        check("registered two options", g_nbar == 2);
        check("option 1 = (2,5,'F',Files)",
              g_bar[0].row == 2 && g_bar[0].col == 5 && g_bar[0].key == 'F' &&
              strcmp(g_bar[0].label, "Files") == 0);
        check("option 2 = (3,5,'M',Mail)",
              g_bar[1].row == 3 && g_bar[1].col == 5 && g_bar[1].key == 'M' &&
              strcmp(g_bar[1].label, "Mail") == 0);
    }

    /* 15. marker label may contain spaces (and commas after the 3rd field) */
    {
        pp_ctx c;
        pp_ctx_init(&c);
        c.bar = bar_cb;
        g_nbar = 0;
        reset();
        render_strn("|{10,20,G,Goodbye, caller}", &c);
        check("label keeps spaces/commas",
              g_nbar == 1 && strcmp(g_bar[0].label, "Goodbye, caller") == 0);
    }

    printf("\nrender_test: %s\n", g_fail ? "FAILED" : "all passed");
    return g_fail;
}
