// Package auth handles password hashing for the board. The store only ever
// holds the bcrypt hash; plaintext lives no longer than a single login or
// signup turn. Keeping this separate keeps the store dependency-free.
package auth

import "golang.org/x/crypto/bcrypt"

// Hash returns the bcrypt hash of a plaintext password.
func Hash(plain string) (string, error) {
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
