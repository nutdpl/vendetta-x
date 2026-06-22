// Package chat is the multi-user teleconference: the live, multi-node chat
// room that makes the board feel inhabited. Callers Join a named channel and
// receive a stream of Messages; anything one member Says is broadcast to every
// other member of that channel, plus system join/leave notices.
//
// This file is the CONTRACT. The Hub must be safe for concurrent use by many
// goroutines (one per telnet/web caller). Bodies stub out until implemented.
package chat

import (
	"sort"
	"sync"
	"time"
)

// recvCap is the buffer size of each member's receive channel. When a
// member's buffer is full, broadcasts to that member are dropped rather than
// blocking the broadcaster.
const recvCap = 64

// Message is one line delivered to a channel member.
type Message struct {
	From string    // handle of the speaker, or "" for system notices
	Text string    // the line of text
	Sys  bool      // true for join/leave/system notices
	At   time.Time // when it was said
}

// Hub owns all channels and their members. Create one per server.
type Hub struct {
	mu       sync.Mutex
	channels map[string]map[*Member]struct{}
}

// NewHub returns a ready Hub.
func NewHub() *Hub {
	return &Hub{
		channels: make(map[string]map[*Member]struct{}),
	}
}

// Member is one participant's presence on one channel. Obtain via Hub.Join.
type Member struct {
	hub     *Hub
	channel string
	handle  string
	recv    chan Message
	left    bool // guarded by hub.mu; true once Leave has run
}

// Join adds handle to channel and returns the Member. A system "has joined"
// notice is broadcast to the rest of the channel.
func (h *Hub) Join(channel, handle string) *Member {
	m := &Member{
		hub:     h,
		channel: channel,
		handle:  handle,
		recv:    make(chan Message, recvCap),
	}

	msg := Message{From: "", Text: handle + " has joined", Sys: true, At: time.Now()}

	h.mu.Lock()
	members := h.channels[channel]
	if members == nil {
		members = make(map[*Member]struct{})
		h.channels[channel] = members
	}
	// Broadcast to the existing members (not the joiner) before adding.
	broadcastLocked(members, msg)
	members[m] = struct{}{}
	h.mu.Unlock()

	return m
}

// Who returns the handles currently in channel, sorted.
func (h *Hub) Who(channel string) []string {
	h.mu.Lock()
	members := h.channels[channel]
	handles := make([]string, 0, len(members))
	for m := range members {
		handles = append(handles, m.handle)
	}
	h.mu.Unlock()

	sort.Strings(handles)
	return handles
}

// Recv is the member's inbound stream. It is closed when the member Leaves.
// Reads must never block the broadcaster: a slow reader drops lines rather
// than stalling the channel.
func (m *Member) Recv() <-chan Message {
	return m.recv
}

// Say broadcasts text from this member to everyone else on the channel.
func (m *Member) Say(text string) {
	msg := Message{From: m.handle, Text: text, Sys: false, At: time.Now()}

	h := m.hub
	h.mu.Lock()
	members := h.channels[m.channel]
	// Send to everyone except the speaker.
	for other := range members {
		if other == m {
			continue
		}
		select {
		case other.recv <- msg:
		default:
			// Receiver's buffer is full; drop the line for them.
		}
	}
	h.mu.Unlock()
}

// Leave removes the member, closes its Recv channel, and broadcasts a system
// "has left" notice. Safe to call once.
func (m *Member) Leave() {
	msg := Message{From: "", Text: m.handle + " has left", Sys: true, At: time.Now()}

	h := m.hub
	h.mu.Lock()
	if m.left {
		// Already left; idempotent no-op (do not double-close).
		h.mu.Unlock()
		return
	}
	m.left = true

	members := h.channels[m.channel]
	// Remove the member from the set BEFORE closing its channel so a
	// concurrent broadcast either sees it (and sent before we hold the lock)
	// or doesn't (and skips it). All under the lock => no send-on-closed race.
	delete(members, m)
	if len(members) == 0 {
		delete(h.channels, m.channel)
	} else {
		broadcastLocked(members, msg)
	}

	// Safe to close now: the member is no longer reachable by any broadcaster,
	// which can only run while holding h.mu, which we hold.
	close(m.recv)
	h.mu.Unlock()
}

// broadcastLocked sends msg to every member in members with a non-blocking
// send (dropping for full receivers). The caller must hold h.mu.
func broadcastLocked(members map[*Member]struct{}, msg Message) {
	for other := range members {
		select {
		case other.recv <- msg:
		default:
			// Full buffer; drop for this member.
		}
	}
}
