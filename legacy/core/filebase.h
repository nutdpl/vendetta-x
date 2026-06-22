#ifndef PIGPEN_FILEBASE_H
#define PIGPEN_FILEBASE_H

/*
 * File areas (DESIGN.md -- the file catalog layer; NO transfer protocol). Each
 * area is its own file data/<TAG>.FIL: a 16-byte header ("PFIL" | u16 ver | u16
 * recsize | u32 count | 4 rsvd) then append-only fixed FB_REC (128) byte
 * records describing one cataloged file each. Strings are fixed NUL-padded
 * fields, all integers little-endian, serialized field by field. One area open
 * at a time (single node), mirroring core/msgbase.c.
 */
#include "pigtypes.h"

#define FB_NAME     16    /* 8.3 filename, NUL-padded */
#define FB_DESC     64    /* short description */
#define FB_UPLOADER 32    /* uploader handle */
#define FB_REC      128   /* on-disk record size */

typedef struct {
    char   name[FB_NAME];        /* 8.3 filename */
    char   desc[FB_DESC];        /* description */
    pp_u32 size;                 /* file size in bytes */
    char   uploader[FB_UPLOADER];/* uploader handle */
    pp_u32 when;                 /* unix time uploaded */
    pp_u32 downloads;            /* download counter */
    pp_u32 flags;                /* area-specific flags */
} pp_file;

int  fb_open(const char *tag);   /* data/<TAG>.FIL, creating if absent; 0 ok */
void fb_close(void);
int  fb_count(void);
int  fb_get(int index, pp_file *out);
int  fb_add(const pp_file *f);
int  fb_inc_downloads(int index); /* bump downloads counter and persist; 0 ok */

#endif /* PIGPEN_FILEBASE_H */
