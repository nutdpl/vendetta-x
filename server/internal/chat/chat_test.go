package chat

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// recvWithin reads one message from m's recv channel, failing if none arrives
// within the timeout.
func recvWithin(t *testing.T, m *Member, d time.Duration) (Message, bool) {
	t.Helper()
	select {
	case msg, ok := <-m.Recv():
		return msg, ok
	case <-time.After(d):
		t.Fatalf("timed out waiting for message for %q", m.handle)
		return Message{}, false
	}
}

func TestSayReachesOthersNotSelf(t *testing.T) {
	h := NewHub()
	a := h.Join("room", "alice")
	b := h.Join("room", "bob")

	// Drain the join notice that alice received when bob joined.
	if msg, ok := recvWithin(t, a, time.Second); !ok || !msg.Sys {
		t.Fatalf("expected join notice for alice, got %+v ok=%v", msg, ok)
	}

	a.Say("hello")

	msg, ok := recvWithin(t, b, time.Second)
	if !ok {
		t.Fatal("bob's recv closed unexpectedly")
	}
	if msg.From != "alice" || msg.Text != "hello" || msg.Sys {
		t.Fatalf("bob got wrong message: %+v", msg)
	}

	// alice must NOT receive her own line.
	select {
	case extra := <-a.Recv():
		t.Fatalf("alice received her own line: %+v", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestJoinNoticeReachesExisting(t *testing.T) {
	h := NewHub()
	a := h.Join("room", "alice")

	h.Join("room", "bob")

	msg, ok := recvWithin(t, a, time.Second)
	if !ok {
		t.Fatal("alice recv closed")
	}
	if !msg.Sys || msg.From != "" || msg.Text != "bob has joined" {
		t.Fatalf("unexpected join notice: %+v", msg)
	}
}

func TestWhoSorted(t *testing.T) {
	h := NewHub()
	h.Join("room", "charlie")
	h.Join("room", "alice")
	h.Join("room", "bob")

	got := h.Who("room")
	want := []string{"alice", "bob", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("Who returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Who returned %v, want %v", got, want)
		}
	}

	if got := h.Who("nonexistent"); len(got) != 0 {
		t.Fatalf("Who on empty channel returned %v", got)
	}
}

func TestLeaveNoticeAndCloses(t *testing.T) {
	h := NewHub()
	a := h.Join("room", "alice")
	b := h.Join("room", "bob")

	// Drain alice's join notice for bob.
	recvWithin(t, a, time.Second)

	b.Leave()

	// alice should get the leave notice.
	msg, ok := recvWithin(t, a, time.Second)
	if !ok {
		t.Fatal("alice recv closed")
	}
	if !msg.Sys || msg.Text != "bob has left" {
		t.Fatalf("unexpected leave notice: %+v", msg)
	}

	// bob's channel must be closed; after draining we get ok=false.
	for {
		_, ok := <-b.Recv()
		if !ok {
			break
		}
	}

	// Channel still has alice.
	if got := h.Who("room"); len(got) != 1 || got[0] != "alice" {
		t.Fatalf("after bob left, Who = %v", got)
	}

	// alice leaves; channel becomes empty and should be deleted.
	a.Leave()
	if got := h.Who("room"); len(got) != 0 {
		t.Fatalf("after all left, Who = %v", got)
	}
}

func TestLeaveIdempotent(t *testing.T) {
	h := NewHub()
	a := h.Join("room", "alice")

	a.Leave()
	// Second Leave must not panic or double-close.
	a.Leave()

	// recv is closed; reading returns ok=false.
	if _, ok := <-a.Recv(); ok {
		t.Fatal("expected closed recv after Leave")
	}
}

func TestSlowReaderDropsRatherThanBlocks(t *testing.T) {
	h := NewHub()
	speaker := h.Join("room", "speaker")
	slow := h.Join("room", "slow") // never reads

	// Drain speaker's join notice for slow.
	recvWithin(t, speaker, time.Second)

	// Fill slow's buffer well beyond capacity. Each Say must return promptly.
	done := make(chan struct{})
	go func() {
		for i := 0; i < recvCap*4; i++ {
			speaker.Say(fmt.Sprintf("line %d", i))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Say blocked on a full slow reader (deadlock)")
	}

	// slow's buffer holds at most recvCap messages; extras were dropped.
	if n := len(slow.Recv()); n > recvCap {
		t.Fatalf("slow buffer holds %d > cap %d; messages not dropped", n, recvCap)
	}
}

func TestConcurrentJoinSayLeave(t *testing.T) {
	h := NewHub()
	const goroutines = 50
	const channels = 5

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ch := fmt.Sprintf("room-%d", id%channels)
			handle := fmt.Sprintf("user-%d", id)
			m := h.Join(ch, handle)

			// Drain in the background so broadcasts have somewhere to go and
			// to exercise concurrent reads against Leave's close.
			drainDone := make(chan struct{})
			go func() {
				for range m.Recv() {
				}
				close(drainDone)
			}()

			for j := 0; j < 20; j++ {
				m.Say(fmt.Sprintf("msg %d from %s", j, handle))
				_ = h.Who(ch)
			}

			m.Leave()
			m.Leave() // idempotent under concurrency
			<-drainDone
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent test timed out")
	}

	// All channels should be empty/deleted.
	for c := 0; c < channels; c++ {
		if got := h.Who(fmt.Sprintf("room-%d", c)); len(got) != 0 {
			t.Fatalf("channel room-%d not empty: %v", c, got)
		}
	}
}
