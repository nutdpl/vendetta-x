#ifndef PIGPEN_LBMENU_H
#define PIGPEN_LBMENU_H

/*
 * lbmenu.c -- the lightbar menu engine. Runs a moving highlight over options
 * the sysop placed in their own ANSI screen with |{R,C,K,Label} markers (see
 * render.h): the menu loader collects those markers into lb_opt[] via the
 * renderer's bar registrar, then this drives the bar -- Up/Down move (wrapping),
 * Enter picks, an option's hotkey selects it outright. Only the two changed
 * options repaint as the bar moves (cheap on a 486 -- no full redraw).
 */
#include "render.h"

#define LB_LABEL 24

typedef struct {
    int  row, col;            /* 1-based screen position of the option label */
    int  key;                 /* hotkey that also selects it */
    char label[LB_LABEL];     /* visible text */
} lb_opt;

/* Run the bar over n pre-collected options drawn on the current screen.
 * Returns the chosen index (Enter or hotkey), or -1 on carrier loss. */
int lbmenu_run(pp_ctx *ctx, const lb_opt *opts, int n, int start);

/* Render a prebuilt lightbar screen (a .pp/.ans whose art carries |{...}
 * option markers) and run the bar over it. Returns the chosen option's hotkey
 * character, or -1 on carrier loss / no options. For standalone lightbar
 * screens outside the .MNU menu system (e.g. the login matrix). */
int lbmenu_screen(pp_ctx *ctx, const char *path);

#endif /* PIGPEN_LBMENU_H */
