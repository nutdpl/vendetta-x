/*
 * acs.c -- recursive-descent ACS evaluator. See acs.h for the grammar.
 */
#include <string.h>
#include "acs.h"

typedef struct {
    const char    *p;
    const pp_user *u;
} acs_st;

static int parse_or(acs_st *s);   /* fwd */

static int up(int c)      { return (c >= 'a' && c <= 'z') ? c - 32 : c; }
static int is_alpha(int c){ c = up(c); return c >= 'A' && c <= 'Z'; }
static int is_digit(int c){ return c >= '0' && c <= '9'; }

static void skip(acs_st *s) { while (*s->p == ' ' || *s->p == '\t') s->p++; }

static int read_num(acs_st *s)
{
    int n = 0;
    while (is_digit((unsigned char)*s->p)) { n = n * 10 + (*s->p - '0'); s->p++; }
    return n;
}

static void read_id(acs_st *s, char *buf, int max)
{
    int i = 0;
    while (is_alpha((unsigned char)*s->p) && i < max - 1) {
        buf[i++] = (char)up((unsigned char)*s->p);
        s->p++;
    }
    buf[i] = '\0';
}

/* bit for a letter A..P, else 0 */
static unsigned letter_bit(int c)
{
    c = up(c);
    return (c >= 'A' && c <= 'P') ? (unsigned)(1u << (c - 'A')) : 0u;
}

static int cmp_level(int op1, int op2, int val, int n)
{
    if (op1 == '>') return op2 ? (val >= n) : (val > n);
    if (op1 == '<') return op2 ? (val <= n) : (val < n);
    if (op1 == '=') return val == n;
    return 0;
}

static int parse_atom(acs_st *s)
{
    skip(s);
    if (*s->p == '(') {
        int v;
        s->p++;
        v = parse_or(s);
        skip(s);
        if (*s->p == ')') s->p++;
        return v;
    }
    if (*s->p == '-') { s->p++; return 1; }

    if (is_alpha((unsigned char)*s->p)) {
        char id[8];
        read_id(s, id, (int)sizeof id);

        if (strcmp(id, "SYSOP") == 0) return s->u->sl >= 255;
        if (strcmp(id, "ANY") == 0 || strcmp(id, "TRUE") == 0) return 1;

        if (strcmp(id, "SL") == 0 || strcmp(id, "DSL") == 0) {
            int isdsl = (id[0] == 'D');
            int op1, op2 = 0, n, val;
            skip(s);
            op1 = *s->p; if (op1) s->p++;
            if (*s->p == '=') { op2 = '='; s->p++; }
            n = read_num(s);
            val = isdsl ? (int)s->u->dsl : (int)s->u->sl;
            return cmp_level(op1, op2, val, n);
        }

        if (strcmp(id, "AR") == 0 || strcmp(id, "DAR") == 0 || strcmp(id, "R") == 0) {
            unsigned field, b;
            skip(s);
            if (*s->p == ':') s->p++;
            skip(s);
            b = letter_bit((unsigned char)*s->p);
            if (*s->p) s->p++;
            field = (strcmp(id, "AR") == 0)  ? s->u->ar
                  : (strcmp(id, "DAR") == 0) ? s->u->dar
                                             : s->u->restr;
            return (field & b) ? 1 : 0;
        }
        return 0;                                   /* unknown keyword -> false */
    }
    return 0;
}

static int parse_not(acs_st *s)
{
    skip(s);
    if (*s->p == '!') { s->p++; return !parse_not(s); }
    return parse_atom(s);
}

static int parse_and(acs_st *s)
{
    int v = parse_not(s);
    for (;;) {
        skip(s);
        if (*s->p == '&') { int r; s->p++; r = parse_not(s); v = v && r; }
        else break;
    }
    return v;
}

static int parse_or(acs_st *s)
{
    int v = parse_and(s);
    for (;;) {
        skip(s);
        if (*s->p == '|') { int r; s->p++; r = parse_and(s); v = v || r; }
        else break;
    }
    return v;
}

int acs_eval(const char *acs, const pp_user *u)
{
    acs_st s;
    if (acs == (const char *)0 || *acs == '\0') return 1;
    s.p = acs; s.u = u;
    return parse_or(&s) ? 1 : 0;
}
