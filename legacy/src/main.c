/*
 * main.c -- Vendetta/X session: greeting -> identify caller -> the board.
 *
 * The board now remembers people. A handle is looked up in USER.DAT; returning
 * callers get their call count bumped, new callers are created (handle-only --
 * "no nup"). Every call is pushed to the persisted last-callers log. Identity
 * and last-callers are real data, not canned.
 *
 * "auto" mode runs non-interactively (handle "newbie", canned location) for
 * the headless build/connection harness.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include "ppio.h"
#include "render.h"
#include "strtab.h"
#include "userbase.h"
#include "callers.h"
#include "oneliner.h"
#include "msgbase.h"
#include "editor.h"
#include "lightbar.h"
#include "config.h"
#include "email.h"
#include "voting.h"
#include "filebase.h"
#include "bbslist.h"
#include "gfiles.h"
#include "qscan.h"
#include "trashcan.h"
#include "syslog.h"
#include "node.h"
#include "page.h"
#include "door.h"
#include "xmodem.h"
#include "qwk.h"
#include "acs.h"
#include "menu.h"
#include "lbmenu.h"

#define ART_MATRIX   "art/matrix.pp"
#define ART_GREETING "art/greeting.ans"
#define ART_WELCOME  "art/welcome.pp"
#define ART_MAINMENU "art/mainmenu.ans"
#define ART_MSGEDIT  "art/msgedit.pp"
#define ART_NEWUSER  "art/newuser.pp"
#define UL_HDR       "art/userlist.hdr"
#define UL_LINE      "art/userlist.lin"
#define UL_FTR       "art/userlist.ftr"
#define LC_HDR       "art/lastcall.hdr"
#define LC_LINE      "art/lastcall.lin"
#define LC_FTR       "art/lastcall.ftr"
#define OL_HDR       "art/oneliner.hdr"
#define OL_LINE      "art/oneliner.lin"
#define OL_FTR       "art/oneliner.ftr"
#define STR_FILE     "data/VENDX.STR"
#define USER_FILE    "data/USER.DAT"
#define CALL_FILE    "data/LASTCALL.DAT"
#define ONE_FILE     "data/ONELINER.DAT"
#define MAIL_FILE    "data/MAIL.DAT"
#define VOTE_FILE    "data/VOTE.DAT"
#define BBS_FILE     "data/BBSLIST.DAT"
#define CONFIG_FILE  "data/CONFIG.DAT"
#define GFILES_FILE  "data/GFILES.DAT"
#define QSCAN_FILE   "data/QSCAN.DAT"
#define TRASH_FILE   "data/TRASHCAN"
#define LOG_FILE     "data/SYSOP.LOG"
#define NODE_FILE    "data/NODE.DAT"
#define NODEMSG_FILE "data/NODEMSG.DAT"
#define DOORS_FILE   "data/DOORS.DAT"
#define FILES_DIR    "data/files/"      /* actual stored file bytes for transfers */
#define DROP_SYS     "DOOR.SYS"         /* drop files written to the board's cwd */
#define DROP_INFO    "DORINFO1.DEF"
#define XFER_MAX     120000L            /* whole-file transfer buffer cap (RAM-bound) */
#define ART_WFC      "art/wfc.ans"
#define WO_HDR       "art/whoson.hdr"
#define WO_LINE      "art/whoson.lin"
#define WO_FTR       "art/whoson.ftr"
#define ART_MAILEDIT "art/msgedit.pp"     /* reuse the editor frame for mail */
#define UL2_HDR      "art/userlist.hdr"

static pp_config g_cfg;                   /* loaded once at startup */
static pp_u32 g_uindex;                   /* current caller's userbase index */
static int    g_node = 1;                 /* this instance's node number (argv) */
static pp_u32 g_msgcursor;                /* inbox poll cursor for the node bus */

/* file-transfer helpers (defined below cmd_filearea, used by it) */
static int  xfer_send_file(const char *path, pp_ctx *ctx);
static long xfer_recv_file(const char *name, pp_ctx *ctx);

#define CLEAR "\x1b[2J\x1b[H"

/* ---- formatting helpers ------------------------------------------------- */

static void u32_to_str(pp_u32 v, char *buf)
{
    char tmp[12];
    int n = 0, i = 0;
    if (v == 0) { buf[0] = '0'; buf[1] = '\0'; return; }
    while (v > 0) { tmp[n++] = (char)('0' + (int)(v % 10)); v /= 10; }
    while (n > 0) buf[i++] = tmp[--n];
    buf[i] = '\0';
}

static void fmt_hhmm(pp_u32 when, char *buf)
{
    time_t t = (time_t)when;
    struct tm *lt = localtime(&t);
    if (lt) sprintf(buf, "%02d:%02d", lt->tm_hour, lt->tm_min);
    else    strcpy(buf, "--:--");
}

/* ---- last-callers list: token resolver over the persisted ring ---------- */

static char g_when[8];   /* scratch for the |LT field of the current row */

static const char *lc_lookup(void *user, int a, int b)
{
    const pp_caller *r = (const pp_caller *)user;
    if (a == 'C' && b == 'T') return pps_get(PPS_LASTCALL_TITLE);
    if (r == (const pp_caller *)0) return (const char *)0;
    if (a == 'L' && b == 'H') return r->handle;
    if (a == 'L' && b == 'L') return r->location;
    if (a == 'L' && b == 'T') { fmt_hhmm(r->when, g_when); return g_when; }
    return (const char *)0;
}

/* ---- helpers ------------------------------------------------------------ */

static const char *slurp(const char *path, char *buf, int max)
{
    FILE *f = fopen(path, "rb");
    int n;
    if (f == (FILE *)0) return (const char *)0;
    n = (int)fread(buf, 1, (size_t)(max - 1), f);
    buf[n] = '\0';
    fclose(f);
    return buf;
}

static void show(const char *path, int is_tpl, pp_ctx *ctx)
{
    FILE *f = fopen(path, "rb");
    if (f == (FILE *)0) { render_strn(pps_get(PPS_MISSING_ART), ctx); return; }
    if (is_tpl) render_tpl(f, ctx);
    else        render_raw(f);
    fclose(f);
}

static void read_field(char *buf, int max)
{
    int n = 0, c;
    for (;;) {
        c = io_getch();
        if (c < 0 || c == '\r' || c == '\n') break;
        if (c == 8 || c == 127) { if (n > 0) { n--; io_puts("\b \b"); } continue; }
        if (c >= 32 && n < max - 1) { buf[n++] = (char)c; io_putc((pp_u8)c); }
    }
    buf[n] = '\0';
}

/* ---- new-user application (infoform) ------------------------------------ */

/* Simple non-cryptographic password hash (FNV-1a 32-bit -> 8 hex). Keeps the
 * password out of the file as plaintext; a real board would use something
 * stronger, but this matches the spirit and the C89/DOS budget. out >= 9. */
static void pw_hash(const char *pw, char *out)
{
    unsigned long h = 2166136261UL;
    while (*pw) { h ^= (unsigned char)*pw++; h *= 16777619UL; }
    sprintf(out, "%08lx", (unsigned long)(h & 0xffffffffUL));
}

/* line input that echoes '*' (for passwords) */
static void read_secret(char *buf, int max)
{
    int n = 0, c;
    for (;;) {
        c = io_getch();
        if (c < 0 || c == '\r' || c == '\n') break;
        if (c == 8 || c == 127) { if (n > 0) { n--; io_puts("\b \b"); } continue; }
        if (c >= 32 && n < max - 1) { buf[n++] = (char)c; io_putc((pp_u8)'*'); }
    }
    buf[n] = '\0';
}

/* labelled prompt -> read one field */
static void ask_field(pp_ctx *ctx, const char *label, char *buf, int max)
{
    render_strn("|07  ", ctx); io_puts(label); render_strn(" |08: |15", ctx);
    read_field(buf, max);
    io_puts("\x1b[0m\r\n");
}

/* labelled yes/no lightbar; returns 1 for yes */
static int ask_yn(pp_ctx *ctx, const char *label, int dflt_yes)
{
    static const char *yn[2] = { "yes", "no" };
    int r;
    render_strn("|07  ", ctx); io_puts(label); render_strn("  ", ctx);
    r = lightbar(ctx, yn, 2, dflt_yes ? 0 : 1);
    io_puts("\x1b[0m\r\n");
    return r == 0;
}

static const char *const NU_PROTO[3] = { "Zmodem", "Ymodem", "Xmodem" };

static void show_rules(pp_ctx *ctx)
{
    render_strn(
        "|CL|15\r\n  |BN |08-- |07new user application\r\n\r\n"
        "|08  ----------------------------------------------------------\r\n"
        "|07  a private system. by applying for access you agree to:\r\n\r\n"
        "|07   |08*|07 no flooding, no narcing, no lamer behaviour\r\n"
        "|07   |08*|07 share -- don't leech. ratios are watched.\r\n"
        "|07   |08*|07 the sysop's word is final\r\n\r\n"
        "|08  ----------------------------------------------------------\r\n\r\n"
        "|07  do you accept?  ", ctx);
}

