# Vendetta/X host build -- the co-equal test bench (DESIGN.md).
# The DOS ship binary is built with Open Watcom via mk.bat; this builds the
# same strict-C89 core natively for instant iteration and real debuggers.

CC      ?= cc
CFLAGS  ?= -std=c89 -pedantic -Wall -Wextra -Icore -Iio
SRC      = core/render.c core/strtab.c core/userbase.c core/callers.c \
           core/oneliner.c core/msgbase.c core/editor.c core/lightbar.c \
           core/acs.c core/config.c core/email.c core/voting.c \
           core/filebase.c core/bbslist.c core/gfiles.c core/qscan.c \
           core/trashcan.c core/syslog.c core/node.c core/page.c \
           core/door.c core/xmodem.c core/qwk.c \
           core/menu.c core/lbmenu.c io/io_local.c src/main.c
HDR      = core/render.h core/strtab.h core/userbase.h core/callers.h \
           core/oneliner.h core/msgbase.h core/editor.h core/lightbar.h \
           core/acs.h core/config.h core/email.h core/voting.h \
           core/filebase.h core/bbslist.h core/gfiles.h core/qscan.h \
           core/trashcan.h core/syslog.h core/node.h core/page.h \
           core/door.h core/xmodem.h core/qwk.h \
           core/menu.h core/lbmenu.h core/pigtypes.h io/ppio.h

# VXCFG -- the standalone full-screen sysop config editor (separate program).
CFGSRC   = src/vxcfg.c core/config.c core/render.c core/lightbar.c io/io_local.c

vendx: $(SRC) $(HDR)
	$(CC) $(CFLAGS) $(SRC) -o $@

# net: same engine, telnet over BSD sockets / Winsock (io_sock) instead of the
# console. Runs as a real telnet server on the host, Win32, and Windows 98 --
# no DOS, no packet driver. core/telnet.c (the IAC codec) is linked in;
# io_local is swapped out. This is the host-testable, Win98-ready transport.
NETSRC = $(filter-out io/io_local.c,$(SRC)) core/telnet.c io/io_sock.c
vendx-net: $(NETSRC) $(HDR) core/telnet.h
	$(CC) $(CFLAGS) $(NETSRC) -o $@

# net-smoke: build the sockets board, run it, connect as a caller, and assert
# the matrix screen comes back -- "it answers telnet", proven with no DOS.
net-smoke: vendx-net
	@sh tools/net-smoke.sh

vxcfg: $(CFGSRC) core/config.h
	$(CC) $(CFLAGS) $(CFGSRC) -o $@

all: vendx vxcfg

# render_dump: host tool that runs the real renderer over a screen file and
# dumps raw ANSI to stdout (see tools/README.md). Not part of the board.
render_dump: tools/render_dump.c core/render.c core/render.h core/lbmenu.h
	$(CC) $(CFLAGS) tools/render_dump.c core/render.c -o $@

# shots: render screens to PNG via the ANSI->PNG feedback loop (needs Pillow).
# Kept out of `test`/CI -- it's a dev convenience, not a gate.
shots: render_dump
	@mkdir -p shots
	./render_dump art/lbdemo.pp 1 | python3 tools/ansi2png.py - shots/lbdemo.png
	./render_dump art/sysinfo.pp  | python3 tools/ansi2png.py - shots/sysinfo.png

# Host unit tests for transport-/IO-independent logic.
test: core/telnet.c core/userbase.c core/acs.c core/config.c core/email.c \
      core/voting.c core/filebase.c core/bbslist.c
	$(CC) $(CFLAGS) -Itests tests/telnet_test.c   core/telnet.c   -o telnet_test
	$(CC) $(CFLAGS) -Itests tests/userbase_test.c core/userbase.c -o userbase_test
	$(CC) $(CFLAGS) -Itests tests/acs_test.c      core/acs.c core/userbase.c -o acs_test
	$(CC) $(CFLAGS) -Itests tests/config_test.c   core/config.c   -o config_test
	$(CC) $(CFLAGS) -Itests tests/email_test.c    core/email.c    -o email_test
	$(CC) $(CFLAGS) -Itests tests/voting_test.c   core/voting.c   -o voting_test
	$(CC) $(CFLAGS) -Itests tests/filebase_test.c core/filebase.c -o filebase_test
	$(CC) $(CFLAGS) -Itests tests/bbslist_test.c  core/bbslist.c  -o bbslist_test
	$(CC) $(CFLAGS) -Itests tests/gfiles_test.c   core/gfiles.c   -o gfiles_test
	$(CC) $(CFLAGS) -Itests tests/qscan_test.c    core/qscan.c    -o qscan_test
	$(CC) $(CFLAGS) -Itests tests/trashcan_test.c core/trashcan.c -o trashcan_test
	$(CC) $(CFLAGS) -Itests tests/syslog_test.c   core/syslog.c   -o syslog_test
	$(CC) $(CFLAGS) -Itests tests/node_test.c     core/node.c     -o node_test
	$(CC) $(CFLAGS) -Itests tests/page_test.c     core/page.c     -o page_test
	$(CC) $(CFLAGS) -Itests tests/door_test.c     core/door.c     -o door_test
	$(CC) $(CFLAGS) -Itests tests/xmodem_test.c   core/xmodem.c   -o xmodem_test
	$(CC) $(CFLAGS) -Itests tests/qwk_test.c      core/qwk.c      -o qwk_test
	$(CC) $(CFLAGS) -Itests tests/render_test.c   core/render.c   -o render_test
	$(CC) $(CFLAGS) -Itests tests/frame_test.c    core/frame.c    -o frame_test
	$(CC) $(CFLAGS) -Itests tests/lightbar_test.c core/lightbar.c -o lightbar_test
	$(CC) $(CFLAGS) -Itests tests/msgbase_test.c  core/msgbase.c  -o msgbase_test
	$(CC) $(CFLAGS) -Itests tests/lbmenu_test.c   core/lbmenu.c core/render.c -o lbmenu_test
	./telnet_test && ./userbase_test && ./acs_test && ./config_test && \
	./email_test && ./voting_test && ./filebase_test && ./bbslist_test && \
	./gfiles_test && ./qscan_test && ./trashcan_test && ./syslog_test && \
	./node_test && ./page_test && ./door_test && ./xmodem_test && ./qwk_test && \
	./render_test && ./frame_test && ./lightbar_test && ./msgbase_test && \
	./lbmenu_test

clean:
	rm -f vendx vxcfg *_test

.PHONY: clean test
