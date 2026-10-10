package secretbox

import (
	"errors"
	"testing"
)

const key = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 bytes

func TestSealOpen(t *testing.T) {
	b, err := Parse(key)
	if err != nil {
		t.Fatal(err)
	}
	s1, s2 := b.Seal([]byte("sk_test_123")), b.Seal([]byte("sk_test_123"))
	if s1 == s2 {
		t.Fatal("same ciphertext twice: nonce not random")
	}
	got, err := b.Open(s1)
	if err != nil || string(got) != "sk_test_123" {
		t.Fatalf("open: %q %v", got, err)
	}
	tampered := s1[:len(s1)-2] + "AA"
	if _, err := b.Open(tampered); !errors.Is(err, ErrSealed) {
		t.Fatalf("tampered: %v", err)
	}
	other, _ := Parse("ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA=")
	if _, err := other.Open(s1); !errors.Is(err, ErrSealed) {
		t.Fatalf("other key: %v", err)
	}
	for _, bad := range []string{"", "short", "bm90IDMyIGJ5dGVz"} {
		if _, err := Parse(bad); !errors.Is(err, ErrKey) {
			t.Errorf("Parse(%q): %v", bad, err)
		}
	}
}
