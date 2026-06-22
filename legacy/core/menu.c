/*
 * menu.c -- the data-driven menu engine. See menu.h for the file format.
 */
#include <stdio.h>
#include <string.h>
#include "menu.h"
#include "ppio.h"
#include "strtab.h"
#include "acs.h"
#include "lbmenu.h"

#define CLEAR "\x1b[2J\x1b[H"
#define MENU_MAX_ITEMS 24

typedef struct {
    char  key;
    char  acs[32];          /* ACS expression, e.g. "-", "SL>=20", "SYSOP" */
    char  cmd[16];
    char  arg[64];
} menu_item;

typedef struct {
    char      screen[64];
    int       n;
    int       lightbar;     /* STYLE LIGHTBAR -> run a moving bar over markers */
    menu_item items[MENU_MAX_ITEMS];
} menu_def;

static int load(const char *name, menu_def *m)
{
    char path[80];
    char line[160];
    FILE *f;

    m->n = 0;
    m->lightbar = 0;
    m->screen[0] = '\0';
    sprintf(path, "data/%s.MNU", name);
    f = fopen(path, "rb");
    if (f == (FILE *)0) return 1;

    while (fgets(line, (int)sizeof line, f)) {
        char *p = line;
        while (*p == ' ' || *p == '\t') p++;
        if (*p == '#' || *p == '\0' || *p == '\r' || *p == '\n') continue;

        if (strncmp(p, "SCREEN", 6) == 0) {
            sscanf(p, "SCREEN %63s", m->screen);
        } else if (strncmp(p, "STYLE", 5) == 0) {
            char st[16];
            st[0] = '\0';
            sscanf(p, "STYLE %15s", st);
            if (strcmp(st, "LIGHTBAR") == 0 || strcmp(st, "lightbar") == 0) m->lightbar = 1;
        } else if (strncmp(p, "KEY", 3) == 0 && m->n < MENU_MAX_ITEMS) {
            char key, acs[32], cmd[16], arg[64];
            int got;
            arg[0] = '\0';
            got = sscanf(p, "KEY %c %31s %15s %63[^\r\n]", &key, acs, cmd, arg);
            if (got >= 3) {
                menu_item *it = &m->items[m->n++];
                it->key = key;
                strncpy(it->acs, acs, sizeof it->acs - 1); it->acs[sizeof it->acs - 1] = '\0';
                strncpy(it->cmd, cmd, sizeof it->cmd - 1); it->cmd[sizeof it->cmd - 1] = '\0';
                strncpy(it->arg, arg, sizeof it->arg - 1); it->arg[sizeof it->arg - 1] = '\0';
            }
        }
    }
    fclose(f);
    return 0;
}

static int lower(int c) { return (c >= 'A' && c <= 'Z') ? c + 32 : c; }

static menu_item *find(menu_def *m, int key, const pp_user *user)
{
    int i, k = lower(key);
    for (i = 0; i < m->n; i++)
        if (lower(m->items[i].key) == k && acs_eval(m->items[i].acs, user))
            return &m->items[i];
    return (menu_item *)0;
}

/* While a STYLE LIGHTBAR screen renders, the renderer reports each |{R,C,K,..}
 * option marker here. We keep only those whose hotkey maps to an item the
 * caller's ACS allows, so disallowed options never join the moving bar. */
static lb_opt        g_lb[MENU_MAX_ITEMS];
static int           g_nlb;
static menu_def     *g_lbmenu;
static const pp_user *g_lbuser;

static void lb_collect(void *u, int row, int col, int key, const char *label)
{
    (void)u;
    if (g_nlb < MENU_MAX_ITEMS && find(g_lbmenu, key, g_lbuser)) {
        lb_opt *o = &g_lb[g_nlb++];
        o->row = row; o->col = col; o->key = key;
        strncpy(o->label, label, LB_LABEL - 1);
        o->label[LB_LABEL - 1] = '\0';
    }
}

static void show_screen(const char *path, pp_ctx *ctx)
{
    FILE *f = fopen(path, "rb");
    size_t n;
    if (f == (FILE *)0) { io_puts("\x1b[0m\r\n  [menu screen missing]\r\n"); return; }
    n = strlen(path);
    if (n >= 3 && path[n - 3] == '.' && lower(path[n - 2]) == 'p' && lower(path[n - 1]) == 'p')
        render_tpl(f, ctx);          /* .pp template: parse pipe codes/tokens */
    else
        render_raw(f);               /* finished .ans art */
    fclose(f);
}

static void pause_key(pp_ctx *ctx)
{
    render_strn(pps_get(PPS_PRESS_KEY), ctx);
    io_getch();
}

void menu_run(const char *start, pp_ctx *ctx, const pp_user *user,
              menu_cmd_fn handler)
{
    char cur[16];
    menu_def m;
    int redraw = 1;

    strncpy(cur, start, sizeof cur - 1);
    cur[sizeof cur - 1] = '\0';

    for (;;) {
        menu_item *it;
        int c;

        if (load(cur, &m) != 0) {
            io_puts("\x1b[0m\r\n  [no such menu]\r\n");
            return;
        }

        if (m.lightbar) {
            /* Render the sysop's ANSI, collecting its |{...} option markers,
             * then run the moving bar over them. The chosen option's hotkey
             * flows into the same dispatch the hotkey menus use. */
            int sel;
            g_lbmenu = &m; g_lbuser = user; g_nlb = 0;
            ctx->bar = lb_collect; ctx->bar_user = (void *)0;
            io_puts(CLEAR); show_screen(m.screen, ctx);
            ctx->bar = (pp_bar_fn)0;
            redraw = 1;                          /* a hotkey GOTO target redraws fresh */
            if (g_nlb == 0) {                    /* no selectable options: don't spin */
                c = io_getch();
                if (c < 0) return;
                continue;
            }
            sel = lbmenu_run(ctx, g_lb, g_nlb, 0);
            if (sel < 0) return;                 /* carrier lost */
            c = g_lb[sel].key;
        } else {
            if (redraw) { io_puts(CLEAR); show_screen(m.screen, ctx); redraw = 0; }
            c = io_getch();
            if (c < 0) return;                   /* carrier lost */
        }

        it = find(&m, c, user);
        if (it == (menu_item *)0) continue;      /* unbound key: ignore */

        if (strcmp(it->cmd, "GOTO") == 0) {
            strncpy(cur, it->arg, sizeof cur - 1); cur[sizeof cur - 1] = '\0';
            redraw = 1;
        } else if (strcmp(it->cmd, "DISPLAY") == 0) {
            io_puts(CLEAR);
            show_screen(it->arg, ctx);
            pause_key(ctx);
            redraw = 1;
        } else if (strcmp(it->cmd, "LOGOFF") == 0) {
            return;
        } else if (handler != (menu_cmd_fn)0) {
            if (handler(it->cmd, it->arg, ctx, (void *)user) == MENU_LOGOFF) return;
            redraw = 1;
        }
    }
}
