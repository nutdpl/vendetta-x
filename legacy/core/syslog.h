#ifndef PIGPEN_SYSLOG_H
#define PIGPEN_SYSLOG_H

/*
 * syslog.c -- the SYSOP ACTIVITY LOG / audit trail (data/SYSOP.LOG).
 *
 * Unlike the binary stores (oneliner/userbase/callers), this is a plain,
 * human-readable text log so a sysop can `type SYSOP.LOG` from DOS or tail it
 * on the WFC screen. Every interesting event -- logons, validations, admin
 * actions, page-sysop alerts -- gets one timestamped line appended.
 *
 * Robust by design: each call opens "a", writes one line, closes. No global
 * state, no held handle, nothing to corrupt if the program dies mid-session.
 */

int sy_log(const char *path, const char *event);
/*
 * Append one line: "[YYYY-MM-DD HH:MM] <event>\n" using localtime.
 * Returns 0 on success, non-zero on failure (bad path / I/O error).
 */

int sy_tail(const char *path, char *buf, int max, int lines);
/*
 * Fill buf with the last `lines` lines of the log (for a WFC log-tail
 * display). Best-effort: NUL-terminates buf, never overflows `max`.
 * Returns the number of lines actually placed into buf.
 */

#endif /* PIGPEN_SYSLOG_H */
