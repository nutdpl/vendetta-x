/*
 * render.c -- the Vendetta/X template engine (phase 1).
 *
 * Everything reads through pp_src, a one-byte-pushback source backed by
 * either a FILE or an in-memory string, so the same pipe-code parser drives
 * art files, string-table entries, and list-line templates alike.
 */
#include <string.h>
#include "render.h"
#include "ppio.h"

/* DOS palette index -> ANSI SGR base code (RGB bit order differs from DOS). */
static const int ANSI_FG[8] = { 30, 34, 32, 36, 31, 35, 33, 37 };
static const int ANSI_BG[8] = { 40, 44, 42, 46, 41, 45, 43, 47 };

/* Default theme: slots 1..9 -> DOS color index. Config will override later. */
static const int THEME[9] = { 8, 7, 15, 14, 11, 9, 12, 1, 0 };

/* Newline state for LF->CRLF normalization (single local caller, phase 1). */
static int g_last;

/* ---- byte source: file or string, with one char of pushback ------------- */

typedef struct {
    FILE       *f;     /* read from here if non-NULL ... */
    const char *s;     /* ... else from here */
    int         pb;    /* pushed-back char, or -1 */
} pp_src;

static void src_file(pp_src *x, FILE *f) { x->f = f; x->s = (const char *)0; x->pb = -1; }
static void src_str(pp_src *x, const char *s) { x->f = (FILE *)0; x->s = s; x->pb = -1; }

static int src_next(pp_src *x)
{
    if (x->pb != -1) { int c = x->pb; x->pb = -1; return c; }
    if (x->f) return fgetc(x->f);
    if (x->s && *x->s) return (unsigned char)*x->s++;
    return EOF;
}

static void src_back(pp_src *x, int c) { x->pb = c; }

/* ---- output helpers ----------------------------------------------------- */

void pp_ctx_init(pp_ctx *c)
{
    c->handle   = "";
    c->board    = "Vendetta/X";
    c->version  = "0.1";
    c->location = "";
    c->callnum  = "";
    c->cur_fg  = 7;
    c->cur_bg  = 0;
    c->lookup  = (pp_lookup_fn)0;
    c->user    = (void *)0;
    c->bar     = (pp_bar_fn)0;
    c->bar_user = (void *)0;
}

/* Emit one art byte, turning a bare LF into CRLF but never doubling \r. */
static void emit_byte(int c)
{
    if (c == '\n' && g_last != '\r')
        io_putc((pp_u8)'\r');
    io_putc((pp_u8)c);
    g_last = c;
}

static void emit_str(const char *s)
{
    while (*s)
        emit_byte((unsigned char)*s++);
}

static void emit_attr(pp_ctx *c)
{
    char buf[16];
    int bold = (c->cur_fg >= 8) ? 1 : 0;
    sprintf(buf, "\x1b[%d;%d;%dm", bold, ANSI_FG[c->cur_fg & 7], ANSI_BG[c->cur_bg & 7]);
    io_puts(buf);          /* escape sequence bypasses newline normalization */
    g_last = 'm';
}

static int is_digit(int c) { return c >= '0' && c <= '9'; }

/* Emit a control/escape sequence verbatim (no LF->CRLF rewriting). The
 * sequences never contain '\n'; g_last is left non-CR so a following bare LF
 * still normalizes to CRLF. */
static void emit_ctrl(const char *seq)
{
    io_puts(seq);
    g_last = 'm';
}

/* Read a non-negative decimal from src; if no digits follow, return def. */
static int read_num(pp_src *src, int def)
{
    int d = src_next(src);
    int n;
    if (!is_digit(d)) { src_back(src, d); return def; }
    n = 0;
    do { n = n * 10 + (d - '0'); d = src_next(src); } while (is_digit(d));
    src_back(src, d);
    return n;
}

/*
 * Emit a token value, applying an optional width modifier that follows it in
 * the source:  \[<>^]?NN  -> pad/truncate to NN columns (left/right/center,
 * default left). If what follows '\' is not a valid spec, the consumed bytes
 * are emitted literally and the value is printed unpadded.
 */
static void emit_value(pp_src *src, const char *val)
{
    int c = src_next(src);
    int align = '<';
    int width = -1;

    if (c == '\\') {
        int d = src_next(src);
        if (d == '<' || d == '>' || d == '^') { align = d; d = src_next(src); }
        if (is_digit(d)) {
            width = 0;
            do { width = width * 10 + (d - '0'); d = src_next(src); } while (is_digit(d));
            src_back(src, d);
        } else {
            src_back(src, d);
            emit_byte('\\');
            if (align != '<') emit_byte(align);
        }
    } else {
        src_back(src, c);
    }

    if (width < 0) { emit_str(val); return; }

    {
        int len = (int)strlen(val);
        if (len >= width) {
            int i;
            for (i = 0; i < width; i++) emit_byte((unsigned char)val[i]);   /* truncate */
        } else {
            int pad = width - len, left = 0, right = 0, i;
            if (align == '>')      { left = pad; }
            else if (align == '^') { left = pad / 2; right = pad - left; }
            else                   { right = pad; }
            for (i = 0; i < left; i++)  emit_byte(' ');
            emit_str(val);
            for (i = 0; i < right; i++) emit_byte(' ');
        }
    }
}

/* Parse and act on a lightbar marker body "R,C,K,Label" (the braces already
 * stripped): draw Label at (R,C) and register it with ctx->bar if present. */
