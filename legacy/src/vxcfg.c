/*
 * vxcfg.c -- VXCFG, the Vendetta/X sysop control panel.
 *
 * A standalone, full-screen CP437 config editor for CONFIG.DAT. This is the
 * showpiece: a bordered AMi/X-style panel with a red accent, a category menu
 * on the left and the editable fields of the selected category on the right,
 * a title bar, a status/help bar and an inline coloured input box for editing.
 *
 * Talks only to the IO backend (io_local console) and the renderer; it owns
 * the screen as raw CP437 bytes + ANSI escapes. Strict C89.
 *
 *   build:
 *     cc -std=c89 -pedantic -Wall -Wextra -Icore -Iio \
 *        src/vxcfg.c core/config.c core/render.c core/lightbar.c \
 *        io/io_local.c -o /tmp/vxcfg
 *
 *   non-interactive smoke (edit board name, save, quit):
 *     printf '\r...keys...' | /tmp/vxcfg
 *   (exact key sequence is documented at the bottom of this file).
 */
#include <stdio.h>
#include <string.h>
#include "config.h"
#include "render.h"
#include "ppio.h"

#define CFG_PATH "data/CONFIG.DAT"
#define WATTCP_PATH "WATTCP.CFG"

/* ------------------------------------------------------------------------- *
 * Screen geometry. An 80x24 classic ANSI canvas.
 * ------------------------------------------------------------------------- */
#define SCR_W      80
#define SCR_H      24
#define BOX_TOP     3      /* first row inside the main frame border */
#define BOX_LEFT    2      /* first col of the frame                  */
#define BOX_RIGHT  79
#define BOX_BOTTOM 21
#define CAT_COL     4      /* category labels start here              */
#define CAT_ROW     6      /* first category row                      */
#define FLD_LBLCOL 26      /* field label column                      */
#define FLD_VALCOL 46      /* field value column                      */
#define FLD_ROW     6      /* first field row                         */
#define FLD_VALW   30      /* width of the value box                  */

/* ------------------------------------------------------------------------- *
 * CP437 box-drawing bytes (written raw, never UTF-8).
 * ------------------------------------------------------------------------- */
#define B_HORZ  0xC4
#define B_VERT  0xB3
#define B_TL    0xDA
#define B_TR    0xBF
#define B_BL    0xC0
#define B_BR    0xD9
#define B_TUP   0xC2   /* top tee */
#define B_TDN   0xC1   /* bottom tee */
#define B_TLT   0xB4   /* left-pointing tee */
#define B_TRT   0xC3   /* right-pointing tee */
#define B_BLK   0xB0   /* light shade */
#define B_BLK2  0xB1   /* medium shade */

/* ------------------------------------------------------------------------- *
 * ANSI helpers. We position absolutely with ESC[r;cH (1-based).
 * ------------------------------------------------------------------------- */
static void gotoxy(int row, int col)
{
    char buf[16];
    sprintf(buf, "\x1b[%d;%dH", row, col);
    io_puts(buf);
}

static void cls(void)        { io_puts("\x1b[2J\x1b[H"); }
static void attr_off(void)   { io_puts("\x1b[0m"); }
static void cursor_off(void) { io_puts("\x1b[?25l"); }
static void cursor_on(void)  { io_puts("\x1b[?25h"); }

/* Emit n copies of one raw byte. */
static void rep(int ch, int n)
{
    while (n-- > 0)
        io_putc((pp_u8)ch);
}

/* ------------------------------------------------------------------------- *
 * The model: categories, and the fields inside each category.
 * ------------------------------------------------------------------------- */
enum {
    FT_STR,      /* free-text string, capped to .width             */
    FT_U8,       /* numeric 0..255                                 */
    FT_U16,      /* numeric 0..65535                               */
    FT_FLAG,     /* one bit of cfg.flags, toggled                  */
    FT_ENUM      /* cycle through a fixed option list (ENTER)      */
};

typedef struct {
    const char *label;
    int         type;
    int         width;     /* string capacity (incl. NUL) or display width */
    pp_u32      mask;       /* FT_FLAG: which bit                          */
    int         cat;       /* owning category index                       */
} field_t;

enum { CAT_IDENTITY, CAT_ACCESS, CAT_LIMITS, CAT_NETWORK, CAT_COUNT };

static const char *const CAT_NAME[CAT_COUNT] = {
    "Board Identity",
    "Access Policy",
    "Limits & Timers",
    "Network"
};

