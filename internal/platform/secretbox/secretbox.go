// Package secretbox encrypts small secrets at rest, such as payment provider
// credentials, with AES-256-GCM. The key comes from the operator
// (SERVLY_SECRET_KEY) and never from the database.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
)

// version prefixes every sealed value, so a later key or algorithm can be told apart.
const version = "v1."

var (
	ErrKey    = errors.New("secret key must be 32 bytes, base64-encoded (openssl rand -base64 32)")
	ErrSealed = errors.New("sealed value is damaged or was sealed with another key")
)

// Box seals and opens values with one key.
type Box struct{ aead cipher.AEAD }

// Parse builds a Box from a base64-encoded 32-byte key.
func Parse(b64 string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		key, err = base64.RawURLEncoding.DecodeString(strings.TrimSpace(b64))
	}
	if err != nil || len(key) != 32 {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plain with a fresh random nonce.
func (b *Box) Seal(plain []byte) string {
	nonce := make([]byte, b.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return version + base64.RawURLEncoding.EncodeToString(b.aead.Seal(nonce, nonce, plain, nil))
}

// Open decrypts a value from Seal; tampering or another key fails with ErrSealed.
func (b *Box) Open(sealed string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(sealed, version))
	if !strings.HasPrefix(sealed, version) || err != nil || len(raw) < b.aead.NonceSize() {
		return nil, ErrSealed
	}
	plain, err := b.aead.Open(nil, raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():], nil)
	if err != nil {
		return nil, ErrSealed
	}
	return plain, nil
}