static void show_app_summary(pp_ctx *ctx, const pp_user *u, const char *ref)
{
    render_strn("|CL|15  application summary |08-- |07review before you send"
                "\x1b[0m\r\n\r\n", ctx);
    render_strn("|07  alias     |08: |15", ctx); io_puts(u->handle);
    render_strn("\r\n|07  real name |08: |15", ctx); io_puts(u->real_name[0] ? u->real_name : "(blank)");
    render_strn("\r\n|07  email     |08: |15", ctx); io_puts(u->email[0] ? u->email : "(blank)");
    render_strn("\r\n|07  born      |08: |15", ctx); io_puts(u->birthdate[0] ? u->birthdate : "(blank)");
    render_strn("\r\n|07  location  |08: |15", ctx); io_puts(u->location);
    render_strn("\r\n|07  city/state|08: |15", ctx); io_puts(u->city[0] ? u->city : "(blank)");
    render_strn("\r\n|07  zip       |08: |15", ctx); io_puts(u->zip[0] ? u->zip : "(blank)");
    render_strn("\r\n|07  computer  |08: |15", ctx); io_puts(u->computer[0] ? u->computer : "(blank)");
    render_strn("\r\n|07  protocol  |08: |15", ctx); io_puts(NU_PROTO[u->prot < 3 ? u->prot : 0]);
    render_strn("\r\n|07  ansi/pause|08: |15", ctx);
    io_puts((u->flags & UF_ANSI) ? "ansi, " : "no-ansi, ");
    io_puts((u->flags & UF_PAUSE) ? "pause" : "no-pause");
    render_strn("\r\n|07  note      |08: |15", ctx); io_puts(u->tagline[0] ? u->tagline : "(none)");
    render_strn("\r\n|07  heard via |08: |15", ctx); io_puts(ref[0] ? ref : "(blank)");
    render_strn("\r\n\r\n|07  send this application?  ", ctx);
}

/* The new-user application: a multi-screen questionnaire. Returns 1 if the
 * caller completed and submitted it, 0 if they declined the rules or cancelled
 * at review (the session should then drop). The profile lands in the user
 * record (password hashed); the referral is logged for the sysop to vet. */
static int new_user_app(pp_user *u, pp_ctx *ctx)
{
    static const char *agree[2]  = { "i agree", "no thanks" };
    static const char *sendit[2] = { "submit application", "cancel" };
    char referral[60];
    int p;

    ctx->handle = u->handle;               /* so |UH shows in the form */

    /* 1. rules + agreement */
    show_rules(ctx);
    if (lightbar(ctx, agree, 2, 0) != 0) return 0;     /* declined / carrier */

    /* 2. identity + password */
    io_puts("\x1b[0m\x1b[2J\x1b[H");
    render_strn("|15  new user application |08-- |07who are you?\x1b[0m\r\n\r\n", ctx);
    render_strn("|07  alias        |08: |15", ctx); io_puts(u->handle);
    io_puts("\x1b[0m\r\n");
    ask_field(ctx, "real name   ", u->real_name, PP_RNAME_MAX);
    for (;;) {                              /* password + verify, must match */
        char p1[24], p2[24];
        render_strn("|07  password     |08: |15", ctx); read_secret(p1, (int)sizeof p1); io_puts("\r\n");
        render_strn("|07  verify pass  |08: |15", ctx); read_secret(p2, (int)sizeof p2); io_puts("\r\n");
        if (p1[0] == '\0') { render_strn("|12  password can't be blank.\x1b[0m\r\n", ctx); continue; }
        if (strcmp(p1, p2) != 0) { render_strn("|12  did not match, try again.\x1b[0m\r\n", ctx); continue; }
        pw_hash(p1, u->pwhash);
        break;
    }
    ask_field(ctx, "email       ", u->email, PP_EMAIL_MAX);
    ask_field(ctx, "birth date  ", u->birthdate, PP_BDATE_MAX);

    /* 3. location + system */
    io_puts("\x1b[0m\x1b[2J\x1b[H");
    render_strn("|15  new user application |08-- |07where & what?\x1b[0m\r\n\r\n", ctx);
    ask_field(ctx, "location    ", u->location, PP_LOC_MAX);
    ask_field(ctx, "city / state", u->city, PP_CITY_MAX);
    ask_field(ctx, "zip code    ", u->zip, PP_ZIP_MAX);
    ask_field(ctx, "computer    ", u->computer, PP_COMP_MAX);

    /* 4. preferences + notes */
    io_puts("\x1b[0m\x1b[2J\x1b[H");
    render_strn("|15  new user application |08-- |07preferences\x1b[0m\r\n\r\n", ctx);
    if (ask_yn(ctx, "ansi graphics?       ", 1)) u->flags |= UF_ANSI;
    if (ask_yn(ctx, "pause between screens?", 1)) u->flags |= UF_PAUSE;
    render_strn("|07  default protocol     ", ctx);
    p = lightbar(ctx, NU_PROTO, 3, 0); io_puts("\x1b[0m\r\n");
    u->prot = (pp_u8)(p < 0 ? 0 : p);
    ask_field(ctx, "user note   ", u->tagline, PP_TAG_MAX);
    referral[0] = '\0';
    ask_field(ctx, "how'd you hear about us?", referral, (int)sizeof referral);

    if (u->location[0] == '\0') strcpy(u->location, "parts unknown");
    if (u->tagline[0]  == '\0') strcpy(u->tagline, "(quiet type)");
    strcpy(u->group, "none");

    /* 5. review + submit */
    io_puts("\x1b[0m\x1b[2J\x1b[H");
    show_app_summary(ctx, u, referral);
    if (lightbar(ctx, sendit, 2, 0) != 0) return 0;    /* cancelled / carrier */

    {   char ev[180];
        sprintf(ev, "APPLICATION: %.20s (%.22s) | %.20s | comp %.16s | heard: %.36s",
                u->handle, u->real_name, u->location, u->computer, referral);
        sy_log(LOG_FILE, ev);
    }
    return 1;
}

/* ---- user list ---------------------------------------------------------- */

static char g_ucalls[12];

static const char *ul_lookup(void *user, int a, int b)
{
    const pp_user *u = (const pp_user *)user;
    if (a == 'C' && b == 'T') return pps_get(PPS_USERLIST_TITLE);
    if (u == (const pp_user *)0) return (const char *)0;
    if (a == 'W' && b == 'H') return u->handle;
    if (a == 'W' && b == 'L') return u->location;
    if (a == 'W' && b == 'G') return u->group;
    if (a == 'W' && b == 'T') return u->tagline;
    if (a == 'W' && b == 'C') { u32_to_str(u->times_called, g_ucalls); return g_ucalls; }
    return (const char *)0;
}

static void user_list(pp_ctx *ctx)
{
    /* One record at a time off disk -- holding all users in near data cost 10k
     * of the 64k DGROUP; paging them keeps the 16-bit build comfortable and
     * also lifts the old 64-user display cap. */
    pp_user one;        /* auto, not static: keep DGROUP < 64k on the 16-bit build */
    char hbuf[256], lbuf[256], fbuf[256];
    const char *hdr, *line, *ftr;
    pp_u32 n = ub_count(), i;

    hdr  = slurp(UL_HDR,  hbuf, (int)sizeof hbuf);
    line = slurp(UL_LINE, lbuf, (int)sizeof lbuf);
    ftr  = slurp(UL_FTR,  fbuf, (int)sizeof fbuf);

    io_puts(CLEAR);
    ctx->lookup = ul_lookup;
    ctx->user = (void *)0;
    if (hdr) render_strn(hdr, ctx);
    for (i = 0; i < n; i++) {
        if (ub_get(i, &one) != 0) continue;
        ctx->user = &one;
        if (line) render_strn(line, ctx);
    }
    ctx->user = (void *)0;
    if (ftr) render_strn(ftr, ctx);
    ctx->lookup = (pp_lookup_fn)0;
}

static void last_callers(pp_ctx *ctx)
{
    char hbuf[512], lbuf[256], fbuf[256];
    const char *hdr, *line, *ftr;
    void *rows[LC_MAX];
    pp_caller callers[LC_MAX];
    int n = lc_count(), i;

    if (n <= 0) return;
    for (i = 0; i < n; i++) { lc_get(i, &callers[i]); rows[i] = &callers[i]; }

    hdr  = slurp(LC_HDR,  hbuf, (int)sizeof hbuf);
    line = slurp(LC_LINE, lbuf, (int)sizeof lbuf);
    ftr  = slurp(LC_FTR,  fbuf, (int)sizeof fbuf);
    if (!line) { render_strn(pps_get(PPS_MISSING_ART), ctx); return; }

    ctx->lookup = lc_lookup;
    render_list(ctx, hdr, line, ftr, rows, n);
    ctx->lookup = (pp_lookup_fn)0;
}

/* ---- oneliner wall ------------------------------------------------------ */

static const char *ol_lookup(void *user, int a, int b)
{
    const pp_oneliner *r = (const pp_oneliner *)user;
    if (a == 'C' && b == 'T') return pps_get(PPS_ONELINER_TITLE);
    if (r == (const pp_oneliner *)0) return (const char *)0;
    if (a == 'O' && b == 'A') return r->author;
    if (a == 'O' && b == 'T') return r->text;
    return (const char *)0;
}