/* kept short -- the category column is narrow (truncated to fit regardless) */
static const char *const CAT_BLURB[CAT_COUNT] = {
    "Who you are",
    "Who gets in",
    "Caps & timers",
    "NIC, IP, port"
};

/* Field table. Order matters: fields are grouped by category, and the field
 * index is used directly by field_value/num_get/str_ptr/enum_* below -- so new
 * fields are APPENDED, never inserted, to keep those index switches stable. */
static const field_t FIELDS[] = {
    { "Board Name",      FT_STR,  PP_BOARD_MAX,  0,                    CAT_IDENTITY },  /* 0 */
    { "Sysop Handle",    FT_STR,  PP_SYSOP_MAX,  0,                    CAT_IDENTITY },  /* 1 */
    { "Location",        FT_STR,  PP_CFGLOC_MAX, 0,                    CAT_IDENTITY },  /* 2 */

    { "Closed System",   FT_FLAG, 0, PP_CFG_CLOSED_SYSTEM,             CAT_ACCESS   },  /* 3 */
    { "Allow Aliases",   FT_FLAG, 0, PP_CFG_ALLOW_ALIASES,            CAT_ACCESS   },  /* 4 */
    { "New User SL",     FT_U8,   0, 0,                                CAT_ACCESS   },  /* 5 */
    { "New User DSL",    FT_U8,   0, 0,                                CAT_ACCESS   },  /* 6 */

    { "Max Msgs / Area", FT_U16,  0, 0,                                CAT_LIMITS   },  /* 7 */
    { "Idle Minutes",    FT_U16,  0, 0,                                CAT_LIMITS   },  /* 8 */

    { "Network Card",    FT_ENUM, 0, 0,                                CAT_NETWORK  },  /* 9  */
    { "IP Mode",         FT_ENUM, 0, 0,                                CAT_NETWORK  },  /* 10 */
    { "Packet Int",      FT_ENUM, 0, 0,                                CAT_NETWORK  },  /* 11 */
    { "Telnet Port",     FT_U16,  0, 0,                                CAT_NETWORK  },  /* 12 */
    { "IP Address",      FT_STR,  PP_IP_MAX, 0,                        CAT_NETWORK  },  /* 13 */
    { "Subnet Mask",     FT_STR,  PP_IP_MAX, 0,                        CAT_NETWORK  },  /* 14 */
    { "Gateway",         FT_STR,  PP_IP_MAX, 0,                        CAT_NETWORK  },  /* 15 */
    { "DNS Server",      FT_STR,  PP_IP_MAX, 0,                        CAT_NETWORK  }   /* 16 */
};
#define NFIELDS ((int)(sizeof FIELDS / sizeof FIELDS[0]))

/* ------------------------------------------------------------------------- *
 * Value formatting: render a field's current value into a display string.
 * ------------------------------------------------------------------------- */
static unsigned num_get(const pp_config *cfg, int fi);           /* fwd */
static int  enum_get(const pp_config *cfg, int fi);              /* fwd */
static void enum_label(int fi, int v, char *out, int outsz);     /* fwd */

static void field_value(const pp_config *cfg, int fi, char *out, int outsz)
{
    const field_t *f = &FIELDS[fi];
    out[0] = '\0';
    switch (f->type) {
    case FT_STR:
        switch (fi) {
        case 0:  strncpy(out, cfg->board_name, (size_t)outsz - 1); break;
        case 1:  strncpy(out, cfg->sysop_name, (size_t)outsz - 1); break;
        case 2:  strncpy(out, cfg->location,   (size_t)outsz - 1); break;
        case 13: strncpy(out, cfg->ip,      (size_t)outsz - 1); break;
        case 14: strncpy(out, cfg->netmask, (size_t)outsz - 1); break;
        case 15: strncpy(out, cfg->gateway, (size_t)outsz - 1); break;
        case 16: strncpy(out, cfg->dns,     (size_t)outsz - 1); break;
        default: break;
        }
        out[outsz - 1] = '\0';
        break;
    case FT_U8:
    case FT_U16:
        sprintf(out, "%u", num_get(cfg, fi));
        break;
    case FT_FLAG:
        strcpy(out, (cfg->flags & f->mask) ? "[X] ON" : "[ ] off");
        break;
    case FT_ENUM:
        enum_label(fi, enum_get(cfg, fi), out, outsz);
        break;
    default:
        break;
    }
}

