package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestPutOpenDelete(t *testing.T) {
	root := t.TempDir()
	l := NewLocal(root)
	key := Key(uuid.New(), uuid.New())
	if err := l.Put(key, []byte("image bytes")); err != nil {
		t.Fatal(err)
	}
	f, err := l.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "image bytes" {
		t.Fatalf("read %q", b)
	}
	if err := l.Delete(key); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete(key); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := l.Open(key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open deleted: %v", err)
	}
}

func TestKeysCannotEscapeRoot(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(filepath.Dir(root), "secret")
	_ = os.WriteFile(secret, []byte("x"), 0o600)
	l := NewLocal(root)
	for _, key := range []string{"../secret", "/etc/passwd", "a/../../secret", uuid.NewString() + "/../../secret", "", uuid.NewString()} {
		if _, err := l.Open(key); !errors.Is(err, ErrBadKey) {
			t.Errorf("Open(%q) = %v", key, err)
		}
		if err := l.Put(key, nil); !errors.Is(err, ErrBadKey) {
			t.Errorf("Put(%q) = %v", key, err)
		}
	}
}