static void show_oneliners(pp_ctx *ctx)
{
    char hbuf[256], lbuf[256], fbuf[256];
    const char *hdr, *line, *ftr;
    pp_oneliner ols[OL_MAX];
    int n = ol_count(), i;

    hdr  = slurp(OL_HDR,  hbuf, (int)sizeof hbuf);
    line = slurp(OL_LINE, lbuf, (int)sizeof lbuf);
    ftr  = slurp(OL_FTR,  fbuf, (int)sizeof fbuf);

    ctx->lookup = ol_lookup;
    ctx->user = (void *)0;
    if (hdr) render_strn(hdr, ctx);
    if (n <= 0) {
        render_strn("|08\r\n  the wall is bare. drop the first line.\r\n", ctx);
    } else {
        for (i = 0; i < n; i++) {
            ol_get(i, &ols[i]);
            ctx->user = &ols[i];
            if (line) render_strn(line, ctx);
        }
    }
    ctx->user = (void *)0;
    if (ftr) render_strn(ftr, ctx);
    ctx->lookup = (pp_lookup_fn)0;
}

/* ---- menu command handler ----------------------------------------------- */

static void pause_key(pp_ctx *ctx)
{
    render_strn(pps_get(PPS_PRESS_KEY), ctx);
    io_getch();
}

static void stub(const char *what, pp_ctx *ctx)
{
    io_puts(CLEAR);
    io_puts("\x1b[0m\r\n  \x1b[1;37m");
    io_puts(what);
    io_puts("\x1b[0;37m isn't wired up yet \x1b[1;30m-\x1b[0;37m the sysop is on it.\r\n");
    pause_key(ctx);
}

/* ---- message bases ------------------------------------------------------ */

static void fmt_datetime(pp_u32 when, char *buf)
{
    time_t t = (time_t)when;
    struct tm *lt = localtime(&t);
    if (lt) sprintf(buf, "%02d/%02d %02d:%02d", lt->tm_mon + 1, lt->tm_mday, lt->tm_hour, lt->tm_min);
    else    strcpy(buf, "--/-- --:--");
}

static void show_message(const pp_msg *m, int idx, int total, pp_ctx *ctx)
{
    char num[12], dt[24];
    io_puts(CLEAR);
    render_strn("|07\r\n  |08msg |15", ctx);
    u32_to_str((pp_u32)(idx + 1), num); io_puts(num);
    render_strn("|08 of |15", ctx);
    u32_to_str((pp_u32)total, num); io_puts(num); io_puts("\r\n");
    render_strn("|07  from |08: |15", ctx); io_puts(m->from);
    render_strn("|08   to |08: |15", ctx); io_puts(m->to); io_puts("\r\n");
    render_strn("|07  subj |08: |15", ctx); io_puts(m->subject);
    fmt_datetime(m->when, dt);
    render_strn("|08   \372 |07", ctx); io_puts(dt); io_puts("\r\n");
    render_strn("|08  \304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\304\r\n|07\r\n", ctx);
    io_puts(m->body);
    render_strn("|07\r\n", ctx);
}

static void post_message(const char *tag, const pp_user *u, pp_ctx *ctx)
{
    pp_msg m;        /* auto, not static: keeps DGROUP < 64k on the 16-bit build */

    memset(&m, 0, sizeof m);
    strncpy(m.from, u->handle, MB_FROM - 1);
    render_strn(pps_get(PPS_MSG_TO), ctx);
    read_field(m.to, MB_TO);   if (m.to[0] == '\0') strcpy(m.to, "all");
    render_strn(pps_get(PPS_MSG_SUBJ), ctx);
    read_field(m.subject, MB_SUBJ);
    if (m.subject[0] == '\0') strcpy(m.subject, "(no subject)");

    if (editor_run(ctx, ART_MSGEDIT, m.from, m.to, m.subject, m.body, MB_BODY)) {
        io_puts("\x1b[0m\r\n");
        if (tc_blocked(m.subject) || tc_blocked(m.body)) {
            render_strn("|12  your message tripped the filter -- not posted.|07\r\n", ctx);
        } else {
            m.when = (pp_u32)time((time_t *)0);
            if (mb_add(&m) == 0) {
                char ev[80]; sprintf(ev, "%s posted in %s", m.from, tag); sy_log(LOG_FILE, ev);
                render_strn("|10  posted.|07\r\n", ctx);
            } else {
                render_strn("|12  post failed.|07\r\n", ctx);
            }
        }
    } else {
        io_puts("\x1b[0m\r\n");
        render_strn("|12  message discarded.|07\r\n", ctx);
    }
}

static menu_result cmd_msgarea(const char *arg, pp_ctx *ctx, const pp_user *u)
{
    static pp_msg msg;
    char tag[16], name[40], num[12];
    int total, cur, c;
    pp_u32 lastread, fresh;

    name[0] = '\0';
    if (sscanf(arg, "%15s %39[^\r\n]", tag, name) < 1) return MENU_OK;
    if (name[0] == '\0') strcpy(name, tag);
    if (mb_open(tag) != 0) { stub("that message area", ctx); return MENU_OK; }

    for (;;) {
        total = mb_count();
        lastread = qs_get((int)g_uindex, tag);
        if (lastread > (pp_u32)total) lastread = (pp_u32)total;   /* base shrank */
        fresh = (pp_u32)total - lastread;
        io_puts(CLEAR);
        render_strn("|07\r\n  |15", ctx); io_puts(name);
        render_strn("|08 \372 |07", ctx);
        u32_to_str((pp_u32)total, num); io_puts(num); io_puts(" messages");
        if (fresh > 0) {
            render_strn(" |10\372 |15", ctx); u32_to_str(fresh, num); io_puts(num);
            render_strn("|10 new|07", ctx);
        }
        io_puts("\r\n");
        render_strn("|08  [|15r|08]ead  [|15n|08]ew  [|15p|08]ost  [|15q|08]uit |08\372 |07", ctx);

        c = io_getch();
        if (c < 0) { mb_close(); return MENU_LOGOFF; }
        if (c == 'q' || c == 'Q' || c == '\r' || c == '\n') break;

        if (c == 'r' || c == 'R' || c == 'n' || c == 'N') {
            int start = (c == 'n' || c == 'N') ? (int)lastread : 0;
            if (total <= 0) {
                render_strn("|08\r\n  no messages yet. be the first.\r\n", ctx);
                pause_key(ctx);
            } else if (start >= total) {
                render_strn("|08\r\n  no new messages -- you're caught up.\r\n", ctx);
                pause_key(ctx);
            } else {
                for (cur = start; cur < total; cur++) {
                    if (mb_get(cur, &msg) == 0) { show_message(&msg, cur, total, ctx); pause_key(ctx); }
                }
                qs_set((int)g_uindex, tag, (pp_u32)total);   /* mark caught up */
            }
        } else if (c == 'p' || c == 'P') {
            post_message(tag, u, ctx);
            pause_key(ctx);
        }
    }
    mb_close();
    return MENU_OK;
}

/* ---- shared little helpers --------------------------------------------- */

static void put_pad(const char *s, int w)
{
    int i = 0;
    while (s[i] && i < w) { io_putc((pp_u8)s[i]); i++; }
    for (; i < w; i++) io_putc((pp_u8)' ');
}

/* ---- private email ------------------------------------------------------ */

static void mail_send(const pp_user *u, pp_ctx *ctx)
{
    static pp_mail m;
    char to[EM_TO];
    memset(&m, 0, sizeof m);
    render_strn("|07\r\n  to |08(handle)|08: |15", ctx);
    read_field(to, EM_TO);
    if (to[0] == '\0') return;
    if (!ub_find(to, (pp_user *)0, (pp_u32 *)0)) {
        render_strn("|12\r\n  no such user.|07\r\n", ctx); pause_key(ctx); return;
    }
    strncpy(m.to, to, EM_TO - 1);
    strncpy(m.from, u->handle, EM_FROM - 1);
    render_strn("|07  subj |08: |15", ctx);
    read_field(m.subject, EM_SUBJ);
    if (m.subject[0] == '\0') strcpy(m.subject, "(no subject)");
    if (editor_run(ctx, ART_MAILEDIT, m.from, m.to, m.subject, m.body, EM_BODY)) {
        m.when = (pp_u32)time((time_t *)0);
        io_puts("\x1b[0m\r\n");
        render_strn(em_send(&m) == 0 ? "|10  mail sent.|07\r\n" : "|12  send failed.|07\r\n", ctx);
    } else {
        io_puts("\x1b[0m\r\n  message discarded.\r\n");
    }
    pause_key(ctx);
}

