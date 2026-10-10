// Package ids generates internal identifiers and public tokens.
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"strings"

	"github.com/google/uuid"
)

// New returns a new internal identifier.
// UUIDv7, so ids sort roughly by creation time and index well.
func New() uuid.UUID { return uuid.Must(uuid.NewV7()) }

var tokenEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// PublicTokenLen is the length of a token from PublicToken.
const PublicTokenLen = 26

// PublicToken returns an unguessable token for public URLs.
// (QR codes, order status links): 128 random bits as 26 lowercase base32 characters.
func PublicToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return strings.ToLower(tokenEncoding.EncodeToString(b))
}

// IsPublicToken reports whether s has the shape of a PublicToken.
// Use it to reject malformed input before a database lookup.
func IsPublicToken(s string) bool {
	if len(s) != PublicTokenLen {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
}
