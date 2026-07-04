# Vendetta/X

```
 ::: VENDETTA/X :::::::::::::::::::::::::::::::::::::::::::::: dpl productions :::
        a bulletin board for people who never quite logged off
 :::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::: est. 2026 :::
```

So it's a BBS. A real one. The kind you call up at 1 a.m., pick a handle, and
end up arguing in the message bases until the sun comes up. There's mail
waiting, new files to grab, doors to lose a whole evening in, and an oneliner
wall that's usually a disaster. Same as it ever was.

The catch is you don't need a modem anymore. Vendetta/X answers three ways at
once — plain old **telnet**, encrypted **ssh**, or straight out of a **web
browser** — and underneath it's all one board. Drop a message over telnet,
somebody reads it on the web and writes back, and you watch them appear in the
who's-online list while you're still connected. Different doors, same room.

## Dialing in

```
   telnet   port 2323     the genuine article: CP437 ANSI, lightbar menus
   ssh      port 2222     same board, on an encrypted line
   www      port 8080     just point a browser at it
```

First time? Connect, pick a handle, make an account. You're in.

## What's on the board

  * **message bases** — the heart of the place. read, post, reply, new-scan.
  * **electronic mail** — private messages between callers, unread counts and all.
  * **file areas** — browse, download, upload your own warez (or, you know,
    text files).
  * **doors** — outside games. two come built in: a LORD-style dungeon crawl
    and a TradeWars-style space-trading grind. it'll run honest-to-god DOS
    doors too, if you're feeling brave.
  * **g-philes** — the text library. rants, how-tos, ascii, whatever you've got.
  * **the wall** — leave an oneliner on your way out.
  * **teleconference** — live chat, for when there's more than one of you on.
  * **voting booth** — polls. start your own if you've got opinions.
  * **bbs list** — numbers for other boards worth your time.
  * **qwk mail** — pack the messages into a .QWK, read 'em offline, upload your
    replies later.
  * **message networks** — the board is a real FTN node: join fsxNet, FidoNet,
    AgoraNet and friends (and DOVE-Net over QWK), and your bases carry traffic
    from boards all over the world.

...plus the usual: user list, last callers, your stats, and a settings screen
to fix your profile and password.

## Want to run your own?

Vendetta/X is one program. No database server to babysit, no tangle of
dependencies to install — it keeps the whole board in a single file and runs
just about anywhere.

```sh
cd server
go build .
./server          # telnet :2323   ssh :2222   www :8080
```

You run the board from a sysop panel in the browser at `/sysop`. Message bases,
file areas, users, the name over the door, what's turned on and what's not —
it's all there, live, no config files to hand-edit and nothing to recompile.
The board ships with a `nut` (sysop) and a `phantom` (regular caller) account;
neither has a password until the first login sets one.

Rather have it in a container? `docker build -t vendetta-x .` from the repo root
and you're running. There's more for sysops in `docs/` — door setup, deployment
notes, the lot.

## Under the hood, for the curious

This thing's meant to sit on the open internet, not just a friendly LAN.
Passwords are hashed, logins get throttled per address, the web side can run
over TLS, idle nodes get reaped, and one wedged session can't take the whole
board down with it. If you want the real engineering write-up it's in
`DESIGN.md`.

There's also a `legacy/` tree — the original board, written in straight C, that
cross-builds all the way down to actual DOS. It's not what runs today, but it's
where Vendetta/X started, and it's kept around for anyone who still wants to
call a board off a 486.

## Credits

By **nut** / dpl productions. Released under the MIT license — copy it, fork it,
run your own board, change the name. See `LICENSE`.

```
   :: NO CARRIER ::::::::::::::::::::::::::::::::::::::::::::::::::::::: 2026 ::
```