static void mail_read(const pp_user *u, pp_ctx *ctx)
{
    static pp_mail m;
    char num[12], dt[24];
    int n = em_count_to(u->handle), i, idx;
    if (n <= 0) { render_strn("|08\r\n  no mail.\r\n", ctx); pause_key(ctx); return; }
    for (i = 0; i < n; i++) {
        if (em_get_to(u->handle, i, &m, &idx) != 0) continue;
        io_puts(CLEAR);
        render_strn("|07\r\n  |08mail |15", ctx); u32_to_str((pp_u32)(i + 1), num); io_puts(num);
        render_strn("|08 of |15", ctx); u32_to_str((pp_u32)n, num); io_puts(num); io_puts("\r\n");
        render_strn("|07  from |08: |15", ctx); io_puts(m.from);
        fmt_datetime(m.when, dt);
        render_strn("|08  \372 |07", ctx); io_puts(dt); io_puts("\r\n");
        render_strn("|07  subj |08: |15", ctx); io_puts(m.subject); io_puts("\r\n");
        { int k; render_strn("|08  ", ctx); for (k = 0; k < 60; k++) io_putc((pp_u8)'\xc4'); }
        render_strn("|07\r\n\r\n", ctx);
        io_puts(m.body);
        render_strn("|07\r\n", ctx);
        if (!(m.flags & EM_FLAG_READ)) { m.flags |= EM_FLAG_READ; em_update(idx, &m); }
        pause_key(ctx);
    }
}

static void cmd_email(const pp_user *u, pp_ctx *ctx)
{
    char num[12];
    for (;;) {
        int c, n = em_count_to(u->handle);
        io_puts(CLEAR);
        render_strn("|07\r\n  |15your mailbox|08 \372 |07", ctx);
        u32_to_str((pp_u32)n, num); io_puts(num); io_puts(" message(s)\r\n");
        render_strn("|08  [|15r|08]ead  [|15s|08]end  [|15q|08]uit |08\372 |07", ctx);
        c = io_getch();
        if (c < 0 || c == 'q' || c == 'Q' || c == '\r' || c == '\n') break;
        if (c == 'r' || c == 'R') mail_read(u, ctx);
        else if (c == 's' || c == 'S') mail_send(u, ctx);
    }
}

/* ---- voting booth ------------------------------------------------------- */

static void poll_create(pp_ctx *ctx)
{
    static pp_poll p;
    char buf[VT_OPT_MAX];
    int i;
    memset(&p, 0, sizeof p);
    io_puts(CLEAR);
    render_strn("|07\r\n  |15create a poll|07\r\n  question|08: |15", ctx);
    read_field(p.question, VT_Q_MAX);
    if (p.question[0] == '\0') return;
    for (i = 0; i < VT_OPTS_MAX; i++) {
        render_strn("|07  option |08(enter ends)|08: |15", ctx);
        read_field(buf, VT_OPT_MAX);
        if (buf[0] == '\0') break;
        strncpy(p.options[i], buf, VT_OPT_MAX - 1);
    }
    if (i < 2) { render_strn("|12  need at least 2 options.|07\r\n", ctx); pause_key(ctx); return; }
    p.noptions = (pp_u8)i;
    p.when = (pp_u32)time((time_t *)0);
    vt_add(&p);
    render_strn("|10  poll created.|07\r\n", ctx); pause_key(ctx);
}

static void poll_vote(int idx, pp_ctx *ctx)
{
    static pp_poll p;
    char num[12];
    int j;
    if (vt_get(idx, &p) != 0) return;
    io_puts(CLEAR);
    render_strn("|07\r\n  |15", ctx); io_puts(p.question); io_puts("\r\n\r\n");
    for (j = 0; j < p.noptions; j++) {
        render_strn("|08   [|15", ctx); u32_to_str((pp_u32)(j + 1), num); io_puts(num);
        render_strn("|08] |07", ctx); io_puts(p.options[j]);
        render_strn("  |08\372 ", ctx); u32_to_str(p.counts[j], num); io_puts(num);
        render_strn(" votes|07\r\n", ctx);
    }
    if (vt_has_voted(idx, (int)g_uindex)) {
        render_strn("|08\r\n  you've already voted here.\r\n", ctx);
    } else {
        render_strn("|07\r\n  your vote |08(1-N, enter skips)|08: |07", ctx);
        read_field(num, (int)sizeof num);
        if (num[0] >= '1' && num[0] <= ('0' + p.noptions)) {
            vt_cast(idx, num[0] - '1', (int)g_uindex);
            render_strn("|10  vote recorded.|07\r\n", ctx);
        }
    }
    pause_key(ctx);
}

static void cmd_voting(const pp_user *u, pp_ctx *ctx)
{
    static pp_poll p;
    char num[12];
    for (;;) {
        int n = vt_count(), i, c;
        io_puts(CLEAR);
        render_strn("|07\r\n  |15the voting booth|07\r\n\r\n", ctx);
        if (n <= 0) render_strn("|08  no polls yet.\r\n", ctx);
        for (i = 0; i < n; i++) {
            vt_get(i, &p);
            render_strn("|08  [|15", ctx); u32_to_str((pp_u32)(i + 1), num); io_puts(num);
            render_strn("|08] |07", ctx); io_puts(p.question); io_puts("\r\n");
        }
        render_strn("|08\r\n  [|15#|08] vote", ctx);
        if (u->sl >= 255) render_strn("  [|15c|08] create", ctx);
        render_strn("  [|15q|08] quit |08\372 |07", ctx);
        c = io_getch();
        if (c < 0 || c == 'q' || c == 'Q' || c == '\r' || c == '\n') break;
        if ((c == 'c' || c == 'C') && u->sl >= 255) poll_create(ctx);
        else if (c >= '1' && c <= ('0' + n)) poll_vote(c - '1', ctx);
    }
}

/* ---- file areas (catalog only; transfer protocols are a later phase) ---- */

static void file_add(const pp_user *u, pp_ctx *ctx)
{
    static pp_file f;
    char buf[16];
    memset(&f, 0, sizeof f);
    render_strn("|07\r\n  filename |08: |15", ctx);
    read_field(f.name, FB_NAME);
    if (f.name[0] == '\0') return;
    render_strn("|07  describe |08: |15", ctx);
    read_field(f.desc, FB_DESC);
    render_strn("|07  size (k) |08: |15", ctx);
    read_field(buf, (int)sizeof buf);
    { int kb = 0, j = 0; while (buf[j] >= '0' && buf[j] <= '9') { kb = kb * 10 + (buf[j] - '0'); j++; } f.size = (pp_u32)kb * 1024u; }
    strncpy(f.uploader, u->handle, FB_UPLOADER - 1);
    f.when = (pp_u32)time((time_t *)0);
    fb_add(&f);
    render_strn("|10  catalogued.|07\r\n", ctx); pause_key(ctx);
}

static void cmd_filearea(const char *arg, const pp_user *u, pp_ctx *ctx)
{
    static pp_file f;
    char tag[16], name[40], num[12];
    int total, i, c;
    name[0] = '\0';
    if (sscanf(arg, "%15s %39[^\r\n]", tag, name) < 1) return;
    if (name[0] == '\0') strcpy(name, tag);
    if (fb_open(tag) != 0) { stub("that file area", ctx); return; }
    for (;;) {
        total = fb_count();
        io_puts(CLEAR);
        render_strn("|07\r\n  |15", ctx); io_puts(name);
        render_strn("|08 \372 |07", ctx); u32_to_str((pp_u32)total, num); io_puts(num); io_puts(" files\r\n\r\n");
        for (i = 0; i < total; i++) {
            fb_get(i, &f);
            render_strn("|11  ", ctx); put_pad(f.name, 14);
            render_strn("|08", ctx); u32_to_str(f.size / 1024u, num); put_pad(num, 6); render_strn("k |15", ctx);
            put_pad(num, 0);
            render_strn("|08dl:", ctx); u32_to_str(f.downloads, num); put_pad(num, 4);
            render_strn(" |07", ctx); io_puts(f.desc); io_puts("\r\n");
        }
        render_strn("|08\r\n  [|15d|08]ownload  [|15u|08]pload", ctx);
        if (u->sl >= 255) render_strn("  [|15a|08]dd", ctx);
        render_strn("  [|15q|08]uit |08\372 |07", ctx);
        c = io_getch();
        if (c < 0 || c == 'q' || c == 'Q' || c == '\r' || c == '\n') break;
        if ((c == 'a' || c == 'A') && u->sl >= 255) { file_add(u, ctx); }
        else if (c == 'd' || c == 'D') {
            render_strn("|07\r\n  file # |08: |07", ctx);
            read_field(num, (int)sizeof num);
            { int k = 0, j = 0; char p[96];
              while (num[j] >= '0' && num[j] <= '9') { k = k * 10 + (num[j] - '0'); j++; }
              if (k >= 1 && k <= total) {
                  fb_get(k - 1, &f);
                  sprintf(p, "%s%s", FILES_DIR, f.name);
                  if (xfer_send_file(p, ctx) == 0) fb_inc_downloads(k - 1);
                  pause_key(ctx);
              } }
        }
        else if ((c == 'u' || c == 'U') && u->dsl >= 10) {
            char fn[FB_NAME];
            render_strn("|07\r\n  upload filename |08: |15", ctx);
            read_field(fn, FB_NAME);
            if (fn[0] != '\0') {
                long got = xfer_recv_file(fn, ctx);
                if (got > 0) {
                    memset(&f, 0, sizeof f);
                    strncpy(f.name, fn, FB_NAME - 1);
                    strcpy(f.desc, "(uploaded)");
                    f.size = (pp_u32)got;
                    strncpy(f.uploader, u->handle, FB_UPLOADER - 1);
                    f.when = (pp_u32)time((time_t *)0);
                    fb_add(&f);
                }
                pause_key(ctx);
            }
        }
    }
    fb_close();
}

