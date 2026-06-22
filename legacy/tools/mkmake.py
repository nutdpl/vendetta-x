#!/usr/bin/env python3
# Minimal reimplementation of Watt-32's util/mkmake makefile preprocessor.
# Selects @ifdef/@ifndef/@elifdef/@elifndef/@else/@endif sections from
# makefile.all given a set of defined symbols, and (for WATCOM) rewrites
# trailing '\' line-continuations to '&'. Enough for: mkmake.py WATCOM FLAT
import sys

defs = set(sys.argv[1:])              # e.g. {"WATCOM","FLAT"}
watcom = "WATCOM" in defs

def any_def(toks):  return any(t in defs for t in toks)

# stack of frames: each is [emitting_now, any_branch_taken_yet, parent_active]
stack = []
def active():       return all(f[0] for f in stack) if stack else True

out = []
for raw in open("makefile.all", "r", errors="replace"):
    line = raw.rstrip("\n")
    s = line.strip()
    if s.startswith("@"):
        parts = s[1:].split()
        d = parts[0]; toks = parts[1:]
        parent = active()  # is the enclosing context emitting?
        if d == "ifdef" or d == "ifndef":
            # push: need parent active AND this condition
            cond = any_def(toks) if d == "ifdef" else not any_def(toks)
            # parent active = all current frames emitting
            penv = all(f[0] for f in stack) if stack else True
            stack.append([penv and cond, cond, penv])
        elif d in ("elifdef", "elifndef", "else"):
            fr = stack[-1]
            penv = fr[2]
            if d == "else":
                cond = not fr[1]
            else:
                c = any_def(toks) if d == "elifdef" else not any_def(toks)
                cond = (not fr[1]) and c
            fr[0] = penv and cond
            fr[1] = fr[1] or cond
        elif d == "endif":
            stack.pop()
        continue
    if active():
        if watcom and line.endswith("\\"):
            line = line[:-1] + "&"
        out.append(line)

sys.stdout.write("\n".join(out) + "\n")
