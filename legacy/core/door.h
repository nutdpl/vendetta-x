#ifndef PIGPEN_DOOR_H
#define PIGPEN_DOOR_H

/*
 * DOOR (external program) support -- registry + drop-file generation.
 *
 * Two parts:
 *  1. DOORS.DAT registry (magic "PDOR") -- an append-only list of doors the
 *     sysop has installed: a display name, the command line to run, which
 *     drop-file dialect the door expects, and an ACS gate. Same 16-byte header
 *     + fixed little-endian record discipline as bbslist.c (model on it).
 *  2. Drop-file writers -- a door is launched by writing a "drop file" (the
 *     caller/session info in a known text format) into the door's directory,
 *     then EXEC'ing the door. The two universal dialects are DOOR.SYS (the
 *     52-line GAP/SBBS standard) and DORINFO1.DEF (the RBBS/QBBS standard).
 *     These writers are PURE (text out to a path), so they unit-test cleanly;
 *     the actual EXEC/FOSSIL hand-off is DOS-specific and lives in main.c.
 *
 * Clean-room: implement the PUBLISHED drop-file field layouts from scratch;
 * copy no code from any BBS. Telnet-era fields (COM port, modem result) are
 * emitted as sensible fixed values (we are not on a UART).
 */
#include "pigtypes.h"

#define PP_DOOR_NAME_MAX  32
#define PP_DOOR_CMD_MAX   64    /* e.g. "C:\\DOORS\\LORD\\START.BAT %1" */
#define PP_DOOR_ACS_MAX   32
#define PP_DOOR_REC      144    /* bytes on disk per registry record */

/* drop-file dialects (pp_door.type) */
#define DROP_DOORSYS   0       /* DOOR.SYS  -- 52-line GAP standard */
#define DROP_DORINFO   1       /* DORINFO1.DEF -- RBBS/QBBS standard */

typedef struct {
    char  name[PP_DOOR_NAME_MAX];
    char  cmd[PP_DOOR_CMD_MAX];     /* command line; %1 = drop-file dir if present */
    pp_u8 type;                     /* DROP_* dialect this door expects */
    char  acs[PP_DOOR_ACS_MAX];     /* who may run it ("-" = everyone) */
} pp_door;

/* The live session info a drop file describes. The board fills this in for the
 * current caller before launching a door. */
typedef struct {
    const char *handle;        /* caller handle (used as the user name) */
    const char *location;      /* city/state-ish free text */
    int         node;          /* node number */
    int         sl;            /* security level */
    int         dsl;           /* download security level */
    long        seconds_left;  /* time remaining this call, in seconds */
    int         ansi;          /* 1 = ANSI/graphics available, 0 = plain */
    int         baud;          /* effective line rate to advertise (e.g. 38400) */
    pp_u32      times_on;      /* caller's total calls (for the "times on" field) */
} pp_door_session;

/* registry */
int    dr_open(const char *path);     /* open/create DOORS.DAT; 0 on success */
void   dr_close(void);
pp_u32 dr_count(void);
int    dr_get(pp_u32 index, pp_door *out);   /* 0 on success */
int    dr_add(const pp_door *d);             /* append; 0 on success */

/* drop-file writers -- write the file at `path`, 0 on success, nonzero on I/O
 * error. They overwrite any existing file. */
int    door_write_doorsys(const char *path, const pp_door_session *s);
int    door_write_dorinfo(const char *path, const pp_door_session *s);

/* serialization (exposed for tests): registry record <-> PP_DOOR_REC bytes */
void   dr_pack(const pp_door *d, pp_u8 *rec);
void   dr_unpack(const pp_u8 *rec, pp_door *d);

#endif /* PIGPEN_DOOR_H */
