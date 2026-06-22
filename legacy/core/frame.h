#ifndef PIGPEN_FRAME_H
#define PIGPEN_FRAME_H

/*
 * frame.c -- CP437 box-drawing frames for dialogs, panels, and list windows.
 *
 * Positions are 1-based (row, col), matching the renderer's |[Y/|[X cursor
 * codes. The board is CP437 to the bone (DESIGN.md), so borders use the real
 * line-draw glyphs (0xB3.. / 0xC9..), not ASCII +-|. Border only: the caller
 * owns the interior (so a frame can wrap existing art without clearing it).
 */
#include "render.h"

#define FRAME_SINGLE 0      /* single-line border (0xDA 0xC4 0xBF ...) */
#define FRAME_DOUBLE 1      /* double-line border (0xC9 0xCD 0xBB ...) */

/* Draw a w x h border with its top-left corner at (row, col). w,h >= 2. */
void frame_box(int row, int col, int w, int h, int style);

/* Same, with a title centered on the top border (truncated to fit). */
void frame_titled(int row, int col, int w, int h, int style, const char *title);

#endif /* PIGPEN_FRAME_H */
