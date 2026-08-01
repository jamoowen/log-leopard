package auth

import (
	"errors"
	"testing"
	"time"
)

func TestPairingIsSingleUseAndSessionsExpire(t *testing.T) {
	m, pairing := NewManager(time.Minute, time.Hour)
	now := time.Now()
	m.now = func() time.Time { return now }
	session, _, err := m.Exchange(pairing)
	if err != nil || !m.Valid(session) {
		t.Fatalf("exchange failed: %v", err)
	}
	if _, _, err := m.Exchange(pairing); !errors.Is(err, ErrInvalidPairing) {
		t.Fatalf("pairing replay got %v", err)
	}
	now = now.Add(time.Hour)
	if m.Valid(session) {
		t.Fatal("session remained valid at expiry")
	}
}
