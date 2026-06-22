#ifndef PIGPEN_RENDER_H
#define PIGPEN_RENDER_H

/*
 * The renderer is the product (DESIGN.md).
 *
 *   render_raw   -- finished .ans art: pure byte passthrough (art may
 *                   legitimately contain '|' glyphs, so codes are NOT parsed).
 *   render_tpl   -- a template FILE: parse pipe codes and splice live data.
 *   render_strn  -- a template held in memory (a string-table entry, a list
 *                   line): same grammar as render_tpl.
 *   render_list  -- header/line/footer triplet (the Iniquity trick): the line
 *                   template is rendered once per row, so the sysop owns the
 *                   list format completely.
 *
 * Pipe-code grammar:
 *   |00..|15   foreground (DOS palette index)
 *   |16..|23   background (index - 16)
 *   |T1..|T9   theme slot -> foreground (recolor the board from one place)
 *   |UH|BN|VR  built-in data tokens (handle, board, version)
 *   |[X##      move cursor to column ## (1-based);  |[Y## -> row ##
 *   |CL        clear screen + home;  |CE clear to end of line
 *   |{R,C,K,Label}  lightbar option marker: draw Label at row R col C and
 *                   register it (hotkey K) so a lightbar menu can run a moving
 *                   highlight over the sysop's own ANSI -- the option positions
 *                   live in the art, not the code (DESIGN.md's Iniquity lightbar)
 *   a token may be followed by a width modifier  \[<>^]?NN  to pad/truncate
 *   to NN columns, left/right/center justified (default left), e.g. |LT\>5
 *   unknown two-char tokens are offered to ctx->lookup, then emitted literally
 *
 * All output normalizes lone LF to CRLF so Unix-authored art renders on a
 * real terminal without stair-stepping.
 */
#include <stdio.h>
#include "pigtypes.h"

typedef struct pp_ctx pp_ctx;

/* Resolver for tokens not built in (list-row fields, etc). Return the value
 * for the two-char code, or NULL to let the renderer emit it literally. */
typedef const char *(*pp_lookup_fn)(void *user, int a, int b);

/* Called for each |{R,C,K,Label} lightbar marker as the art renders, so a
 * lightbar menu engine can collect option positions out of the sysop's ANSI. */
typedef void (*pp_bar_fn)(void *user, int row, int col, int key, const char *label);

struct pp_ctx {
    const char  *handle;     /* |UH */
    const char  *board;      /* |BN */
    const char  *version;    /* |VR */
    const char  *location;   /* |UL  caller location */
    const char  *callnum;    /* |UC  times called (preformatted) */
    int          cur_fg;     /* DOS palette: 0..15 */
    int          cur_bg;     /* DOS palette: 0..7  */
    pp_lookup_fn lookup;     /* optional custom token resolver (may be NULL) */
    void        *user;       /* passed to lookup (e.g. the current list row) */
    pp_bar_fn    bar;        /* optional lightbar-marker registrar (may be NULL) */
    void        *bar_user;   /* passed to bar */
};

void pp_ctx_init(pp_ctx *c);

void render_raw(FILE *f);
void render_tpl(FILE *f, pp_ctx *c);
void render_strn(const char *s, pp_ctx *c);
void render_list(pp_ctx *c, const char *hdr, const char *line,
                 const char *ftr, void **rows, int nrows);

#endif /* PIGPEN_RENDER_H */
