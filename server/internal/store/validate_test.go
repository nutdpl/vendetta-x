package store

import "testing"

func TestValidateHandle(t *testing.T) {
	good := []string{"phiber", "ac1d", "Zero-Cool", "a.b", "[crew]"}
	for _, h := range good {
		if err := ValidateHandle(h); err != nil {
			t.Errorf("ValidateHandle(%q) = %v, want nil", h, err)
		}
	}
	bad := []string{
		"",                            // empty
		"x",                           // too short
		"this-handle-is-way-too-long", // > 20
		"has space",                   // space
		"a/b",                         // slash
		"a\\b",                        // backslash
		"esc\x1bhere",                 // control byte
		"All", "sysop", "ADMIN",       // reserved (case-insensitive)
	}
	for _, h := range bad {
		if err := ValidateHandle(h); err == nil {
			t.Errorf("ValidateHandle(%q) = nil, want error", h)
		}
	}
}
