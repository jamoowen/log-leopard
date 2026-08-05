package gcp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jamoowen/log-leopard/internal/provider"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Cloud Run services.list requires cloud-platform; LogLeopard uses only read-only calls and viewer IAM roles.
const oauthScope = "https://www.googleapis.com/auth/cloud-platform"

type tokenStore interface {
	Load(context.Context) (storedRefreshToken, error)
	Save(context.Context, storedRefreshToken) error
}

type fileTokenStore struct{ path string }

type storedRefreshToken struct {
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func (s fileTokenStore) Load(context.Context) (storedRefreshToken, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return storedRefreshToken{}, nil
	}
	if err != nil {
		return storedRefreshToken{}, err
	}
	var token storedRefreshToken
	if err := json.Unmarshal(data, &token); err != nil {
		return storedRefreshToken{}, err
	}
	return token, nil
}

func (s fileTokenStore) Save(_ context.Context, token storedRefreshToken) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".google-oauth-token-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

type oauthManager struct {
	clientID, clientSecret string
	store                  tokenStore
	now                    func() time.Time
	random                 func([]byte) (int, error)
	mu                     sync.Mutex
	pending                map[string]oauthPending
	newTokenSource         func(context.Context, string) oauth2.TokenSource
	exchange               func(context.Context, oauth2.Config, string, string) (*oauth2.Token, error)
	validated              oauthValidated
}

type oauthPending struct {
	verifier string
	expires  time.Time
}

type oauthValidated struct {
	refreshToken string
	source       oauth2.TokenSource
	expires      time.Time
}

func newOAuthManager(clientID, clientSecret string, store tokenStore) *oauthManager {
	m := &oauthManager{clientID: clientID, clientSecret: clientSecret, store: store, now: time.Now, random: rand.Read, pending: make(map[string]oauthPending)}
	m.newTokenSource = func(ctx context.Context, refresh string) oauth2.TokenSource {
		config := m.config("")
		return config.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	}
	m.exchange = func(ctx context.Context, config oauth2.Config, code, verifier string) (*oauth2.Token, error) {
		return config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	}
	return m
}

func (m *oauthManager) config(redirectURI string) oauth2.Config {
	return oauth2.Config{ClientID: m.clientID, ClientSecret: m.clientSecret, RedirectURL: redirectURI, Endpoint: google.Endpoint, Scopes: []string{oauthScope}}
}

func (m *oauthManager) configured() bool { return m.clientID != "" && m.clientSecret != "" }

func (m *oauthManager) status(ctx context.Context) provider.AuthStatus {
	if !m.configured() {
		return provider.AuthStatus{State: provider.AuthUnconfigured, Message: "Google sign-in is not configured. Application Default Credentials can still be used."}
	}
	if m.store == nil {
		return provider.AuthStatus{State: provider.AuthFailed, Message: "Google sign-in storage is unavailable. Configure Application Default Credentials instead."}
	}
	token, err := m.store.Load(ctx)
	if err != nil {
		return provider.AuthStatus{State: provider.AuthFailed, Message: "Stored Google sign-in credentials could not be read. Sign in again."}
	}
	if token.RefreshToken == "" {
		return provider.AuthStatus{State: provider.AuthNeedsAuth, Message: "Sign in with Google or configure Application Default Credentials."}
	}
	if token.Scope != oauthScope {
		return provider.AuthStatus{State: provider.AuthFailed, Message: "Stored Google sign-in credentials need to be updated. Sign in again."}
	}
	_, usable := m.tokenSource(ctx)
	if !usable {
		return provider.AuthStatus{State: provider.AuthFailed, Message: "Stored Google sign-in credentials are unavailable. Sign in again or configure Application Default Credentials."}
	}
	return provider.AuthStatus{State: provider.AuthAvailable, Message: "Google sign-in credentials are available."}
}