/* Numeric fields are addressed by index so there is no ambiguity. */
static unsigned num_get(const pp_config *cfg, int fi)
{
    switch (fi) {
    case 5:  return cfg->new_user_sl;
    case 6:  return cfg->new_user_dsl;
    case 7:  return cfg->max_msgs;
    case 8:  return cfg->idle_minutes;
    case 12: return cfg->telnet_port;
    default: return 0;
    }
}

static void num_set(pp_config *cfg, int fi, unsigned v)
{
    switch (fi) {
    case 5:  cfg->new_user_sl  = (pp_u8)(v > 255u ? 255u : v); break;
    case 6:  cfg->new_user_dsl = (pp_u8)(v > 255u ? 255u : v); break;
    case 7:  cfg->max_msgs     = (pp_u16)(v > 65535u ? 65535u : v); break;
    case 8:  cfg->idle_minutes = (pp_u16)(v > 65535u ? 65535u : v); break;
    case 12: cfg->telnet_port  = (pp_u16)(v > 65535u ? 65535u : v); break;
    default: break;
    }
}

static char *str_ptr(pp_config *cfg, int fi)
{
    switch (fi) {
    case 0:  return cfg->board_name;
    case 1:  return cfg->sysop_name;
    case 2:  return cfg->location;
    case 13: return cfg->ip;
    case 14: return cfg->netmask;
    case 15: return cfg->gateway;
    case 16: return cfg->dns;
    default: return (char *)0;
    }
}

/* FT_ENUM binding: cycle a fixed option list, store the index in a cfg field. */
static int enum_count(int fi)
{
    switch (fi) {
    case 9:  return PP_NET_CARDS;     /* network card */
    case 10: return 2;                /* IP mode: static / dhcp */
    case 11: return 8;                /* packet vector 0x60..0x67 */
    default: return 1;
    }
}
static void enum_label(int fi, int v, char *out, int outsz)
{
    out[0] = '\0';
    switch (fi) {
    case 9:  strncpy(out, cfg_card_name(v), (size_t)outsz - 1); out[outsz - 1] = '\0'; break;
    case 10: strcpy(out, v ? "DHCP" : "Static"); break;
    case 11: sprintf(out, "0x%02X", 0x60 + v); break;
    default: break;
    }
}
static int enum_get(const pp_config *cfg, int fi)
{
    switch (fi) {
    case 9:  return cfg->net_card;
    case 10: return cfg->net_dhcp ? 1 : 0;
    case 11: { int v = (int)cfg->pkt_int - 0x60; return (v < 0 || v > 7) ? 0 : v; }
    default: return 0;
    }
}
static void enum_set(pp_config *cfg, int fi, int v)
{
    switch (fi) {
    case 9:  cfg->net_card = (pp_u8)v; break;
    case 10: cfg->net_dhcp = (pp_u8)(v ? 1 : 0); break;
    case 11: cfg->pkt_int  = (pp_u8)(0x60 + v); break;
    default: break;
    }
}

/* ------------------------------------------------------------------------- *
 * Chrome: title bar, frame, status bar.
 * ------------------------------------------------------------------------- */
