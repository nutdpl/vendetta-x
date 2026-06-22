#ifndef PIGPEN_EDITOR_H
#define PIGPEN_EDITOR_H

/*
 * Full-screen message editor (DESIGN.md phase 3). The frame is a sysop-owned
 * ANSI header template (art/<hdr>.pp) drawn at the top, with |MF/|MT/|MS tokens
 * for from/to/subject -- "totally custom ANSI headers." Below it the caller
 * types the message body in a word-wrapping region. Pure ppio/render, so it
 * works identically on the local console and over telnet.
 *
 * Keys: type to compose, Enter for a new line, Backspace to erase,
 *       ^Z save, ^X abort. Returns 1 if saved (body CRLF-joined into out,
 *       NUL-terminated, <= outsz), 0 if aborted or empty.
 */
#include "render.h"

int editor_run(pp_ctx *ctx, const char *hdr_template,
               const char *from, const char *to, const char *subj,
               char *out, int outsz);

#endif /* PIGPEN_EDITOR_H */