func (m *oauthManager) start(ctx context.Context, redirectURI string) provider.AuthStart {
	status := m.status(ctx)
	if !m.configured() {
		return provider.AuthStart{AuthStatus: status}
	}
	if m.store == nil {
		return provider.AuthStart{AuthStatus: status}
	}
	if _, err := m.store.Load(ctx); err != nil {
		return provider.AuthStart{AuthStatus: provider.AuthStatus{State: provider.AuthFailed, Message: "Google sign-in storage is unavailable. Configure Application Default Credentials instead."}}
	}
	verifier := oauth2.GenerateVerifier()
	state, err := m.randomState()
	if err != nil {
		return provider.AuthStart{AuthStatus: provider.AuthStatus{State: provider.AuthFailed, Message: "Google sign-in could not be started. Try again."}}
	}
	m.mu.Lock()
	for key, pending := range m.pending {
		if !m.now().Before(pending.expires) {
			delete(m.pending, key)
		}
	}
	m.pending[state] = oauthPending{verifier: verifier, expires: m.now().Add(10 * time.Minute)}
	m.mu.Unlock()
	config := m.config(redirectURI)
	options := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier)}
	if status.State != provider.AuthAvailable {
		options = append(options, oauth2.ApprovalForce)
	}
	url := config.AuthCodeURL(state, options...)
	return provider.AuthStart{AuthStatus: provider.AuthStatus{State: provider.AuthPending, Message: "Continue signing in with Google."}, AuthorizationURL: url}
}

func (m *oauthManager) callback(ctx context.Context, request provider.GoogleAuthCallback) error {
	if m.store == nil {
		return errors.New("OAuth credential storage is unavailable")
	}
	m.mu.Lock()
	pending, found := m.pending[request.State]
	if found {
		delete(m.pending, request.State)
	}
	m.mu.Unlock()
	if !found || !m.now().Before(pending.expires) {
		return errors.New("invalid OAuth state")
	}
	if request.Error != "" || request.Code == "" {
		return errors.New("google authorization was not completed")
	}
	config := m.config(request.RedirectURI)
	token, err := m.exchange(ctx, config, request.Code, pending.verifier)
	if err != nil {
		return fmt.Errorf("exchange OAuth code: %w", err)
	}
	if token.RefreshToken == "" {
		return errors.New("google did not return a refresh token")
	}
	if !hasOAuthScope(token) {
		return errors.New("google sign-in did not grant the required permissions")
	}
	if err := m.store.Save(ctx, storedRefreshToken{RefreshToken: token.RefreshToken, Scope: oauthScope}); err != nil {
		return fmt.Errorf("store OAuth refresh token: %w", err)
	}
	m.mu.Lock()
	m.validated = oauthValidated{}
	m.mu.Unlock()
	return nil
}

func (m *oauthManager) tokenSource(ctx context.Context) (oauth2.TokenSource, bool) {
	if !m.configured() || m.store == nil {
		return nil, false
	}
	token, err := m.store.Load(ctx)
	if err != nil || token.RefreshToken == "" || token.Scope != oauthScope {
		return nil, false
	}
	m.mu.Lock()
	if m.validated.refreshToken == token.RefreshToken && m.now().Before(m.validated.expires) {
		source := m.validated.source
		m.mu.Unlock()
		return source, true
	}
	m.mu.Unlock()
	source := m.newTokenSource(ctx, token.RefreshToken)
	accessToken, err := source.Token()
	if err != nil {
		return nil, false
	}
	expires := accessToken.Expiry
	now := m.now()
	if expires.IsZero() {
		expires = now.Add(time.Minute)
	} else {
		expires = expires.Add(-time.Minute)
		if !expires.After(now) {
			expires = now
		}
	}
	m.mu.Lock()
	m.validated = oauthValidated{refreshToken: token.RefreshToken, source: source, expires: expires}
	m.mu.Unlock()
	return source, true
}

func hasOAuthScope(token *oauth2.Token) bool {
	grantedScope := token.Extra("scope")
	if grantedScope == nil {
		return true
	}
	granted, ok := grantedScope.(string)
	if !ok {
		return false
	}
	return slices.Contains(strings.Fields(granted), oauthScope)
}

func (m *oauthManager) randomState() (string, error) {
	data := make([]byte, 32)
	if _, err := m.random(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