static void draw_frame(void)
{
    int r;
    pp_ctx c;
    pp_ctx_init(&c);

    cls();

    /* Title bar (row 1): bright on red, the single accent. */
    gotoxy(1, 1);
    io_puts("\x1b[1;37;41m");
    rep(' ', SCR_W);
    gotoxy(1, 3);
    io_putc((pp_u8)B_BLK2); io_putc((pp_u8)B_BLK); io_putc(' ');
    io_puts("P I G C F G ");
    io_putc((pp_u8)B_VERT);
    io_puts("  Vendetta/X Sysop Control Panel");
    gotoxy(1, SCR_W - 16);
    io_puts("Vendetta/X ");
    io_putc((pp_u8)B_BLK); io_putc((pp_u8)B_BLK2);
    attr_off();

    /* Sub-title rule (row 2). */
    gotoxy(2, 1);
    io_puts("\x1b[0;31;40m");
    rep(B_HORZ, SCR_W);
    attr_off();

    /* Main frame: dim red box drawing on black. */
    io_puts("\x1b[0;31;40m");
    gotoxy(BOX_TOP, BOX_LEFT);
    io_putc((pp_u8)B_TL);
    rep(B_HORZ, BOX_RIGHT - BOX_LEFT - 1);
    io_putc((pp_u8)B_TR);
    for (r = BOX_TOP + 1; r < BOX_BOTTOM; r++) {
        gotoxy(r, BOX_LEFT);  io_putc((pp_u8)B_VERT);
        gotoxy(r, BOX_RIGHT); io_putc((pp_u8)B_VERT);
    }
    gotoxy(BOX_BOTTOM, BOX_LEFT);
    io_putc((pp_u8)B_BL);
    rep(B_HORZ, BOX_RIGHT - BOX_LEFT - 1);
    io_putc((pp_u8)B_BR);

    /* Vertical divider between the category column and the field column. */
    gotoxy(BOX_TOP, FLD_LBLCOL - 2);
    io_putc((pp_u8)B_TUP);
    for (r = BOX_TOP + 1; r < BOX_BOTTOM; r++) {
        gotoxy(r, FLD_LBLCOL - 2);
        io_putc((pp_u8)B_VERT);
    }
    gotoxy(BOX_BOTTOM, FLD_LBLCOL - 2);
    io_putc((pp_u8)B_TDN);
    attr_off();

    /* Column captions. */
    gotoxy(BOX_TOP + 1, CAT_COL);
    io_puts("\x1b[1;31;40mCATEGORY\x1b[0m");
    gotoxy(BOX_TOP + 1, FLD_LBLCOL);
    io_puts("\x1b[1;31;40mFIELD\x1b[0m");
    gotoxy(BOX_TOP + 1, FLD_VALCOL);
    io_puts("\x1b[1;31;40mVALUE\x1b[0m");

    /* A thin rule under the captions on each side. */
    io_puts("\x1b[0;31;40m");
    gotoxy(BOX_TOP + 2, BOX_LEFT + 1);
    rep(B_HORZ, FLD_LBLCOL - 2 - (BOX_LEFT + 1));
    gotoxy(BOX_TOP + 2, FLD_LBLCOL - 2);
    io_putc((pp_u8)B_TRT);
    gotoxy(BOX_TOP + 2, FLD_LBLCOL - 1);
    rep(B_HORZ, BOX_RIGHT - (FLD_LBLCOL - 1));
    attr_off();

    (void)c;
}

static void draw_status(const char *msg)
{
    gotoxy(SCR_H - 1, 1);
    io_puts("\x1b[0;31;40m");
    rep(B_HORZ, SCR_W);
    /* Help line / status text. */
    gotoxy(SCR_H, 1);
    io_puts("\x1b[K");                       /* clear the whole status row */
    io_puts("\x1b[1;37;41m ");
    io_puts(msg ? msg : "");
    io_puts(" \x1b[0m");
    attr_off();
}

/* The fixed key-hints, shown when nothing transient needs saying. */
static void draw_hints(void)
{
    gotoxy(SCR_H, 1);
    io_puts("\x1b[K\x1b[0;37;40m ");
    io_putc((pp_u8)0x18); io_putc((pp_u8)0x19);   /* up/down arrows */
    io_puts(" Move   ");
    io_puts("\x1b[1;37;40mENTER\x1b[0;37;40m Edit   ");
    io_puts("\x1b[1;37;40mTAB\x1b[0;37;40m Switch   ");
    io_puts("\x1b[1;37;41m S \x1b[0;37;40m Save  ");
    io_puts("\x1b[1;37;41m W \x1b[0;37;40m WATTCP  ");
    io_puts("\x1b[1;37;41m Q \x1b[0;37;40m Quit");
    attr_off();
}

/* ------------------------------------------------------------------------- *
 * Body: the category list and the field list for the active category.
 *   focus 0 = category column has the bar, 1 = field column has the bar.
 * ------------------------------------------------------------------------- */
