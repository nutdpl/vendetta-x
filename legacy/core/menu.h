#ifndef PIGPEN_MENU_H
#define PIGPEN_MENU_H

/*
 * Data-driven menu engine (DESIGN.md phase 3). A menu is a data file
 * data/<NAME>.MNU -- a SCREEN to display plus hotkey -> command + ACS rows.
 * New menus and re-wired hotkeys need no recompile (the config program will
 * edit these in phase 4). The engine implements navigation itself (GOTO,
 * DISPLAY, LOGOFF); every other command verb is handed to a caller-provided
 * handler so feature subsystems (last callers, oneliners, message bases)
 * live outside the engine.
 *
 *   data/MAIN.MNU:
 *     SCREEN art/mainmenu.ans
 *     KEY l - LASTCALL
 *     KEY i - DISPLAY art/sysinfo.pp
 *     KEY x - GOTO PREFS
 *     KEY ! SYSOP STUB the sysop menu
 *     KEY g - LOGOFF
 *   (KEY <hotkey> <acs> <command> [arg]; <acs> is an ACS expression -- see
 *    acs.h -- e.g. "-", "SL>=20", "SYSOP", "SL>=10&AR:B"; no spaces in <acs>.)
 */
#include "pigtypes.h"
#include "render.h"
#include "userbase.h"

typedef enum { MENU_OK = 0, MENU_LOGOFF = 1 } menu_result;

/* Handles command verbs the engine does not implement itself. */
typedef menu_result (*menu_cmd_fn)(const char *cmd, const char *arg,
                                   pp_ctx *ctx, void *user);

/* Run menus from `start` (e.g. "MAIN") until LOGOFF or carrier loss. Items
 * whose ACS the caller does not satisfy are hidden. `user` is also handed to
 * the command handler. */
void menu_run(const char *start, pp_ctx *ctx, const pp_user *user,
              menu_cmd_fn handler);

#endif /* PIGPEN_MENU_H */
