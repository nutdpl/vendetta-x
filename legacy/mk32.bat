@echo off
rem VX/32 io_local DOS build (Open Watcom, 32-bit protected mode, flat model).
rem The first-class hardware target: flat memory, no 64k DGROUP wall, DOS/4GW.
rem Run from the project root with WATCOM/INCLUDE/PATH set (INCLUDE>=C:\H).
rem Objects compiled individually + linked via dos32.lnk (mirrors mk.bat).
rem Telnet build is separate: see runwatt.sh / tools/build-watt32.sh (io_watt.c).
wcc386 core\render.c   -q -bt=dos -mf -3r -i=core -i=io -fo=render.obj
wcc386 core\strtab.c   -q -bt=dos -mf -3r -i=core -i=io -fo=strtab.obj
wcc386 core\userbase.c -q -bt=dos -mf -3r -i=core -i=io -fo=userbase.obj
wcc386 core\callers.c  -q -bt=dos -mf -3r -i=core -i=io -fo=callers.obj
wcc386 core\oneliner.c -q -bt=dos -mf -3r -i=core -i=io -fo=oneliner.obj
wcc386 core\msgbase.c  -q -bt=dos -mf -3r -i=core -i=io -fo=msgbase.obj
wcc386 core\editor.c   -q -bt=dos -mf -3r -i=core -i=io -fo=editor.obj
wcc386 core\lightbar.c -q -bt=dos -mf -3r -i=core -i=io -fo=lightbar.obj
wcc386 core\acs.c      -q -bt=dos -mf -3r -i=core -i=io -fo=acs.obj
wcc386 core\config.c   -q -bt=dos -mf -3r -i=core -i=io -fo=config.obj
wcc386 core\email.c    -q -bt=dos -mf -3r -i=core -i=io -fo=email.obj
wcc386 core\voting.c   -q -bt=dos -mf -3r -i=core -i=io -fo=voting.obj
wcc386 core\filebase.c -q -bt=dos -mf -3r -i=core -i=io -fo=filebase.obj
wcc386 core\bbslist.c  -q -bt=dos -mf -3r -i=core -i=io -fo=bbslist.obj
wcc386 core\gfiles.c   -q -bt=dos -mf -3r -i=core -i=io -fo=gfiles.obj
wcc386 core\qscan.c    -q -bt=dos -mf -3r -i=core -i=io -fo=qscan.obj
wcc386 core\trashcan.c -q -bt=dos -mf -3r -i=core -i=io -fo=trashcan.obj
wcc386 core\syslog.c   -q -bt=dos -mf -3r -i=core -i=io -fo=syslog.obj
wcc386 core\node.c     -q -bt=dos -mf -3r -i=core -i=io -fo=node.obj
wcc386 core\page.c     -q -bt=dos -mf -3r -i=core -i=io -fo=page.obj
wcc386 core\door.c     -q -bt=dos -mf -3r -i=core -i=io -fo=door.obj
wcc386 core\xmodem.c   -q -bt=dos -mf -3r -i=core -i=io -fo=xmodem.obj
wcc386 core\qwk.c      -q -bt=dos -mf -3r -i=core -i=io -fo=qwk.obj
wcc386 core\menu.c     -q -bt=dos -mf -3r -i=core -i=io -fo=menu.obj
wcc386 core\lbmenu.c   -q -bt=dos -mf -3r -i=core -i=io -fo=lbmenu.obj
wcc386 io\io_local.c   -q -bt=dos -mf -3r -i=core -i=io -fo=io_local.obj
wcc386 src\main.c      -q -bt=dos -mf -3r -i=core -i=io -fo=main.obj
wlink @dos32.lnk