static void draw_categories(int active_cat, int focus)
{
    int i;
    for (i = 0; i < CAT_COUNT; i++) {
        gotoxy(CAT_ROW + i * 2, CAT_COL);
        if (i == active_cat && focus == 0)
            io_puts("\x1b[1;37;41m");          /* selected + focused: white/red */
        else if (i == active_cat)
            io_puts("\x1b[0;31;40m\x1b[1m");    /* selected, unfocused: bright red */
        else
            io_puts("\x1b[0;37;40m");           /* idle: grey */
        io_putc(' ');
        io_puts(CAT_NAME[i]);
        /* pad the bar out to a fixed width for a solid highlight */
        {
            int len = (int)strlen(CAT_NAME[i]);
            int pad = (FLD_LBLCOL - 4 - CAT_COL) - len - 1;
            while (pad-- > 0) io_putc(' ');
        }
        attr_off();
        /* one-line blurb under the active category */
        gotoxy(CAT_ROW + i * 2 + 1, CAT_COL + 1);
        if (i == active_cat) {
            int maxw = (FLD_LBLCOL - 2) - (CAT_COL + 1);   /* stop before the divider */
            const char *b = CAT_BLURB[i];
            int j;
            io_puts("\x1b[0;31;40m");
            for (j = 0; j < maxw && b[j]; j++) io_putc((pp_u8)b[j]);
            for (; j < maxw; j++) io_putc(' ');            /* pad over stale text */
            io_puts("\x1b[0m");
        } else {
            io_puts("\x1b[K");
            /* erase any stale blurb but keep the right-hand frame intact */
            gotoxy(CAT_ROW + i * 2 + 1, CAT_COL + 1);
            {
                int pad = (FLD_LBLCOL - 4) - (CAT_COL + 1);
                while (pad-- > 0) io_putc(' ');
            }
        }
    }
    /* repair the vertical divider the \x1b[K above may have eaten */
    {
        int r;
        io_puts("\x1b[0;31;40m");
        for (r = BOX_TOP + 1; r < BOX_BOTTOM; r++) {
            gotoxy(r, FLD_LBLCOL - 2);
            io_putc((pp_u8)B_VERT);
        }
        gotoxy(r = BOX_TOP, FLD_LBLCOL - 2); io_putc((pp_u8)B_TUP);
        gotoxy(BOX_BOTTOM, FLD_LBLCOL - 2);  io_putc((pp_u8)B_TDN);
        gotoxy(BOX_TOP + 2, FLD_LBLCOL - 2); io_putc((pp_u8)B_TRT);
        attr_off();
        (void)r;
    }
}

/* Indices of the fields that belong to a category, in table order. */
static int cat_fields(int cat, int *idx)
{
    int i, n = 0;
    for (i = 0; i < NFIELDS; i++)
        if (FIELDS[i].cat == cat)
            idx[n++] = i;
    return n;
}

static void draw_value_box(int row, const char *val, int sel)
{
    int len = (int)strlen(val);
    int i;
    gotoxy(row, FLD_VALCOL);
    if (sel)
        io_puts("\x1b[1;37;41m");              /* selected value: white on red */
    else
        io_puts("\x1b[0;33;40m");              /* idle value: amber on black  */
    io_putc(' ');
    for (i = 0; i < FLD_VALW - 2; i++)
        io_putc((pp_u8)(i < len ? (pp_u8)val[i] : ' '));
    io_putc(' ');
    attr_off();
}

static void draw_fields(const pp_config *cfg, int cat, int sel_field, int focus)
{
    int idx[NFIELDS];
    int n = cat_fields(cat, idx);
    int i;
    char val[64];

    /* clear the field area rows first (rows FLD_ROW .. BOX_BOTTOM-1) */
    for (i = FLD_ROW; i < BOX_BOTTOM; i++) {
        gotoxy(i, FLD_LBLCOL);
        {
            int pad = BOX_RIGHT - FLD_LBLCOL;
            io_puts("\x1b[0;40m");
            while (pad-- > 0) io_putc(' ');
        }
    }
    attr_off();

    for (i = 0; i < n; i++) {
        int fi = idx[i];
        int row = FLD_ROW + i * 2;
        int is_sel = (focus == 1 && i == sel_field);

        gotoxy(row, FLD_LBLCOL);
        if (is_sel)
            io_puts("\x1b[1;31;40m\x10 ");      /* a red triangle marker */
        else
            io_puts("\x1b[0;37;40m  ");
        io_puts(FIELDS[fi].label);
        attr_off();

        field_value(cfg, fi, val, (int)sizeof val);
        draw_value_box(row, val, is_sel);
    }
}

/* ------------------------------------------------------------------------- *
 * Inline field editor. Draws a coloured input box at (row,col), pre-filled
 * with `init`, and runs a small read loop: printable chars append, backspace
 * deletes, Enter accepts, ESC cancels. Returns 1 on accept, 0 on cancel.
 * `numeric` restricts input to digits.
 * ------------------------------------------------------------------------- */
