package auth

import "testing"

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"empty", "", false},
		{"too short", "short7!", false}, // 7 chars
		{"min length", "eightch8", true},
		{"long ok", "a-reasonably-long-passphrase", true},
		{"at limit", string(make([]byte, MaxPasswordLen)), true},
		{"over limit", string(make([]byte, MaxPasswordLen+1)), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidatePassword(c.pw) == nil; got != c.ok {
				t.Fatalf("ValidatePassword(%d bytes) ok=%v, want %v", len(c.pw), got, c.ok)
			}
		})
	}
}

func TestHashEnforcesPolicy(t *testing.T) {
	// A too-short password must never produce a hash.
	if _, err := Hash("short7!"); err == nil {
		t.Fatal("Hash accepted a 7-char password; policy not enforced")
	}
	// An over-72-byte password must be rejected, not silently truncated.
	if _, err := Hash(string(make([]byte, MaxPasswordLen+1))); err == nil {
		t.Fatal("Hash accepted a >72-byte password; would silently truncate")
	}
	// A valid password round-trips through Verify.
	h, err := Hash("eightchars")
	if err != nil {
		t.Fatalf("Hash valid password: %v", err)
	}
	if !Verify(h, "eightchars") {
		t.Fatal("Verify rejected the password it just hashed")
	}
	if Verify(h, "wrongpassword") {
		t.Fatal("Verify accepted a wrong password")
	}
	if Verify("", "anything") {
		t.Fatal("Verify matched against an empty (unset) hash")
	}
}
