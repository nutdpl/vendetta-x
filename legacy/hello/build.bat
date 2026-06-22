@echo off
rem Vendetta/X 16-bit DOS build (verified 2026-06-12, Open Watcom v2 @ C:\WATCOM)
set WATCOM=C:\WATCOM
set PATH=C:\WATCOM\binnt64;%PATH%
set INCLUDE=C:\WATCOM\h
wcl -q -bcl=dos -ml -fe=PIGHELLO.EXE hello.c
