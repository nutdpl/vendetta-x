package throttle

import (
	"testing"
	"time"
)

func TestBlockAfterMax(t *testing.T) {
	th := New(3, time.Minute)
	if th.Blocked("1.2.3.4") {
		t.Fatal("blocked before any failures")
	}
	th.Fail("1.2.3.4")
	th.Fail("1.2.3.4")
	if th.Blocked("1.2.3.4") {
		t.Fatal("blocked at 2 < 3 failures")
	}
	th.Fail("1.2.3.4")
	if !th.Blocked("1.2.3.4") {
		t.Fatal("not blocked at the limit")
	}
	// A different key is unaffected.
	if th.Blocked("5.6.7.8") {
		t.Fatal("unrelated key blocked")
	}
}

func TestResetClears(t *testing.T) {
	th := New(2, time.Minute)
	th.Fail("k")
	th.Fail("k")
	if !th.Blocked("k") {
		t.Fatal("should be blocked")
	}
	th.Reset("k")
	if th.Blocked("k") {
		t.Fatal("reset did not clear")
	}
}

func TestWindowExpiry(t *testing.T) {
	th := New(2, 20*time.Millisecond)
	th.Fail("k")
	th.Fail("k")
	if !th.Blocked("k") {
		t.Fatal("should be blocked")
	}
	time.Sleep(35 * time.Millisecond)
	if th.Blocked("k") {
		t.Fatal("failures should have aged out of the window")
	}
}
