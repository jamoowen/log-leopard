package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"sync"
	"time"
)

const CookieName = "logleopard_session"

var ErrInvalidPairing = errors.New("pairing token is invalid or expired")

type Manager struct {
	mu       sync.Mutex
	pairing  []byte
	pairBy   time.Time
	sessions map[string]time.Time
	now      func() time.Time
	ttl      time.Duration
}

func NewManager(pairTTL, sessionTTL time.Duration) (*Manager, string) {
	pairing, token := randomToken()
	now := time.Now
	return &Manager{
		pairing:  pairing,
		pairBy:   now().Add(pairTTL),
		sessions: make(map[string]time.Time),
		now:      now,
		ttl:      sessionTTL,
	}, token
}

func (m *Manager) Exchange(token string) (string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pairing == nil || !m.now().Before(m.pairBy) || subtle.ConstantTimeCompare([]byte(token), m.pairing) != 1 {
		return "", time.Time{}, ErrInvalidPairing
	}
	// Invalidate before generating the session so concurrent exchanges cannot replay the token.
	m.pairing = nil
	_, session := randomToken()
	expires := m.now().Add(m.ttl)
	m.sessions[session] = expires
	return session, expires, nil
}

func (m *Manager) Valid(session string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	expires, ok := m.sessions[session]
	if !ok || !m.now().Before(expires) {
		delete(m.sessions, session)
		return false
	}
	return true
}

func (m *Manager) Logout(session string) {
	m.mu.Lock()
	delete(m.sessions, session)
	m.mu.Unlock()
}

func randomToken() ([]byte, string) {
	token := rand.Text()
	return []byte(token), token
}
