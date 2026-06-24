// Package auth handles password hashing for the board. The store only ever
// holds the bcrypt hash; plaintext lives no longer than a single login or
// signup turn. Keeping this separate keeps the store dependency-free.
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// Password policy. The upper bound is bcrypt's hard limit: it ignores bytes past
// 72, so we reject longer inputs outright rather than silently truncating them
// (which would give the caller a false sense of strength).
const (
	MinPasswordLen = 8
	MaxPasswordLen = 72
)

// ValidatePassword enforces the board's password policy. Call it before Hash at
// every set-password site (telnet, ssh, web) so the rule is uniform.
func ValidatePassword(plain string) error {
	if len(plain) < MinPasswordLen {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	}
	if len(plain) > MaxPasswordLen {
		return fmt.Errorf("password must be at most %d characters", MaxPasswordLen)
	}
	return nil
}

// Hash returns the bcrypt hash of a plaintext password. It enforces the policy
// first, so a too-short or over-72-byte password can never be hashed.
func Hash(plain string) (string, error) {
	if err := ValidatePassword(plain); err != nil {
		return "", err
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Verify reports whether plain matches the stored bcrypt hash. An empty stored
// hash means "no password set" and never matches.
func Verify(hash, plain string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
