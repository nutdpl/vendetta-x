#ifndef PIGPEN_PIGTYPES_H
#define PIGPEN_PIGTYPES_H

/*
 * Fixed-width integer types. 32-bit-clean from line one (DESIGN.md): the
 * core must compile identically under 16-bit Watcom DOS (int = 16 bits,
 * long = 32) and a modern host (int = 32, long = 64). C89 has no <stdint.h>,
 * so we pick exact widths from <limits.h> rather than assuming sizeof(int).
 */
#include <limits.h>

typedef unsigned char  pp_u8;
typedef signed   char  pp_i8;
typedef unsigned short pp_u16;
typedef signed   short pp_i16;

#if UINT_MAX == 0xFFFFU
typedef unsigned long  pp_u32;   /* 16-bit target: int is 16 bits, use long */
typedef signed   long  pp_i32;
#else
typedef unsigned int   pp_u32;   /* 32/64-bit host: int is 32 bits */
typedef signed   int   pp_i32;
#endif

#endif /* PIGPEN_PIGTYPES_H */