/* ---- bbs list ----------------------------------------------------------- */

static void bbs_add(const pp_user *u, pp_ctx *ctx)
{
    static pp_bbs b;
    memset(&b, 0, sizeof b);
    render_strn("|07\r\n  board name |08: |15", ctx);
    read_field(b.name, PP_BBS_NAME_MAX);
    if (b.name[0] == '\0') return;
    render_strn("|07  address    |08(host:port)|08: |15", ctx);
    read_field(b.address, PP_BBS_ADDR_MAX);
    render_strn("|07  sysop      |08: |15", ctx);
    read_field(b.sysop, PP_BBS_SYSOP_MAX);
    strncpy(b.added_by, u->handle, PP_BBS_BY_MAX - 1);
    b.when = (pp_u32)time((time_t *)0);
    bl_add(&b);
    render_strn("|10  added to the list.|07\r\n", ctx); pause_key(ctx);
}

static void cmd_bbslist(const pp_user *u, pp_ctx *ctx)
{
    static pp_bbs b;
    pp_u32 n, i;
    for (;;) {
        int c;
        n = bl_count();
        io_puts(CLEAR);
        render_strn("|07\r\n  |15the bbs list|07\r\n\r\n", ctx);
        if (n == 0) render_strn("|08  empty -- be the first to add a board.\r\n", ctx);
        for (i = 0; i < n; i++) {
            bl_get(i, &b);
            render_strn("|11  ", ctx); put_pad(b.name, 22);
            render_strn("|08\372 |07", ctx); put_pad(b.address, 24);
            render_strn("|08", ctx); io_puts(b.sysop); io_puts("\r\n");
        }
        render_strn("|08\r\n  [|15a|08]dd  [|15q|08]uit |08\372 |07", ctx);
        c = io_getch();
        if (c < 0 || c == 'q' || c == 'Q' || c == '\r' || c == '\n') break;
        if (c == 'a' || c == 'A') bbs_add(u, ctx);
    }
}

/* ---- bulletins / g-files ------------------------------------------------ */

static int ends_pp(const char *s)
{
    size_t n = strlen(s);
    return n >= 3 && s[n - 3] == '.' && (s[n - 2] == 'p' || s[n - 2] == 'P')
                                     && (s[n - 1] == 'p' || s[n - 1] == 'P');
}

static void gfile_add(const pp_user *u, pp_ctx *ctx)
{
    static pp_gfile g;
    (void)u;
    memset(&g, 0, sizeof g);
    render_strn("|07\r\n  title |08: |15", ctx);  read_field(g.title, PP_GF_TITLE_MAX);
    if (g.title[0] == '\0') return;
    render_strn("|07  file  |08(art path)|08: |15", ctx); read_field(g.file, PP_GF_FILE_MAX);
    render_strn("|07  acs   |08(- for all)|08: |15", ctx); read_field(g.acs, PP_GF_ACS_MAX);
    if (g.acs[0] == '\0') strcpy(g.acs, "-");
    gf_add(&g);
    render_strn("|10  bulletin added.|07\r\n", ctx); pause_key(ctx);
}

static void cmd_gfiles(const pp_user *u, pp_ctx *ctx)
{
    static pp_gfile g;
    int shown[64];
    for (;;) {
        pp_u32 n = gf_count(), i;
        char num[12];
        int ns = 0, c;
        io_puts(CLEAR);
        render_strn("|07\r\n  |15bulletins \372 g-files|07\r\n\r\n", ctx);
        for (i = 0; i < n && ns < 64; i++) {
            gf_get((int)i, &g);
            if (!acs_eval(g.acs, u)) continue;
            shown[ns++] = (int)i;
            render_strn("|08  [|15", ctx); u32_to_str((pp_u32)ns, num); io_puts(num);
            render_strn("|08] |07", ctx); io_puts(g.title); io_puts("\r\n");
        }
        if (ns == 0) render_strn("|08  nothing posted yet.\r\n", ctx);
        render_strn("|08\r\n  [|15#|08] read", ctx);
        if (u->sl >= 255) render_strn("  [|15a|08]dd", ctx);
        render_strn("  [|15q|08]uit |08\372 |07", ctx);
        c = io_getch();
        if (c < 0 || c == 'q' || c == 'Q' || c == '\r' || c == '\n') break;
        if ((c == 'a' || c == 'A') && u->sl >= 255) { gfile_add(u, ctx); }
        else if (c >= '1' && c <= ('0' + ns)) {
            gf_get(shown[c - '1'], &g);
            io_puts(CLEAR);
            show(g.file, ends_pp(g.file), ctx);
            pause_key(ctx);
        }
    }
}

/* ---- multinode: presence, who's-online, paging ------------------------- */

/* Update this node's live action line in the presence table. */
static void nd_act(const char *what)
{
    nd_action(g_node, what, (pp_u32)time((time_t *)0));
}

/* Drain the node's inbox and surface any pages / chat lines that arrived.
 * Cheap to call -- safe between prompts; returns the number shown. */
static int poll_inbox(pp_ctx *ctx)
{
    pp_nodemsg m;
    int shown = 0;
    while (pg_poll(g_node, &g_msgcursor, &m)) {
        io_puts("\x1b[0m\r\n");
        switch (m.kind) {
        case PG_PAGE:
            render_strn("|14  \372\372 |15", ctx); io_puts(m.from_handle);
            render_strn("|14 is paging you", ctx);
            if (m.text[0]) { render_strn(" |08(|07", ctx); io_puts(m.text); render_strn("|08)", ctx); }
            render_strn(" \372\372|07\r\n", ctx);
            break;
        case PG_ACK:
            render_strn("|10  \372 |15", ctx); io_puts(m.from_handle);
            render_strn("|10 answers|08: |07", ctx); io_puts(m.text); io_puts("\r\n");
            break;
        case PG_END:
            render_strn("|08  \372 ", ctx); io_puts(m.from_handle);
            render_strn(" ended the chat.\r\n", ctx);
            break;
        default: /* PG_CHAT */
            render_strn("|11  ", ctx); io_puts(m.from_handle);
            render_strn("|08: |07", ctx); io_puts(m.text); io_puts("\r\n");
            break;
        }
        shown++;
    }
    return shown;
}

/* who's-online list -- token resolver over a pp_node row. */
static char g_nodenum[6];
static char g_nodestat[10];

static const char *wo_lookup(void *user, int a, int b)
{
    const pp_node *r = (const pp_node *)user;
    if (r == (const pp_node *)0) return (const char *)0;
    if (a == 'N' && b == 'N') { u32_to_str((pp_u32)r->node_no, g_nodenum); return g_nodenum; }
    if (a == 'N' && b == 'H') return r->handle;
    if (a == 'N' && b == 'L') return r->location;
    if (a == 'N' && b == 'A') return r->action;
    if (a == 'N' && b == 'S') {
        strcpy(g_nodestat, r->status == ND_BETWEEN ? "login" : "online");
        return g_nodestat;
    }
    return (const char *)0;
}

static void cmd_whoson(pp_ctx *ctx)
{
    pp_node nodes[PP_MAX_NODES];          /* auto, not static: keep DGROUP < 64k */
    char hbuf[512], lbuf[256], fbuf[256];
    const char *hdr, *line, *ftr;
    void *rows[PP_MAX_NODES];
    int i, n = 0, max = nd_max();

    for (i = 1; i <= max; i++) {
        pp_node nd;
        if (nd_get(i, &nd) != 0) continue;
        if (nd.status == ND_IDLE) continue;
        nodes[n] = nd; rows[n] = &nodes[n]; n++;
    }
    io_puts(CLEAR);
    hdr  = slurp(WO_HDR,  hbuf, (int)sizeof hbuf);
    line = slurp(WO_LINE, lbuf, (int)sizeof lbuf);
    ftr  = slurp(WO_FTR,  fbuf, (int)sizeof fbuf);
    if (!line) { render_strn(pps_get(PPS_MISSING_ART), ctx); pause_key(ctx); return; }
    if (n == 0) {
        if (hdr) render_strn(hdr, ctx);
        render_strn("|08\r\n  no one else is online right now.\r\n", ctx);
        if (ftr) render_strn(ftr, ctx);
    } else {
        ctx->lookup = wo_lookup;
        render_list(ctx, hdr, line, ftr, rows, n);
        ctx->lookup = (pp_lookup_fn)0;
    }
    pause_key(ctx);
}

