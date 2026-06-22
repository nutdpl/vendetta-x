#ifndef PIGPEN_GFILES_H
#define PIGPEN_GFILES_H

/*
 * gfiles.h -- G-FILES / BULLETINS catalog (data/GFILES.DAT).
 *
 * A categorized text-library: each entry names a title, the path to a
 * .ans/.txt file to display, and an ACS expression string gating access.
 * This module is the CATALOG ONLY -- it stores and serves the index of
 * bulletins. DISPLAYING a bulletin's file (and EVALUATING its acs string)
 * is the caller's job.
 *
 * File layout (all little-endian, same discipline as oneliner/bbslist):
 *   header (16 bytes): "PGFL" | u16 version | u16 recsize | u32 count | 4 rsvd
 *   then `count` records of PP_GF_REC (144) bytes each:
 *     off   0  title[48]   (NUL-padded)
 *     off  48  file[64]    (path to .ans/.txt)
 *     off 112  acs[32]     (ACS expression string)
 *
 * Append-only, newest-last: gf_add writes at the end of file.
 */
#include "pigtypes.h"

#define PP_GF_TITLE_MAX 48
#define PP_GF_FILE_MAX  64
#define PP_GF_ACS_MAX   32
#define PP_GF_REC      144

typedef struct {
    char title[PP_GF_TITLE_MAX];   /* shown in the catalog list           */
    char file[PP_GF_FILE_MAX];     /* path to the .ans/.txt to display    */
    char acs[PP_GF_ACS_MAX];       /* ACS expr; caller evaluates it       */
} pp_gfile;

int    gf_open(const char *path);        /* 0 on success */
void   gf_close(void);
pp_u32 gf_count(void);
int    gf_get(int i, pp_gfile *out);     /* i in [0,count); 0 on success */
int    gf_add(const pp_gfile *g);        /* append; persists; 0 on success */

/* exposed for tooling / tests */
void gf_pack(const pp_gfile *g, pp_u8 *rec);
void gf_unpack(const pp_u8 *rec, pp_gfile *g);

#endif /* PIGPEN_GFILES_H */
