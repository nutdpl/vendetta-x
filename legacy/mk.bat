@echo off
rem VX/16 io_local DOS build (Open Watcom, real-mode large model).
rem Run from the project root with WATCOM/INCLUDE/PATH set (INCLUDE>=C:\H).
rem Sources are compiled to objects individually + linked via vendx.lnk: a
rem one-shot wcl with this many files overflows the DOS 127-char command line.
wcc core\render.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=render.obj
wcc core\strtab.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=strtab.obj
wcc core\userbase.c -q -bt=dos -ml -zt256 -i=core -i=io -fo=userbase.obj
wcc core\callers.c  -q -bt=dos -ml -zt256 -i=core -i=io -fo=callers.obj
wcc core\oneliner.c -q -bt=dos -ml -zt256 -i=core -i=io -fo=oneliner.obj
wcc core\msgbase.c  -q -bt=dos -ml -zt256 -i=core -i=io -fo=msgbase.obj
wcc core\editor.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=editor.obj
wcc core\lightbar.c -q -bt=dos -ml -zt256 -i=core -i=io -fo=lightbar.obj
wcc core\acs.c      -q -bt=dos -ml -zt256 -i=core -i=io -fo=acs.obj
wcc core\config.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=config.obj
wcc core\email.c    -q -bt=dos -ml -zt256 -i=core -i=io -fo=email.obj
wcc core\voting.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=voting.obj
wcc core\filebase.c -q -bt=dos -ml -zt256 -i=core -i=io -fo=filebase.obj
wcc core\bbslist.c  -q -bt=dos -ml -zt256 -i=core -i=io -fo=bbslist.obj
wcc core\gfiles.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=gfiles.obj
wcc core\qscan.c    -q -bt=dos -ml -zt256 -i=core -i=io -fo=qscan.obj
wcc core\trashcan.c -q -bt=dos -ml -zt256 -i=core -i=io -fo=trashcan.obj
wcc core\syslog.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=syslog.obj
wcc core\node.c     -q -bt=dos -ml -zt256 -i=core -i=io -fo=node.obj
wcc core\page.c     -q -bt=dos -ml -zt256 -i=core -i=io -fo=page.obj
wcc core\door.c     -q -bt=dos -ml -zt256 -i=core -i=io -fo=door.obj
wcc core\xmodem.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=xmodem.obj
wcc core\qwk.c      -q -bt=dos -ml -zt256 -i=core -i=io -fo=qwk.obj
wcc core\menu.c     -q -bt=dos -ml -zt256 -i=core -i=io -fo=menu.obj
wcc core\lbmenu.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=lbmenu.obj
wcc io\io_local.c   -q -bt=dos -ml -zt256 -i=core -i=io -fo=io_local.obj
wcc src\main.c      -q -bt=dos -ml -zt256 -i=core -i=io -fo=main.obj
wlink @vendx.lnk
