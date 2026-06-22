#ifndef PIGPEN_LIGHTBAR_H
#define PIGPEN_LIGHTBAR_H

/*
 * Horizontal lightbar selector (DESIGN.md's Iniquity lightbar, brought forward
 * for the new-user yes/no). Draws the options at the current cursor position
 * (saved/restored so it works inline anywhere). Left/Right arrows move the
 * highlight; Enter picks it; an option's first letter selects it outright.
 * Returns the chosen index, or -1 on carrier loss.
 */
#include "render.h"

int lightbar(pp_ctx *ctx, const char *const *opts, int nopts, int start);

/* Vertical variant: one option per line from the current cursor position;
 * Up/Down move (wrapping), Enter picks, first letter selects. The backbone of
 * arrow-key menus and area/file pickers. Returns the index, or -1 on hangup. */
int lightbar_vert(pp_ctx *ctx, const char *const *opts, int nopts, int start);

#endif /* PIGPEN_LIGHTBAR_H */
