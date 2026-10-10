package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash format: %s", h)
	}
	if !VerifyPassword(h, "correct horse battery") {
		t.Fatal("right password rejected")
	}
	if VerifyPassword(h, "correct horse batterY") {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse battery")
	if h == h2 {
		t.Fatal("same salt twice")
	}
}

func TestPolicy(t *testing.T) {
	if _, err := HashPassword("short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("short: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("x", MaxPasswordLen+1)); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("long: %v", err)
	}
	if err := ValidatePassword("äöüäöüäöüä"); err != nil {
		t.Fatalf("10 runes rejected: %v", err)
	}
}

func TestMalformedHashNeverMatches(t *testing.T) {
	for _, h := range []string{"", "plain", "$argon2id$v=19$m=0,t=2,p=1$c2FsdHNhbHQ$a2V5a2V5a2V5a2V5a2V5", "$bcrypt$x$y$z$w"} {
		if VerifyPassword(h, "") || VerifyPassword(h, "anything at all") {
			t.Errorf("malformed hash %q matched", h)
		}
	}
}
