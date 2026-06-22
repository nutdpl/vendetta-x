#ifndef PIGPEN_ACS_H
#define PIGPEN_ACS_H

/*
 * ACS -- Access Condition String evaluator (DESIGN.md / feature study: the
 * security spine). Menus, message areas, files and doors all gate on an ACS
 * expression evaluated against the caller. Clean-room grammar (our own):
 *
 *   atom :=  SL <op> <n>        security level   (op: >= <= > < =)
 *         |  DSL <op> <n>       download level
 *         |  AR:<L>             has access-right bit L     (L in A..P)
 *         |  DAR:<L>            has dir-access-right bit L
 *         |  R:<L>              has restriction bit L
 *         |  SYSOP              SL >= 255
 *         |  ANY | -            always true
 *         |  ( expr )
 *   expr := atom, combined with  !  (not),  &  (and),  |  (or)   ! > & > |
 *
 * Empty / NULL / "-" / "any" -> true (no restriction). Spaces are ignored, so
 * "SL>=20 & AR:B" and "SL>=20&AR:B" are equivalent.
 */
#include "userbase.h"

int acs_eval(const char *acs, const pp_user *u);

#endif /* PIGPEN_ACS_H */