/* Page another node (or the sysop console) -- send a chat request. */
static void cmd_page(const pp_user *u, pp_ctx *ctx)
{
    pp_node nodes[PP_MAX_NODES];          /* auto, not static: keep DGROUP < 64k */
    char num[12], text[PP_MSG_TEXT_MAX];
    int i, n = 0, max = nd_max(), target;

    io_puts(CLEAR);
    render_strn("|07\r\n  |15page a node|07\r\n\r\n", ctx);
    render_strn("|08  [|150|08] the sysop console\r\n", ctx);
    for (i = 1; i <= max; i++) {
        pp_node nd;
        if (nd_get(i, &nd) != 0) continue;
        if (nd.status == ND_IDLE || i == g_node) continue;
        nodes[n++] = nd;
        render_strn("|08  [|15", ctx); u32_to_str((pp_u32)nd.node_no, num); io_puts(num);
        render_strn("|08] |07", ctx); io_puts(nd.handle);
        render_strn(" |08\372 ", ctx); io_puts(nd.action); io_puts("\r\n");
    }
    render_strn("|08\r\n  page which node |08(enter cancels)|08: |07", ctx);
    read_field(num, (int)sizeof num);
    if (num[0] == '\0') return;
    target = 0;
    for (i = 0; num[i] >= '0' && num[i] <= '9'; i++) target = target * 10 + (num[i] - '0');
    if (target != PG_SYSOP) {
        int ok = 0;
        for (i = 0; i < n; i++) if ((int)nodes[i].node_no == target) ok = 1;
        if (!ok) { render_strn("|12  no one on that node.|07\r\n", ctx); pause_key(ctx); return; }
    }
    render_strn("|07  one-line message |08(optional)|08: |15", ctx);
    read_field(text, (int)sizeof text);
    if (tc_blocked(text)) { render_strn("|12  that tripped the filter.|07\r\n", ctx); pause_key(ctx); return; }
    if (pg_send(target, g_node, u->handle, PG_PAGE, text, (pp_u32)time((time_t *)0)) != 0)
        render_strn("|10  page sent.|07\r\n", ctx);
    else
        render_strn("|12  page failed.|07\r\n", ctx);
    pause_key(ctx);
}

/* ---- file transfers (XMODEM over the io byte stream) ------------------- */

static int  xm_io_get(void *io, int tmo) { (void)io; (void)tmo; return io_getch(); }
static void xm_io_put(void *io, pp_u8 b) { (void)io; io_putc(b); }

/* Send a stored file over XMODEM-1K. 0 on success. Whole file is buffered
 * (RAM-bound, XFER_MAX) -- streaming from disk is a later refinement. */
static int xfer_send_file(const char *path, pp_ctx *ctx)
{
    FILE *f;
    long len;
    pp_u8 *buf;
    int rc;

    f = fopen(path, "rb");
    if (f == (FILE *)0) { render_strn("|12  no bytes on disk for that file.|07\r\n", ctx); return -1; }
    fseek(f, 0L, SEEK_END); len = ftell(f); fseek(f, 0L, SEEK_SET);
    if (len <= 0 || len > XFER_MAX) { fclose(f); render_strn("|12  file too large to send.|07\r\n", ctx); return -1; }
    buf = (pp_u8 *)malloc((size_t)len);
    if (buf == (pp_u8 *)0) { fclose(f); render_strn("|12  out of memory.|07\r\n", ctx); return -1; }
    if (fread(buf, 1, (size_t)len, f) != (size_t)len) { fclose(f); free(buf); return -1; }
    fclose(f);
    render_strn("|10\r\n  start your XMODEM receive now...|07\r\n", ctx);
    rc = xm_send(xm_io_get, xm_io_put, (void *)0, buf, len, 1);
    free(buf);
    render_strn(rc == XM_OK ? "|10\r\n  transfer complete.|07\r\n"
                            : "|12\r\n  transfer aborted.|07\r\n", ctx);
    return rc == XM_OK ? 0 : -1;
}

/* Receive a file over XMODEM into FILES_DIR. Returns bytes received or -1. */
static long xfer_recv_file(const char *name, pp_ctx *ctx)
{
    pp_u8 *buf;
    long got = 0;
    int rc;
    FILE *f;
    char path[96];

    buf = (pp_u8 *)malloc((size_t)XFER_MAX);
    if (buf == (pp_u8 *)0) { render_strn("|12  out of memory.|07\r\n", ctx); return -1; }
    render_strn("|10\r\n  start your XMODEM send now...|07\r\n", ctx);
    rc = xm_recv(xm_io_get, xm_io_put, (void *)0, buf, XFER_MAX, &got);
    if (rc != XM_OK) { free(buf); render_strn("|12\r\n  transfer aborted.|07\r\n", ctx); return -1; }
    sprintf(path, "%s%s", FILES_DIR, name);
    f = fopen(path, "wb");
    if (f != (FILE *)0) { fwrite(buf, 1, (size_t)got, f); fclose(f); }
    free(buf);
    render_strn("|10\r\n  upload received.|07\r\n", ctx);
    return got;
}

/* ---- doors (external programs) ----------------------------------------- */

static void door_add(pp_ctx *ctx)
{
    pp_door d;
    char t[8];
    memset(&d, 0, sizeof d);
    render_strn("|07\r\n  name    |08: |15", ctx); read_field(d.name, PP_DOOR_NAME_MAX);
    if (d.name[0] == '\0') return;
    render_strn("|07  command |08(.bat/.exe)|08: |15", ctx); read_field(d.cmd, PP_DOOR_CMD_MAX);
    render_strn("|07  drop    |08([|151|08]DOOR.SYS [|152|08]DORINFO)|08: |15", ctx);
    read_field(t, (int)sizeof t);
    d.type = (t[0] == '2') ? DROP_DORINFO : DROP_DOORSYS;
    render_strn("|07  acs     |08(- for all)|08: |15", ctx); read_field(d.acs, PP_DOOR_ACS_MAX);
    if (d.acs[0] == '\0') strcpy(d.acs, "-");
    dr_add(&d);
    render_strn("|10  door installed.|07\r\n", ctx); pause_key(ctx);
}

static void door_run(const pp_door *d, const pp_user *u, pp_ctx *ctx)
{
    pp_door_session s;
    int rc;
    s.handle = u->handle;
    s.location = u->location;
    s.node = g_node;
    s.sl = (int)u->sl;
    s.dsl = (int)u->dsl;
    s.seconds_left = 3600L;          /* placeholder: no per-call time bank yet */
    s.ansi = 1;
    s.baud = 38400;
    s.times_on = u->times_called;
    rc = (d->type == DROP_DORINFO) ? door_write_dorinfo(DROP_INFO, &s)
                                   : door_write_doorsys(DROP_SYS, &s);
    if (rc != 0) { render_strn("|12  could not write the drop file.|07\r\n", ctx); pause_key(ctx); return; }
    render_strn("|10\r\n  launching |15", ctx); io_puts(d->name); render_strn("|10 ...|07\r\n", ctx);
    nd_act("in a door");
#if defined(__DOS__)
    system(d->cmd);                  /* EXEC the door; it reads the drop file from cwd */
#else
    render_strn("|08  (on the DOS board this EXECs: |07", ctx); io_puts(d->cmd);
    render_strn("|08 -- drop file written here.)|07\r\n", ctx);
#endif
    pause_key(ctx);
}

static void cmd_doors(const pp_user *u, pp_ctx *ctx)
{
    pp_door d;
    int shown[64];
    for (;;) {
        pp_u32 n = dr_count(), i;
        char num[12];
        int ns = 0, c;
        io_puts(CLEAR);
        render_strn("|07\r\n  |15the door wing|07\r\n\r\n", ctx);
        for (i = 0; i < n && ns < 64; i++) {
            if (dr_get(i, &d) != 0) continue;
            if (!acs_eval(d.acs, u)) continue;
            shown[ns++] = (int)i;
            render_strn("|08  [|15", ctx); u32_to_str((pp_u32)ns, num); io_puts(num);
            render_strn("|08] |07", ctx); io_puts(d.name); io_puts("\r\n");
        }
        if (ns == 0) render_strn("|08  no doors installed yet.\r\n", ctx);
        render_strn("|08\r\n  [|15#|08] enter", ctx);
        if (u->sl >= 255) render_strn("  [|15a|08]dd", ctx);
        render_strn("  [|15q|08]uit |08\372 |07", ctx);
        c = io_getch();
        if (c < 0 || c == 'q' || c == 'Q' || c == '\r' || c == '\n') break;
        if ((c == 'a' || c == 'A') && u->sl >= 255) door_add(ctx);
        else if (c >= '1' && c <= ('0' + ns)) { dr_get((pp_u32)shown[c - '1'], &d); door_run(&d, u, ctx); }
    }
}

/* ---- QWK offline mail (export) ----------------------------------------- */

