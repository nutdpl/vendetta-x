/*
 * lbmenu_test.c -- host unit tests for the lightbar menu engine. `make test`.
 * Drives the REAL navigation loop with a scripted keystroke backend and a
 * capturing output backend: asserts the chosen index for each key sequence,
 * and that the initial paint highlights the selected option at its position.
 */
#include <stdio.h>
#include <string.h>
#include "lbmenu.h"

/* scripted input */
static int g_keys[64], g_klen, g_kpos;
int io_getch(void) { return (g_kpos < g_klen) ? g_keys[g_kpos++] : -1; }

/* captured output */
static char g_buf[4096];
static int  g_len;
void io_putc(pp_u8 c)       { if (g_len < (int)sizeof(g_buf) - 1) g_buf[g_len++] = (char)c; }
void io_puts(const char *s) { while (*s) io_putc((pp_u8)*s++); }
static void cap_reset(void) { g_len = 0; }
static const char *cap(void){ g_buf[g_len] = '\0'; return g_buf; }

static void script(const int *k, int n)
{
    int i;
    for (i = 0; i < n && i < (int)(sizeof g_keys / sizeof g_keys[0]); i++) g_keys[i] = k[i];
    g_klen = n; g_kpos = 0; cap_reset();
}

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define DOWN  27, '[', 'B'
#define UP    27, '[', 'A'
#define RIGHT 27, '[', 'C'
#define LEFT  27, '[', 'D'
#define ENT   '\r'

static lb_opt OPTS[3];
static void setup(void)
{
    OPTS[0].row = 5;  OPTS[0].col = 10; OPTS[0].key = 'F'; strcpy(OPTS[0].label, "Files");
    OPTS[1].row = 7;  OPTS[1].col = 10; OPTS[1].key = 'M'; strcpy(OPTS[1].label, "Mail");
    OPTS[2].row = 9;  OPTS[2].col = 10; OPTS[2].key = 'G'; strcpy(OPTS[2].label, "Goodbye");
}

int main(void)
{
    pp_ctx ctx;                 /* engine ignores ctx ((void)ctx) */
    setup();

    {   int k[] = { DOWN, DOWN, ENT };
        script(k, sizeof k / sizeof k[0]);
        check("down,down,enter -> 2", lbmenu_run(&ctx, OPTS, 3, 0) == 2); }

    {   int k[] = { UP, ENT };       /* spatial nav: at the top edge, stay put */
        script(k, sizeof k / sizeof k[0]);
        check("up at top edge stays 0", lbmenu_run(&ctx, OPTS, 3, 0) == 0); }

    {   int k[] = { 'm' };
        script(k, sizeof k / sizeof k[0]);
        check("hotkey m -> 1", lbmenu_run(&ctx, OPTS, 3, 0) == 1); }

    {   int k[] = { ENT };
        script(k, sizeof k / sizeof k[0]);
        check("enter at start 2 -> 2", lbmenu_run(&ctx, OPTS, 3, 2) == 2); }

    {   script((int *)0, 0);
        check("hangup -> -1", lbmenu_run(&ctx, OPTS, 3, 0) == -1); }

    {   /* initial paint: option 1 (start) highlighted at its (row,col) */
        int k[] = { ENT };
        script(k, sizeof k / sizeof k[0]);
        lbmenu_run(&ctx, OPTS, 3, 1);
        check("paints selected highlighted in place",
              strstr(cap(), "\x1b[7;10H\x1b[1;37;41mMail") != (char *)0);
        check("paints an unselected normal",
              strstr(cap(), "\x1b[5;10H\x1b[0;37;40mFiles") != (char *)0); }

    {   /* 2x2 grid: idx0=(5,10) idx1=(5,40) idx2=(7,10) idx3=(7,40).
         * right -> 1, down -> 3, left -> 2, enter -> 2. */
        lb_opt G[4];
        int k[] = { RIGHT, DOWN, LEFT, ENT };
        G[0].row = 5; G[0].col = 10; G[0].key = 'a'; strcpy(G[0].label, "A");
        G[1].row = 5; G[1].col = 40; G[1].key = 'b'; strcpy(G[1].label, "B");
        G[2].row = 7; G[2].col = 10; G[2].key = 'c'; strcpy(G[2].label, "C");
        G[3].row = 7; G[3].col = 40; G[3].key = 'd'; strcpy(G[3].label, "D");
        script(k, sizeof k / sizeof k[0]);
        check("grid right,down,left,enter -> 2", lbmenu_run(&ctx, G, 4, 0) == 2); }

    {   /* right from a left-column option lands in the right column */
        lb_opt G[2];
        int k[] = { RIGHT, ENT };
        G[0].row = 5; G[0].col = 10; G[0].key = 'a'; strcpy(G[0].label, "A");
        G[1].row = 5; G[1].col = 40; G[1].key = 'b'; strcpy(G[1].label, "B");
        script(k, sizeof k / sizeof k[0]);
        check("right -> other column", lbmenu_run(&ctx, G, 2, 0) == 1); }

    printf("\nlbmenu_test: %s\n", g_fail ? "FAILED" : "all passed");
    return g_fail;
}
