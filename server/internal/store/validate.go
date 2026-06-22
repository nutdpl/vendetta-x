package store

import (
	"errors"
	"strings"
)

// reservedHandles are names that would be confusing or impersonating if a caller
// could register them (they collide with the "All" recipient, the sysop, system
// accounts, or routing words).
var reservedHandles = map[string]bool{
	"all": true, "sysop": true, "admin": true, "administrator": true,
	"root": true, "guest": true, "anonymous": true, "system": true,
	"server": true, "board": true, "everyone": true, "nobody": true,
	"new": true, "none": true,
}

// ValidateHandle reports whether a proposed user handle is acceptable. Handles
// are echoed onto other callers' terminals, used in URLs (/users/{handle}), and
// stand in as message authors, so they must be short, printable, space-free,
// and not a reserved word. It does not check uniqueness (the caller does that).
func ValidateHandle(handle string) error {
	h := strings.TrimSpace(handle)
	if len(h) < 2 {
		return errors.New("handle must be at least 2 characters")
	}
	if len(h) > 20 {
		return errors.New("handle must be 20 characters or fewer")
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if c <= 0x20 || c >= 0x7f {
			return errors.New("handle may only use printable characters, no spaces")
		}
		if c == '/' || c == '\\' {
			return errors.New("handle may not contain slashes")
		}
	}
	if reservedHandles[strings.ToLower(h)] {
		return errors.New("that handle is reserved -- pick another")
	}
	return nil
}