static void cmd_qwk(const pp_user *u, pp_ctx *ctx)
{
    static const char *tags[3]  = { "gen", "warez", "scene" };
    static const char *names[3] = { "general", "warez", "scene" };
    char path[96];
    int c;
    io_puts(CLEAR);
    render_strn("|07\r\n  |15qwk offline mail|07\r\n\r\n", ctx);
    render_strn("|08  [|15d|08]ownload a .QWK packet  [|15q|08]uit |08\372 |07", ctx);
    c = io_getch();
    if (c == 'd' || c == 'D') {
        pp_msg m;
        int a, total = 0;
        char num[12];
        strcpy(path, "data/QWKMAIL.QWK");
        if (qwk_begin(path, g_cfg.board_name) != 0) {
            render_strn("|12  could not open the packet.|07\r\n", ctx); pause_key(ctx); return;
        }
        for (a = 0; a < 3; a++) {
            int i, nn;
            if (mb_open(tags[a]) != 0) continue;
            nn = mb_count();
            for (i = 0; i < nn; i++) {
                pp_qwk_msg q;
                if (mb_get(i, &m) != 0) continue;
                memset(&q, 0, sizeof q);
                q.conference = (pp_u16)(a + 1);
                q.number = (pp_u32)(i + 1);
                q.when = m.when;
                strncpy(q.to, m.to, QWK_TO_MAX);
                strncpy(q.from, m.from, QWK_FROM_MAX);
                strncpy(q.subject, m.subject, QWK_SUBJ_MAX);
                q.body = m.body;
                qwk_add(&q);
                total++;
            }
            mb_close();
        }
        qwk_finish();
        {
            const char *cn[3];
            cn[0] = names[0]; cn[1] = names[1]; cn[2] = names[2];
            qwk_write_control("data/CONTROL.DAT", g_cfg.board_name, u->location, "sysop", cn, 3);
        }
        render_strn("|10\r\n  packed |15", ctx);
        u32_to_str((pp_u32)total, num); io_puts(num);
        render_strn("|10 messages.|07  [|15x|08]modem-send it, or any key to keep on disk |08\372 |07", ctx);
        c = io_getch();
        if (c == 'x' || c == 'X') xfer_send_file(path, ctx);
        pause_key(ctx);
    }
}

/* Feature commands the menu engine hands off (GOTO/DISPLAY/LOGOFF are its). */
static menu_result cmd_handler(const char *cmd, const char *arg, pp_ctx *ctx, void *user)
{
    (void)user;
    nd_act(cmd);                    /* who's-online reflects the current activity */
    poll_inbox(ctx);                /* surface any pages that arrived while idle */
    if (strcmp(cmd, "MSGAREA") == 0) {
        return cmd_msgarea(arg, ctx, (const pp_user *)user);
    } else if (strcmp(cmd, "EMAIL") == 0) {
        cmd_email((const pp_user *)user, ctx);
    } else if (strcmp(cmd, "VOTING") == 0) {
        cmd_voting((const pp_user *)user, ctx);
    } else if (strcmp(cmd, "FILEAREA") == 0) {
        cmd_filearea(arg, (const pp_user *)user, ctx);
    } else if (strcmp(cmd, "BBSLIST") == 0) {
        cmd_bbslist((const pp_user *)user, ctx);
    } else if (strcmp(cmd, "GFILES") == 0) {
        cmd_gfiles((const pp_user *)user, ctx);
    } else if (strcmp(cmd, "WHOSON") == 0) {
        cmd_whoson(ctx);
    } else if (strcmp(cmd, "PAGE") == 0) {
        cmd_page((const pp_user *)user, ctx);
    } else if (strcmp(cmd, "DOORS") == 0) {
        cmd_doors((const pp_user *)user, ctx);
    } else if (strcmp(cmd, "QWK") == 0) {
        cmd_qwk((const pp_user *)user, ctx);
    } else if (strcmp(cmd, "LASTCALL") == 0) {
        io_puts(CLEAR);
        last_callers(ctx);
        pause_key(ctx);
    } else if (strcmp(cmd, "USERLIST") == 0) {
        user_list(ctx);
        pause_key(ctx);
    } else if (strcmp(cmd, "ONELINER") == 0) {
        char text[72];
        const pp_user *u = (const pp_user *)user;
        io_puts(CLEAR);
        show_oneliners(ctx);
        render_strn(pps_get(PPS_ONELINER_PROMPT), ctx);
        read_field(text, (int)sizeof text);
        if (text[0]) {
            if (tc_blocked(text)) {
                render_strn("|12\r\n  that line tripped the filter -- not posted.|07\r\n", ctx);
                pause_key(ctx);
            } else {
                ol_push(u->handle, text, (pp_u32)time((time_t *)0));
                io_puts(CLEAR);
                show_oneliners(ctx);        /* show the wall with their line on top */
                pause_key(ctx);
            }
        }
    } else if (strcmp(cmd, "STUB") == 0) {
        stub(arg[0] ? arg : "that", ctx);
    } else {
        stub(cmd, ctx);
    }
    return MENU_OK;
}

/* ---- session ------------------------------------------------------------ */

/* ---- login helpers ------------------------------------------------------ */

static int ieq(const char *a, const char *b)
{
    while (*a && *b) {
        int ca = *a, cb = *b;
        if (ca >= 'A' && ca <= 'Z') ca += 32;
        if (cb >= 'A' && cb <= 'Z') cb += 32;
        if (ca != cb) return 0;
        a++; b++;
    }
    return *a == *b;
}

/* Prompt for a brand-new handle: non-empty and not already taken. Returns 1
 * with `handle` set, or 0 if the caller backs out with an empty entry. */
static int pick_handle(char *handle, pp_ctx *ctx)
{
    for (;;) {
        render_strn(pps_get(PPS_NEWHANDLE_PROMPT), ctx);
        read_field(handle, PP_HANDLE_MAX);
        if (handle[0] == '\0') return 0;
        if (ub_find(handle, (pp_user *)0, (pp_u32 *)0)) {
            render_strn(pps_get(PPS_HANDLE_TAKEN), ctx);
            continue;
        }
        return 1;
    }
}

/* One caller, start to finish. The data stores are opened per session so the
 * board picks up changes (and state resets) between callers. */
/* The login matrix: the first screen a caller sees, before authenticating.
 * Shows the matrix art and reads one hotkey. Returns 0=login, 1=new user,
 * 2=goodbye, or -1 on carrier loss. */
static int matrix_login(pp_ctx *ctx)
{
    int k = lbmenu_screen(ctx, ART_MATRIX);     /* render + run the bar */
    if (k < 0) return -1;
    if (k >= 'A' && k <= 'Z') k += 32;           /* fold case */
    if (k == 'l') return 0;
    if (k == 'n') return 1;
    return 2;                                    /* 'g' or anything -> goodbye */
}

/* Verify a returning caller's password (up to 3 tries). Returns 1 on success,
 * or if the account has no password (legacy accounts); 0 on failure. */
static int verify_pw(const pp_user *u, pp_ctx *ctx)
{
    int tries;
    if (u->pwhash[0] == '\0') return 1;          /* no password on file */
    for (tries = 0; tries < 3; tries++) {
        char pw[24], h[PP_PW_MAX];
        render_strn("|07  password|08: |15", ctx);
        read_secret(pw, (int)sizeof pw);
        io_puts("\x1b[0m\r\n");
        pw_hash(pw, h);
        if (strcmp(h, u->pwhash) == 0) return 1;
        render_strn("|12  incorrect.\x1b[0m\r\n", ctx);
    }
    return 0;
}

/* close every per-session store opened at the top of run_session */
static void close_stores(void)
{
    qs_close(); gf_close(); bl_close(); vt_close();
    em_close(); ol_close(); lc_close(); ub_close();
}

