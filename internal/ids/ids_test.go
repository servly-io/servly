package ids

import "testing"

func TestPublicToken(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		tok := PublicToken()
		if !IsPublicToken(tok) {
			t.Fatalf("PublicToken() = %q, not a valid token", tok)
		}
		if seen[tok] {
			t.Fatalf("duplicate token %q", tok)
		}
		seen[tok] = true
	}
}

func TestIsPublicTokenRejects(t *testing.T) {
	for _, s := range []string{"", "short", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxy1", "../../../../../etc/passwd!"} {
		if IsPublicToken(s) {
			t.Errorf("IsPublicToken(%q) = true", s)
		}
	}
}

func TestNewIsVersion7(t *testing.T) {
	if v := New().Version(); v != 7 {
		t.Fatalf("version = %d", v)
	}
}