static int read_field(int row, int col, int width,
                      char *buf, int cap, int numeric)
{
    int len = (int)strlen(buf);
    int boxw = width;

    if (boxw > cap - 1) boxw = cap - 1;
    if (boxw > FLD_VALW - 2) boxw = FLD_VALW - 2;

    cursor_on();
    for (;;) {
        int c, i;

        /* paint the input box: bright white text on a dim red field */
        gotoxy(row, col);
        io_puts("\x1b[1;37;41m\x10");           /* left cap marker */
        io_puts("\x1b[1;37;41m");
        for (i = 0; i < boxw; i++)
            io_putc((pp_u8)(i < len ? (pp_u8)buf[i] : ' '));
        io_puts("\x1b[0m");
        /* park the cursor at the insertion point */
        gotoxy(row, col + 1 + (len < boxw ? len : boxw));

        c = io_getch();
        if (c < 0) { cursor_off(); return 0; }

        if (c == '\r' || c == '\n') { cursor_off(); buf[len] = '\0'; return 1; }
        if (c == 27)               { cursor_off(); return 0; }
        if (c == 8 || c == 127) {                /* backspace / DEL */
            if (len > 0) len--;
            continue;
        }
        if (c >= ' ' && c < 127 && len < boxw) {
            if (numeric && (c < '0' || c > '9'))
                continue;
            buf[len++] = (char)c;
        }
    }
}

/* Edit whichever field is highlighted. Returns 1 if the value changed. */
static int edit_field(pp_config *cfg, int cat, int sel_field)
{
    int idx[NFIELDS];
    int n = cat_fields(cat, idx);
    int fi, row;
    const field_t *f;

    if (sel_field < 0 || sel_field >= n) return 0;
    fi = idx[sel_field];
    f  = &FIELDS[fi];
    row = FLD_ROW + sel_field * 2;

    if (f->type == FT_FLAG) {
        cfg->flags ^= f->mask;                   /* toggle the bit in place */
        return 1;
    }

    if (f->type == FT_ENUM) {                    /* ENTER cycles to the next option */
        int n2 = enum_count(fi);
        enum_set(cfg, fi, (enum_get(cfg, fi) + 1) % n2);
        return 1;
    }

    if (f->type == FT_STR) {
        char buf[PP_BOARD_MAX];
        char *p = str_ptr(cfg, fi);
        if (!p) return 0;
        strncpy(buf, p, sizeof buf - 1);
        buf[sizeof buf - 1] = '\0';
        draw_status("Editing -- type, BACKSPACE deletes, ENTER saves, ESC cancels");
        if (read_field(row, FLD_VALCOL, f->width - 1, buf,
                       (int)sizeof buf, 0)) {
            strncpy(p, buf, (size_t)f->width - 1);
            p[f->width - 1] = '\0';
            return 1;
        }
        return 0;
    }

    /* numeric */
    {
        char buf[8];
        unsigned v;
        sprintf(buf, "%u", num_get(cfg, fi));
        draw_status("Enter a number, ENTER saves, ESC cancels");
        if (read_field(row, FLD_VALCOL,
                       (f->type == FT_U8) ? 3 : 5,
                       buf, (int)sizeof buf, 1)) {
            v = 0;
            sscanf(buf, "%u", &v);
            num_set(cfg, fi, v);
            return 1;
        }
        return 0;
    }
}

/* ------------------------------------------------------------------------- *
 * A small centred confirm prompt for [Q]uit.  Returns 1 for yes.
 * ------------------------------------------------------------------------- */
static int confirm(const char *q)
{
    int c;
    draw_status(q);
    for (;;) {
        c = io_getch();
        if (c < 0) return 1;
        if (c == 'y' || c == 'Y') return 1;
        if (c == 'n' || c == 'N' || c == 27) return 0;
    }
}

/* ------------------------------------------------------------------------- *
 * main.
 * ------------------------------------------------------------------------- */