static void run_session(int interactive)
{
    pp_ctx ctx;
    pp_user user;
    char handle[PP_HANDLE_MAX];
    char callnum[12];
    pp_u32 index, now;

    ub_open(USER_FILE);
    lc_open(CALL_FILE);
    ol_open(ONE_FILE);
    em_open(MAIL_FILE);
    vt_open(VOTE_FILE);
    bl_open(BBS_FILE);
    gf_open(GFILES_FILE);
    qs_open(QSCAN_FILE);
    now = (pp_u32)time((time_t *)0);

    pp_ctx_init(&ctx);
    ctx.board = g_cfg.board_name;        /* |BN comes from config now */

    {
        int want_new = 0;
        if (interactive) {
            int mx = matrix_login(&ctx);                  /* the login matrix */
            if (mx < 0 || mx == 2) {                      /* carrier loss / goodbye */
                render_strn(pps_get(PPS_GOODBYE), &ctx);
                io_puts("\x1b[0m");
                close_stores();
                return;
            }
            want_new = (mx == 1);
        } else {
            show(ART_GREETING, 0, &ctx);
        }

    {
        int is_new = 0;
        if (want_new && pick_handle(handle, &ctx)) is_new = 1;   /* matrix: new user */
        if (!is_new)
        for (;;) {
            render_strn(pps_get(PPS_HANDLE_PROMPT), &ctx);
            if (interactive) read_field(handle, (int)sizeof handle);
            else             strcpy(handle, "newbie");
            if (handle[0] == '\0') { if (interactive) continue; break; }

            if (interactive && ieq(handle, "new")) {      /* explicit new-user signup */
                if (pick_handle(handle, &ctx)) { is_new = 1; break; }
                continue;                                 /* backed out: re-prompt */
            }
            if (ub_find(handle, &user, &index)) {          /* returning caller */
                if (interactive && !verify_pw(&user, &ctx)) {  /* bad password */
                    render_strn(pps_get(PPS_GOODBYE), &ctx);
                    io_puts("\x1b[0m");
                    close_stores();
                    return;
                }
                user.times_called++;
                user.last_call = now;
                ub_update(index, &user);
                break;
            }
            if (!interactive) { is_new = 1; break; }       /* auto mode: just create */

            /* unknown handle -> say so and offer to sign up (lightbar) */
            render_strn(pps_get(PPS_HANDLE_NOTFOUND), &ctx);
            {
                static const char *yn[2] = { "yes", "no" };
                int ch = lightbar(&ctx, yn, 2, 0);
                io_puts("\x1b[0m\r\n");
                if (ch == 0) { is_new = 1; break; }        /* yes -> create this handle */
                /* no (or carrier loss handled below) -> loop and re-prompt */
                if (ch < 0) { close_stores(); return; }
            }
        }

        if (is_new) {
            memset(&user, 0, sizeof user);
            strncpy(user.handle, handle, PP_HANDLE_MAX - 1);
            if (interactive) {
                if (!new_user_app(&user, &ctx)) {          /* declined / cancelled */
                    render_strn(pps_get(PPS_GOODBYE), &ctx);
                    io_puts("\x1b[0m");
                    close_stores();
                    return;
                }
            } else {
                strcpy(user.location, "somewhere new");
                strcpy(user.tagline,  "just arrived");
                strcpy(user.group,    "none");
            }
            if (user.location[0] == '\0') strcpy(user.location, "parts unknown");
            if (user.tagline[0]  == '\0') strcpy(user.tagline, "(quiet type)");
            if (user.group[0]    == '\0') strcpy(user.group, "none");
            if (ub_count() == 0) { user.sl = 255; user.dsl = 255; }  /* first caller = sysop */
            else { user.sl = g_cfg.new_user_sl; user.dsl = g_cfg.new_user_dsl; }
            user.times_called = 1;
            user.first_call = now;
            user.last_call = now;
            ub_add(&user, &index);
            { char ev[96]; sprintf(ev, "NEW USER: %s from %s (sl %d)",
                  user.handle, user.location, (int)user.sl);
              sy_log(LOG_FILE, ev); }
        }
    }
    }
    g_uindex = index;
    lc_push(user.handle, user.location, now);
    nd_claim(g_node, user.handle, user.location, now);   /* light up our node */
    g_msgcursor = pg_seq();                              /* skip backlog */
    { char ev[96]; sprintf(ev, "%s on from %s (sl %d) [node %d]",
          user.handle, user.location, (int)user.sl, g_node);
      sy_log(LOG_FILE, ev); }

    u32_to_str(user.times_called, callnum);
    ctx.handle   = user.handle;
    ctx.location = user.location;
    ctx.callnum  = callnum;

    io_puts(CLEAR);
    show(ART_WELCOME, 1, &ctx);

    /* private-mail notice */
    {
        int mail = em_count_to(user.handle);
        if (mail > 0) {
            char num[12];
            render_strn("|14\r\n  you have |15", &ctx);
            u32_to_str((pp_u32)mail, num); io_puts(num);
            render_strn("|14 piece(s) of mail waiting. |08(press 'e')|07\r\n", &ctx);
        }
    }

    render_strn(pps_get(PPS_PRESS_KEY), &ctx);
    if (interactive) {
        io_getch();
        nd_act("at the main menu");
        menu_run("MAIN", &ctx, &user, cmd_handler);
    } else {
        /* headless harness: linear, deterministic walkthrough */
        io_puts(CLEAR);
        show(ART_MAINMENU, 0, &ctx);
        io_puts(CLEAR);
        last_callers(&ctx);
    }

    render_strn(pps_get(PPS_GOODBYE), &ctx);
    io_puts("\x1b[0m");

    nd_release(g_node, (pp_u32)time((time_t *)0));   /* free our node slot */
    close_stores();
}

/* ---- waiting-for-callers screen (sysop console, between callers) -------- */

/* Resolve one wfc.ans data token into buf (>= 40 bytes). Returns 1 if known
 * (buf filled), 0 if not (caller emits the token literally). The stores are
 * opened transiently -- WFC only runs while no caller session is active. */
static int wfc_field(int a, int b, char *buf)
{
    time_t now = (time_t)time((time_t *)0);
    struct tm *lt;
    buf[0] = '\0';
    if (a == 'B' && b == 'N') { strncpy(buf, g_cfg.board_name, 39); buf[39] = '\0'; return 1; }
    if (a == 'D' && b == 'T') { lt = localtime(&now);
        if (lt) sprintf(buf, "%04d-%02d-%02d", lt->tm_year + 1900, lt->tm_mon + 1, lt->tm_mday); return 1; }
    if (a == 'T' && b == 'M') { lt = localtime(&now);
        if (lt) sprintf(buf, "%02d:%02d", lt->tm_hour, lt->tm_min); return 1; }
    if (a == 'N' && b == 'O') { u32_to_str((pp_u32)nd_online_count(), buf); return 1; }
    if (a == 'T' && b == 'U') { ub_open(USER_FILE); u32_to_str(ub_count(), buf); ub_close(); return 1; }
    if (a == 'L' && b == 'H') { pp_caller c; lc_open(CALL_FILE);
        if (lc_count() > 0) { lc_get(0, &c); strncpy(buf, c.handle, 39); buf[39] = '\0'; }
        else strcpy(buf, "(nobody yet)");
        lc_close(); return 1; }
    if (a == 'L' && b == 'L') { pp_caller c; lc_open(CALL_FILE);
        if (lc_count() > 0) { lc_get(0, &c); strncpy(buf, c.location, 39); buf[39] = '\0'; }
        else strcpy(buf, "-");
        lc_close(); return 1; }
    if (a == 'C' && b == 'T') {                       /* calls since local midnight */
        pp_caller c; int i, n, cnt = 0; time_t mid = 0; struct tm *m;
        m = localtime(&now);
        if (m) { m->tm_hour = 0; m->tm_min = 0; m->tm_sec = 0; mid = mktime(m); }
        lc_open(CALL_FILE); n = lc_count();
        for (i = 0; i < n; i++) { lc_get(i, &c); if ((time_t)c.when >= mid) cnt++; }
        lc_close();
        u32_to_str((pp_u32)cnt, buf); return 1;
    }
    return 0;
}

/* Paint the WFC screen to the physical console (stdout), splicing live values
 * into wfc.ans's |XY data tokens. Colour is raw ANSI in the art, passed thru. */
static void wfc_show(void)
{
    FILE *f = fopen(ART_WFC, "rb");
    int c;
    if (f == (FILE *)0) return;
    while ((c = fgetc(f)) != EOF) {
        if (c == '|') {
            int a = fgetc(f), b;
            if (a == EOF) { fputc('|', stdout); break; }
            b = fgetc(f);
            if (b == EOF) { fputc('|', stdout); fputc(a, stdout); break; }
            if (a >= 'A' && a <= 'Z' && b >= 'A' && b <= 'Z') {
                char val[40];
                if (wfc_field(a, b, val)) fputs(val, stdout);
                else { fputc('|', stdout); fputc(a, stdout); fputc(b, stdout); }
            } else { fputc('|', stdout); fputc(a, stdout); fputc(b, stdout); }
        } else {
            fputc(c, stdout);
        }
    }
    fclose(f);
    fflush(stdout);
}

int main(int argc, char **argv)
{
    int interactive = 1;
    int i;

    /* args: "auto" -> headless harness; a number 1..PP_MAX_NODES -> node no. */
    for (i = 1; i < argc; i++) {
        if (strcmp(argv[i], "auto") == 0) {
            interactive = 0;
        } else {
            int v = atoi(argv[i]);
            if (v >= 1 && v <= PP_MAX_NODES) g_node = v;
        }
    }

    pps_init();
    pps_load(STR_FILE);
    cfg_load(CONFIG_FILE, &g_cfg);       /* falls back to defaults if absent */
    tc_load(TRASH_FILE);                 /* bad-word list (0 if none) */
    nd_open(NODE_FILE);                  /* multinode presence table */
    pg_open(NODEMSG_FILE);               /* inter-node page/chat bus */
    dr_open(DOORS_FILE);                 /* door registry */
    if (io_init() != 0) return 1;

    /* Serve callers. The telnet backend loops (io_session_begin re-listens);
     * the local console serves one session then returns -1. In auto/test mode
     * we serve exactly one caller so the headless harness terminates.
     * NOTE: clean logoff ('g') frees the node immediately and the loop
     * re-listens (verified). An abrupt disconnect is caught by io_mtcp's
     * inactivity timeout as a backstop, but prompt detection over slirp is
     * still unreliable -- robust carrier detection is phase-5 hardening. */
    for (;;) {
        if (interactive) wfc_show();          /* the board's idle face on the console */
        if (io_session_begin() != 0) break;
        run_session(interactive);
        io_session_end();
        if (!interactive) break;
    }

    dr_close();
    pg_close();
    nd_close();
    io_shutdown();
    return 0;
}
