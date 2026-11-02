// Package secret generates and hashes high-entropy bearer secrets (session, email and caller tokens).
package secret

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// New returns a random URL-safe secret and its SHA-256 hash for storage.
// Secrets carry 256 bits of entropy, so a fast hash is sufficient (no need for a password KDF).
func New() (raw string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate secret: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, Hash(raw), nil
}

// Hash returns the storage hash of a raw secret.
func Hash(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// MaxLen bounds accepted secret length before hashing (encoded secrets are 43 characters).
const MaxLen = 128
