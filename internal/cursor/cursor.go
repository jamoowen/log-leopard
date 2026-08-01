package cursor

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalid = errors.New("cursor is invalid or expired")

type Signer struct {
	key []byte
	now func() time.Time
	ttl time.Duration
}

type payload struct {
	Token       string `json:"token"`
	Fingerprint string `json:"fingerprint"`
	Expires     int64  `json:"expires"`
}

func New(ttl time.Duration) *Signer {
	return &Signer{key: []byte(rand.Text()), now: time.Now, ttl: ttl}
}

func Fingerprint(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte{0})
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Signer) Encode(token, fingerprint string) (string, error) {
	b, err := json.Marshal(payload{Token: token, Fingerprint: fingerprint, Expires: s.ExpiresAt().Unix()})
	if err != nil {
		return "", err
	}
	sig := s.sign(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *Signer) ExpiresAt() time.Time {
	return s.now().Add(s.ttl)
}

func (s *Signer) Decode(value, fingerprint string) (string, error) {
	encodedPayload, encodedSignature, ok := strings.Cut(value, ".")
	if !ok {
		return "", ErrInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return "", ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil || !hmac.Equal(signature, s.sign(b)) {
		return "", ErrInvalid
	}
	var p payload
	if err := json.Unmarshal(b, &p); err != nil || p.Fingerprint != fingerprint || p.Token == "" || !s.now().Before(time.Unix(p.Expires, 0)) {
		return "", ErrInvalid
	}
	return p.Token, nil
}

func (s *Signer) sign(payload []byte) []byte {
	h := hmac.New(sha256.New, s.key)
	h.Write(payload)
	return h.Sum(nil)
}