int main(void)
{
    pp_config cfg;
    int cat = 0;          /* active category                 */
    int sel = 0;          /* selected field within category  */
    int focus = 1;        /* 0 = category column, 1 = fields */
    int dirty = 0;        /* unsaved changes?                */
    int running = 1;
    int idx[NFIELDS];
    int nf;

    if (io_init() != 0)
        return 1;

    cfg_load(CFG_PATH, &cfg);     /* defaults if absent / damaged */

    nf = cat_fields(cat, idx);

    cursor_off();
    draw_frame();
    draw_categories(cat, focus);
    draw_fields(&cfg, cat, sel, focus);
    draw_hints();

    while (running) {
        int c = io_getch();
        if (c < 0) break;

        if (c == 27) {                            /* ESC: arrow keys or quit */
            int d = io_getch();
            if (d == '[' || d == 'O') {
                int e = io_getch();
                if (e == 'A') {                   /* up */
                    if (focus == 0) cat = (cat > 0) ? cat - 1 : CAT_COUNT - 1;
                    else            sel = (sel > 0) ? sel - 1 : nf - 1;
                } else if (e == 'B') {            /* down */
                    if (focus == 0) cat = (cat < CAT_COUNT - 1) ? cat + 1 : 0;
                    else            sel = (sel < nf - 1) ? sel + 1 : 0;
                } else if (e == 'D') {            /* left -> categories */
                    focus = 0;
                } else if (e == 'C') {            /* right -> fields */
                    focus = 1;
                }
                if (focus == 0) sel = 0;
                nf = cat_fields(cat, idx);
                if (sel >= nf) sel = nf - 1;
                draw_categories(cat, focus);
                draw_fields(&cfg, cat, sel, focus);
                draw_hints();
            } else if (d < 0) {
                break;
            } else {
                /* bare ESC -> treat as quit request */
                if (confirm(dirty
                        ? "Quit with UNSAVED changes? (Y/N)"
                        : "Quit VXCFG? (Y/N)")) {
                    running = 0;
                } else {
                    draw_hints();
                }
            }
            continue;
        }

        switch (c) {
        case '\t':                                /* TAB toggles the focus */
            focus = !focus;
            if (focus == 0) sel = 0;
            nf = cat_fields(cat, idx);
            if (sel >= nf) sel = nf - 1;
            draw_categories(cat, focus);
            draw_fields(&cfg, cat, sel, focus);
            draw_hints();
            break;

        case '\r':
        case '\n':
            if (focus == 0) {
                focus = 1;                        /* Enter on a category dives in */
                sel = 0;
                nf = cat_fields(cat, idx);
                draw_categories(cat, focus);
                draw_fields(&cfg, cat, sel, focus);
                draw_hints();
            } else {
                if (edit_field(&cfg, cat, sel))
                    dirty = 1;
                draw_fields(&cfg, cat, sel, focus);
                draw_hints();
            }
            break;

        case 's': case 'S':
            if (cfg_save(CFG_PATH, &cfg) == 0) {
                dirty = 0;
                draw_status("Saved to " CFG_PATH " -- the pen is configured.");
            } else {
                draw_status("SAVE FAILED -- could not write " CFG_PATH);
            }
            break;

        case 'w': case 'W':                       /* generate the Watt-32 config */
            if (cfg_write_wattcp(WATTCP_PATH, &cfg) == 0)
                draw_status("Wrote " WATTCP_PATH " -- hand it to Watt-32 on the 486.");
            else
                draw_status("WRITE FAILED -- could not write " WATTCP_PATH);
            break;

        case 'q': case 'Q':
            if (confirm(dirty
                    ? "Quit with UNSAVED changes? (Y/N)"
                    : "Quit VXCFG? (Y/N)")) {
                running = 0;
            } else {
                draw_hints();
            }
            break;

        default:
            break;
        }
    }

    cursor_on();
    attr_off();
    gotoxy(SCR_H, 1);
    io_puts("\x1b[2J\x1b[H");
    io_puts("VXCFG closed.\r\n");
    io_shutdown();
    return 0;
}

/*
 * NON-INTERACTIVE SMOKE KEY SEQUENCE
 * ----------------------------------
 * On start the bar is on the first field (Board Name) of the first category
 * (Board Identity), focus = fields. So:
 *
 *   \r              Enter -> open the inline editor on "Board Name"
 *   PIGTEST\r       type the new name, Enter accepts
 *   S               save to data/CONFIG.DAT
 *   Q y             quit, confirm yes
 *
 * i.e.   printf '\rPIGTEST\rSQy' | /tmp/vxcfg
 * then read back data/CONFIG.DAT: bytes 0..3 are "PCFG", the board name at
 * record offset 16 (header) + 0 is "PIGTEST".
 */