static void handle_bar(pp_ctx *c, const char *spec)
{
    int row = 0, col = 0, key = 0, i = 0;
    char seq[24];

    while (spec[i] == ' ') i++;
    while (is_digit(spec[i])) { if (row < 1000) row = row * 10 + (spec[i] - '0'); i++; }
    if (spec[i] == ',') i++;
    while (spec[i] == ' ') i++;
    while (is_digit(spec[i])) { if (col < 1000) col = col * 10 + (spec[i] - '0'); i++; }
    if (spec[i] == ',') i++;
    while (spec[i] == ' ') i++;
    if (spec[i]) key = (unsigned char)spec[i++];
    if (spec[i] == ',') i++;
    while (spec[i] == ' ') i++;

    if (row < 1) row = 1; else if (row > 999) row = 999;
    if (col < 1) col = 1; else if (col > 999) col = 999;
    sprintf(seq, "\x1b[%d;%dH", row, col);
    emit_ctrl(seq);
    emit_str(spec + i);                       /* the label, drawn in place */
    if (c->bar) c->bar(c->bar_user, row, col, key, spec + i);
}

/* Resolve a two-char data token to its value, or NULL if not a token. */
static const char *token_value(pp_ctx *c, int a, int b)
{
    if (a == 'U' && b == 'H') return c->handle;
    if (a == 'U' && b == 'L') return c->location ? c->location : "";
    if (a == 'U' && b == 'C') return c->callnum ? c->callnum : "";
    if (a == 'B' && b == 'N') return c->board;
    if (a == 'V' && b == 'R') return c->version;
    if (c->lookup) return c->lookup(c->user, a, b);
    return (const char *)0;
}

/* Handle one '|' code; the leading '|' has already been consumed. */
static void handle_code(pp_src *src, pp_ctx *c)
{
    int a = src_next(src);
    int b;
    const char *val;

    if (a == EOF) { emit_byte('|'); return; }
    b = src_next(src);
    if (b == EOF) { emit_byte('|'); emit_byte(a); return; }

    /* Cursor positioning (DESIGN.md / Mystic-style): |[X## -> column,
     * |[Y## -> row, 1-based. ANSI CHA (col) / VPA (row); default 1. */
    if (a == '[' && (b == 'X' || b == 'x' || b == 'Y' || b == 'y')) {
        char seq[16];
        int n = read_num(src, 1);
        if (n < 1) n = 1;
        sprintf(seq, "\x1b[%d%c", n, (b == 'X' || b == 'x') ? 'G' : 'd');
        emit_ctrl(seq);
        return;
    }
    /* Screen control: |CL clear screen + home, |CE clear to end of line.
     * Intercepted before token lookup so a data token can never shadow them. */
    if (a == 'C' && b == 'L') { emit_ctrl("\x1b[2J\x1b[H"); return; }
    if (a == 'C' && b == 'E') { emit_ctrl("\x1b[K"); return; }

    /* Lightbar option marker: |{R,C,K,Label} -- collect the body up to '}'. */
    if (a == '{') {
        char m[96];
        int mi = 0, d = b;
        while (d != EOF && d != '}' && mi < (int)sizeof m - 1) { m[mi++] = (char)d; d = src_next(src); }
        m[mi] = '\0';
        if (d == '}') { handle_bar(c, m); return; }
        emit_byte('|'); emit_byte('{');       /* malformed: emit literally */
        { int i; for (i = 0; i < mi; i++) emit_byte((unsigned char)m[i]); }
        return;
    }

    if (is_digit(a) && is_digit(b)) {
        int n = (a - '0') * 10 + (b - '0');
        if (n <= 15) { c->cur_fg = n;      emit_attr(c); return; }
        if (n <= 23) { c->cur_bg = n - 16; emit_attr(c); return; }
    } else if (a == 'T' && b >= '1' && b <= '9') {
        c->cur_fg = THEME[b - '1'];
        emit_attr(c);
        return;
    } else {
        val = token_value(c, a, b);
        if (val) { emit_value(src, val); return; }
    }

    /* unrecognized: emit literally so a stray pipe never eats characters */
    emit_byte('|');
    emit_byte(a);
    emit_byte(b);
}

/* ---- public entry points ------------------------------------------------ */

static void render_src(pp_src *src, pp_ctx *c)
{
    int ch;
    while ((ch = src_next(src)) != EOF) {
        if (ch == '|')
            handle_code(src, c);
        else
            emit_byte(ch);
    }
}

void render_raw(FILE *f)
{
    int c;
    g_last = 0;
    while ((c = fgetc(f)) != EOF)
        emit_byte(c);
}

void render_tpl(FILE *f, pp_ctx *c)
{
    pp_src src;
    g_last = 0;
    src_file(&src, f);
    render_src(&src, c);
}

void render_strn(const char *s, pp_ctx *c)
{
    pp_src src;
    g_last = 0;
    src_str(&src, s);
    render_src(&src, c);
}

void render_list(pp_ctx *c, const char *hdr, const char *line,
                 const char *ftr, void **rows, int nrows)
{
    void *save = c->user;
    int i;
    if (hdr) render_strn(hdr, c);
    for (i = 0; i < nrows; i++) {
        c->user = rows[i];
        render_strn(line, c);
    }
    c->user = save;
    if (ftr) render_strn(ftr, c);
}
