package main

import (
	"fmt"
	"strings"
	"testing"

	"vendetta-x/server/internal/term"
)

func TestPresenceSendDrain(t *testing.T) {
	p := newPresence()
	n1 := p.join("alice")
	if !p.send(n1, "hi alice") {
		t.Fatal("send to an online node should succeed")
	}
	if p.send(9999, "nope") {
		t.Error("send to a missing node should fail")
	}
	msgs := p.drain(n1)
	if len(msgs) != 1 || msgs[0] != "hi alice" {
		t.Fatalf("drain = %v, want [hi alice]", msgs)
	}
	if p.drain(n1) != nil {
		t.Error("a second drain should be empty")
	}
}

func TestSendNodeMessageFlow(t *testing.T) {
	b := newTestBoard(t)
	user, _ := b.st.UserByHandle("nut")
	target := b.pres.join("bob")

	se := runOnSession(t, b, func(s *term.Session) { b.sendNodeMessage(s, user) })
	se.expect("Send to node")
	se.send(fmt.Sprintf("%d\r", target))
	se.expect("Message:")
	se.send("meet me in teleconf\r")
	se.expect("Sent to node")
	se.send(" ")
	se.drain()
	se.waitDone()

	msgs := b.pres.drain(target)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "meet me in teleconf") || !strings.Contains(msgs[0], "nut") {
		t.Fatalf("queued node message = %v", msgs)
	}
}

func TestSendNodeMessageNoSuchNode(t *testing.T) {
	b := newTestBoard(t)
	user, _ := b.st.UserByHandle("nut")
	se := runOnSession(t, b, func(s *term.Session) { b.sendNodeMessage(s, user) })
	se.expect("Send to node")
	se.send("42\r")
	se.expect("Message:")
	se.send("anyone home\r")
	se.expect("No one is on node 42")
	se.send(" ")
	se.drain()
	se.waitDone()
}

func TestDeliverNodeMessages(t *testing.T) {
	b := newTestBoard(t)
	node := b.pres.join("nut")
	b.pres.send(node, "node message from phantom: yo")

	se := runOnSession(t, b, func(s *term.Session) { b.deliverNodeMessages(s, node) })
	se.expect("node message from phantom: yo")
	se.send(" ") // dismiss the pause
	se.drain()
	se.waitDone()

	if b.pres.drain(node) != nil {
		t.Error("delivery should have cleared the inbox")
	}
}

func TestWhosOnlineSendPath(t *testing.T) {
	b := newTestBoard(t)
	user, _ := b.st.UserByHandle("nut")
	target := b.pres.join("bob")

	se := runOnSession(t, b, func(s *term.Session) { b.whosOnline(s, user) })
	se.expect("who's online")
	se.expect("end to a node") // "[S]end to a node" -- the S is colored separately
	se.send("s\r")
	se.expect("Send to node")
	se.send(fmt.Sprintf("%d\r", target))
	se.expect("Message:")
	se.send("hey there\r")
	se.expect("Sent to node")
	se.send(" ")   // dismiss pause
	se.send("q\r") // leave who's-online
	se.drain()
	se.waitDone()

	if msgs := b.pres.drain(target); len(msgs) != 1 {
		t.Fatalf("expected one queued message on the target node, got %v", msgs)
	}
}
