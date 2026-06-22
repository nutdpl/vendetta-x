@echo off
rem VX/16 telnet build -- engine + io_mtcp backend + mTCP library.
rem
rem Mounts expected (set by the build harness / DOSBox-X conf):
rem   C: = Open Watcom    D: = this project (cwd)
rem   E: = mTCP source tree (the dir with TCPINC\ TCPLIB\ INCLUDE\)
rem   F: = scratch dir for .obj output
rem Env: WATCOM=C:\  PATH includes C:\BINW
rem      INCLUDE=C:\H;D:\CORE;D:\IO;E:\TCPINC;E:\INCLUDE
rem
rem mTCP is C++ and GPLv3 -- an EXTERNAL dependency, never vendored here.
rem Sources are compiled to objects individually (the DOS 127-char command
rem line can't hold a one-shot wcl), then linked via mtcp.lnk.

rem --- mTCP library (large model, 8086 codegen, our feature config) ---
wpp e:\tcplib\packet.cpp   -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\packet.obj
wpp e:\tcplib\arp.cpp      -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\arp.obj
wpp e:\tcplib\eth.cpp      -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\eth.obj
wpp e:\tcplib\ip.cpp       -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\ip.obj
wpp e:\tcplib\tcp.cpp      -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\tcp.obj
wpp e:\tcplib\tcpsockm.cpp -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\tcpsockm.obj
wpp e:\tcplib\udp.cpp      -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\udp.obj
wpp e:\tcplib\dns.cpp      -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\dns.obj
wpp e:\tcplib\utils.cpp    -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\utils.obj
wpp e:\tcplib\timer.cpp    -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\timer.obj
wpp e:\tcplib\trace.cpp    -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\trace.obj
wasm -0 -ml e:\tcplib\ipasm.asm -fo=f:\ipasm.obj

rem --- Vendetta/X engine + telnet backend ---
wcc d:\core\render.c   -bt=dos -ml -0 -fo=f:\render.obj
wcc d:\core\strtab.c   -bt=dos -ml -0 -fo=f:\strtab.obj
wcc d:\core\telnet.c   -bt=dos -ml -0 -fo=f:\telnet.obj
wcc d:\core\userbase.c -bt=dos -ml -0 -fo=f:\userbase.obj
wcc d:\core\callers.c  -bt=dos -ml -0 -fo=f:\callers.obj
wcc d:\core\oneliner.c -bt=dos -ml -0 -fo=f:\oneliner.obj
wcc d:\core\msgbase.c  -bt=dos -ml -0 -fo=f:\msgbase.obj
wcc d:\core\editor.c   -bt=dos -ml -0 -fo=f:\editor.obj
wcc d:\core\lightbar.c -bt=dos -ml -0 -fo=f:\lightbar.obj
wcc d:\core\acs.c      -bt=dos -ml -0 -fo=f:\acs.obj
wcc d:\core\config.c   -bt=dos -ml -0 -fo=f:\config.obj
wcc d:\core\email.c    -bt=dos -ml -0 -fo=f:\email.obj
wcc d:\core\voting.c   -bt=dos -ml -0 -fo=f:\voting.obj
wcc d:\core\filebase.c -bt=dos -ml -0 -fo=f:\filebase.obj
wcc d:\core\bbslist.c  -bt=dos -ml -0 -fo=f:\bbslist.obj
wcc d:\core\gfiles.c   -bt=dos -ml -0 -fo=f:\gfiles.obj
wcc d:\core\qscan.c    -bt=dos -ml -0 -fo=f:\qscan.obj
wcc d:\core\trashcan.c -bt=dos -ml -0 -fo=f:\trashcan.obj
wcc d:\core\syslog.c   -bt=dos -ml -0 -fo=f:\syslog.obj
wcc d:\core\node.c     -bt=dos -ml -0 -fo=f:\node.obj
wcc d:\core\page.c     -bt=dos -ml -0 -fo=f:\page.obj
wcc d:\core\door.c     -bt=dos -ml -0 -fo=f:\door.obj
wcc d:\core\xmodem.c   -bt=dos -ml -0 -fo=f:\xmodem.obj
wcc d:\core\qwk.c      -bt=dos -ml -0 -fo=f:\qwk.obj
wcc d:\core\menu.c     -bt=dos -ml -0 -fo=f:\menu.obj
wcc d:\src\main.c      -bt=dos -ml -0 -fo=f:\main.obj
wpp d:\io\io_mtcp.cpp -bt=dos -ml -0 -dCFG_H="vxmtcp.cfg" -fo=f:\io_mtcp.obj

rem --- link (response file dodges the command-line limit) ---
wlink @d:\mtcp.lnk
