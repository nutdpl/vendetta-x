/*
 * lightbar_test.c -- host unit tests for the vertical lightbar selector.
 * The selector is interactive (reads io_getch, writes io_puts), so the test
 * supplies a SCRIPTED input backend (a queue of keystrokes) and a capturing
 * output backend, then asserts the index it returns for each key sequence.
 * This exercises the real navigation loop, not a mock.
 */
#include <stdio.h>
#include "lightbar.h"
#include "pigtypes.h"

/* scripted input */
static int  g_keys[64];
static int  g_klen, g_kpos;
int io_getch(void) { return (g_kpos < g_klen) ? g_keys[g_kpos++] : -1; }

/* captured output (ignored, but the selector calls these) */
void io_putc(pp_u8 c)       { (void)c; }
void io_puts(const char *s) { (void)s; }

static void script(const int *keys, int n)
{
    int i;
    for (i = 0; i < n && i < (int)(sizeof g_keys / sizeof g_keys[0]); i++)
        g_keys[i] = keys[i];
    g_klen = n; g_kpos = 0;
}

static int g_fail;
static void check(const char *name, int cond)
{
    printf("%-44s %s\n", name, cond ? "ok" : "FAIL");
    if (!cond) g_fail = 1;
}

#define DOWN 27, '[', 'B'
#define UP   27, '[', 'A'
#define ENT  '\r'

int main(void)
{
    static const char *opts[3] = { "Apple", "Banana", "Cherry" };
    pp_ctx ctx;                 /* selector ignores ctx ((void)ctx) */

    {   /* Down, Down, Enter -> index 2 */
        int k[] = { DOWN, DOWN, ENT };
        script(k, sizeof k / sizeof k[0]);
        check("down,down,enter -> 2", lightbar_vert(&ctx, opts, 3, 0) == 2);
    }
    {   /* Up from 0 wraps to last (2), Enter */
        int k[] = { UP, ENT };
        script(k, sizeof k / sizeof k[0]);
        check("up-wrap from 0 -> 2", lightbar_vert(&ctx, opts, 3, 0) == 2);
    }
    {   /* Down from 2 wraps to 0, Enter */
        int k[] = { DOWN, ENT };
        script(k, sizeof k / sizeof k[0]);
        check("down-wrap from 2 -> 0", lightbar_vert(&ctx, opts, 3, 2) == 0);
    }
    {   /* first-letter 'c' selects Cherry outright */
        int k[] = { 'c' };
        script(k, sizeof k / sizeof k[0]);
        check("first-letter c -> 2", lightbar_vert(&ctx, opts, 3, 0) == 2);
    }
    {   /* Enter with no movement returns the start index */
        int k[] = { ENT };
        script(k, sizeof k / sizeof k[0]);
        check("enter at start 1 -> 1", lightbar_vert(&ctx, opts, 3, 1) == 1);
    }
    {   /* carrier loss (empty script -> io_getch returns -1) -> -1 */
        script((int *)0, 0);
        check("hangup -> -1", lightbar_vert(&ctx, opts, 3, 0) == -1);
    }

    printf("\nlightbar_test: %s\n", g_fail ? "FAILED" : "all passed");
    return g_fail;
}
