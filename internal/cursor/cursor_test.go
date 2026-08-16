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

func TestEncodeWithExpiryReturnsExactSignedExpiry(t *testing.T) {
	s := New(time.Minute)
	now := time.Date(2026, 8, 16, 12, 0, 0, 750_000_000, time.UTC)
	s.now = func() time.Time { return now }
	encoded, expires, err := s.EncodeWithExpiry("provider-token", Fingerprint("query"))
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(time.Minute).Truncate(time.Second); !expires.Equal(want) {
		t.Fatalf("expiry = %s, want %s", expires, want)
	}
	now = expires.Add(-time.Nanosecond)
	if _, err := s.Decode(encoded, Fingerprint("query")); err != nil {
		t.Fatalf("cursor expired before returned expiry: %v", err)
	}
	now = expires
	if _, err := s.Decode(encoded, Fingerprint("query")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cursor remained valid at returned expiry: %v", err)
	}
}
