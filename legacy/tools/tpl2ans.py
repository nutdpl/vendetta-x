#!/usr/bin/env python3
"""Convert \\e template to UTF-8 .ans (and optional CP437 .ans).

The editable source format for hand-drawn art: write \\e where an ESC goes,
keep the file as readable UTF-8, then compile to a real CP437 .ans for the
board. (part of the art pipeline.)

usage: python tpl2ans.py in.tpl out-utf8.ans [out-cp437.ans]
"""
import sys

t = open(sys.argv[1], encoding="utf-8").read().replace("\\e", "\x1b")
open(sys.argv[2], "w", encoding="utf-8", newline="\n").write(t)
if len(sys.argv) > 3:
    open(sys.argv[3], "wb").write(t.encode("cp437"))
print("ok")
