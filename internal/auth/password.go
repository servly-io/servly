// Package auth holds staff authentication: password hashing and
// server-side sessions. Contract: docs/docs/security.md#authentication.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"

	"github.com/servly-io/servly/internal/apperr"
)

// Argon2id parameters: the OWASP baseline (19 MiB, 2 passes, 1 lane). They
// are stored in every hash, so raising them later keeps old hashes valid.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

const (
	MinPasswordLen = 10
	MaxPasswordLen = 256 // bytes; bounds the hashing cost of a request
)

var (
	ErrPasswordTooShort = apperr.Invalid("password_too_short", fmt.Sprintf("password must be at least %d characters", MinPasswordLen))
	ErrPasswordTooLong  = apperr.Invalid("password_too_long", fmt.Sprintf("password must be at most %d bytes", MaxPasswordLen))
	errMalformedHash    = errors.New("malformed password hash")
)

// ValidatePassword checks the password policy.
func ValidatePassword(pw string) error {
	if len(pw) > MaxPasswordLen {
		return ErrPasswordTooLong
	}
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	return nil
}

// HashPassword returns an encoded Argon2id hash:
// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<key>
func HashPassword(pw string) (string, error) {
	if err := ValidatePassword(pw); err != nil {
		return "", err
	}
	salt := make([]byte, saltLen)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches the encoded hash. A malformed
// hash never matches.
func VerifyPassword(encoded, pw string) bool {
	if len(pw) > MaxPasswordLen {
		return false
	}
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, p.time, p.memory, p.threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}

// dummyHash is verified against when a login names an unknown user, so the
// response takes as long as for a known user with a wrong password.
var dummyHash, _ = HashPassword("servly-timing-equalizer")

// VerifyDummy spends the time of one password verification.
func VerifyDummy(pw string) { VerifyPassword(dummyHash, pw) }

type params struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return params{}, nil, nil, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return params{}, nil, nil, errMalformedHash
	}
	var p params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil ||
		p.memory == 0 || p.memory > 1<<20 || p.time == 0 || p.time > 10 || p.threads == 0 {
		return params{}, nil, nil, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return params{}, nil, nil, errMalformedHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < 16 {
		return params{}, nil, nil, errMalformedHash
	}
	return p, salt, key, nil
}
