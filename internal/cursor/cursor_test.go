package cursor

import (
	"errors"
	"testing"
	"time"
)

func TestCursorBindingTamperingAndExpiry(t *testing.T) {
	s := New(time.Minute)
	now := time.Now()
	s.now = func() time.Time { return now }
	c, err := s.Encode("provider-secret", Fingerprint("profile", "query"))
	if err != nil {
		t.Fatal(err)
	}
	if token, err := s.Decode(c, Fingerprint("profile", "query")); err != nil || token != "provider-secret" {
		t.Fatalf("decode: %q %v", token, err)
	}
	for _, invalid := range []struct{ cursor, fingerprint string }{
		{c + "x", Fingerprint("profile", "query")}, {c, Fingerprint("other")},
	} {
		if _, err := s.Decode(invalid.cursor, invalid.fingerprint); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid cursor got %v", err)
		}
	}
	now = now.Add(time.Minute)
	if _, err := s.Decode(c, Fingerprint("profile", "query")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expired cursor got %v", err)
	}
}
