package gcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jamoowen/log-leopard/internal/provider"
	"golang.org/x/oauth2"
)

type memoryTokenStore struct{ token storedRefreshToken }

func (s *memoryTokenStore) Load(context.Context) (storedRefreshToken, error) { return s.token, nil }
func (s *memoryTokenStore) Save(_ context.Context, token storedRefreshToken) error {
	s.token = token
	return nil
}

func TestOAuthStartUsesPKCEAndSingleUseState(t *testing.T) {
	manager := newOAuthManager("client-id", "client-secret", &memoryTokenStore{})
	start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback")
	if start.State != provider.AuthPending || start.AuthorizationURL == "" {
		t.Fatalf("start = %#v", start)
	}
	parsed, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("state") == "" || query.Get("code_challenge") == "" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("missing PKCE/state in %q", start.AuthorizationURL)
	}
	if query.Get("access_type") != "offline" || query.Get("prompt") != "consent" {
		t.Fatalf("authorization options = %q", start.AuthorizationURL)
	}
	if scope := query.Get("scope"); scope != oauthScope {
		t.Fatalf("scope = %q, want %q", scope, oauthScope)
	}
	if err := manager.callback(context.Background(), provider.GoogleAuthCallback{RedirectURI: "http://127.0.0.1:1234/api/v1/auth/google/callback", State: query.Get("state"), Error: "access_denied"}); err == nil {
		t.Fatal("callback accepted denied authorization")
	}
	if err := manager.callback(context.Background(), provider.GoogleAuthCallback{RedirectURI: "http://127.0.0.1:1234/api/v1/auth/google/callback", State: query.Get("state"), Error: "access_denied"}); err == nil {
		t.Fatal("callback state was reusable")
	}
}

func TestOAuthStartDoesNotForceApprovalWhenRefreshTokenExists(t *testing.T) {
	manager := newOAuthManager("client-id", "client-secret", &memoryTokenStore{token: storedRefreshToken{RefreshToken: "stored-refresh-token", Scope: oauthScope}})
	manager.newTokenSource = func(context.Context, string) oauth2.TokenSource {
		return tokenSourceFunc(func() (*oauth2.Token, error) {
			return &oauth2.Token{AccessToken: "access-token", Expiry: time.Now().Add(time.Hour)}, nil
		})
	}
	if status := manager.status(context.Background()); status.State != provider.AuthAvailable {
		t.Fatalf("status = %#v, want available", status)
	}
	start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback")
	parsed, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if prompt := parsed.Query().Get("prompt"); prompt != "" {
		t.Fatalf("prompt = %q, want empty", prompt)
	}
}

func TestOAuthLegacyOrWrongScopeRequiresConsent(t *testing.T) {
	tests := []struct {
		name  string
		scope string
	}{
		{name: "legacy", scope: ""},
		{name: "wrong scope", scope: "https://www.googleapis.com/auth/cloud-platform.read-only"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := newOAuthManager("client-id", "client-secret", &memoryTokenStore{token: storedRefreshToken{RefreshToken: "stored-refresh-token", Scope: test.scope}})
			manager.newTokenSource = func(context.Context, string) oauth2.TokenSource {
				t.Fatal("legacy or wrong-scope token was used")
				return nil
			}
			if status := manager.status(context.Background()); status.State != provider.AuthFailed {
				t.Fatalf("status = %#v, want failed", status)
			}
			if _, ok := manager.tokenSource(context.Background()); ok {
				t.Fatal("legacy or wrong-scope token supplied a token source")
			}
			start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback")
			if prompt := mustQuery(t, start.AuthorizationURL).Get("prompt"); prompt != "consent" {
				t.Fatalf("prompt = %q, want consent", prompt)
			}
		})
	}
}

func TestFileTokenStoreLegacyRecordRequiresConsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "google-oauth-token.json")
	if err := os.WriteFile(path, []byte(`{"refresh_token":"legacy-refresh"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := fileTokenStore{path: path}
	stored, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stored.RefreshToken == "" || stored.Scope != "" {
		t.Fatalf("legacy record did not load with an empty scope")
	}
	manager := newOAuthManager("client-id", "client-secret", store)
	manager.newTokenSource = func(context.Context, string) oauth2.TokenSource {
		t.Fatal("legacy record was used as a token source")
		return nil
	}
	if status := manager.status(context.Background()); status.State != provider.AuthFailed {
		t.Fatalf("status = %#v, want failed", status)
	}
	start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback")
	if prompt := mustQuery(t, start.AuthorizationURL).Get("prompt"); prompt != "consent" {
		t.Fatalf("prompt = %q, want consent", prompt)
	}
}

func TestOAuthRequiresClientSecret(t *testing.T) {
	manager := newOAuthManager("client-id", "", &memoryTokenStore{})
	if status := manager.status(context.Background()); status.State != provider.AuthUnconfigured {
		t.Fatalf("status = %#v, want unconfigured", status)
	}
	if start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback"); start.AuthorizationURL != "" {
		t.Fatalf("start = %#v, want no authorization URL", start)
	}
}

func TestOAuthStatusIsSanitized(t *testing.T) {
	manager := newOAuthManager("", "", &memoryTokenStore{})
	status := manager.status(context.Background())
	if status.State != provider.AuthUnconfigured || strings.Contains(status.Message, "token") {
		t.Fatalf("status = %#v", status)
	}
}

func TestOAuthCallbackTokenExchangeRequest(t *testing.T) {
	const (
		clientID     = "test-client-id"
		clientSecret = "test-client-secret"
		callbackURI  = "http://127.0.0.1:1234/api/v1/auth/google/callback"
		code         = "test-authorization-code"
	)
	var form url.Values
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-access-token","refresh_token":"test-refresh-token","token_type":"Bearer","expires_in":3600,"scope":"https://www.googleapis.com/auth/cloud-platform"}`))
	}))
	defer tokenServer.Close()

	store := &memoryTokenStore{}
	manager := newOAuthManager(clientID, clientSecret, store)
	manager.exchange = func(ctx context.Context, config oauth2.Config, receivedCode, verifier string) (*oauth2.Token, error) {
		config.Endpoint.TokenURL = tokenServer.URL
		return config.Exchange(ctx, receivedCode, oauth2.VerifierOption(verifier))
	}
	start := manager.start(context.Background(), callbackURI)
	state := mustQuery(t, start.AuthorizationURL).Get("state")
	if err := manager.callback(context.Background(), provider.GoogleAuthCallback{RedirectURI: callbackURI, State: state, Code: code}); err != nil {
		t.Fatalf("callback() error = %v", err)
	}

	if got := form.Get("client_id"); got != clientID {
		t.Errorf("client_id = %q, want %q", got, clientID)
	}
	if got := form.Get("code"); got != code {
		t.Errorf("code = %q, want %q", got, code)
	}
	verifier := form.Get("code_verifier")
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("code_verifier length = %d, want 43..128", len(verifier))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9._~-]+$`).MatchString(verifier) {
		t.Errorf("code_verifier contains characters outside RFC 7636 unreserved set")
	}
	if got := form.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", got)
	}
	if got := form.Get("redirect_uri"); got != callbackURI {
		t.Errorf("redirect_uri = %q, want %q", got, callbackURI)
	}
	if got := form.Get("client_secret"); got != clientSecret {
		t.Errorf("client_secret = %q, want configured client secret", got)
	}
	if store.token.RefreshToken != "test-refresh-token" || store.token.Scope != oauthScope {
		t.Errorf("stored token = %#v, want refresh token and scope", store.token)
	}
}

func TestOAuthStartDoesNotReturnURLWhenStorageIsUnavailable(t *testing.T) {
	manager := newOAuthManager("client-id", "client-secret", nil)
	start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback")
	if start.AuthorizationURL != "" || start.State != provider.AuthFailed {
		t.Fatalf("start = %#v", start)
	}
}

func TestOAuthCallbackPersistsRefreshTokenAndClearsValidatedToken(t *testing.T) {
	store := &memoryTokenStore{}
	manager := newOAuthManager("client-id", "client-secret", store)
	manager.validated = oauthValidated{refreshToken: "previous-refresh-token"}
	manager.exchange = func(context.Context, oauth2.Config, string, string) (*oauth2.Token, error) {
		return (&oauth2.Token{RefreshToken: "refresh-token"}).WithExtra(map[string]any{"scope": oauthScope}), nil
	}
	start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback")
	state := mustQuery(t, start.AuthorizationURL).Get("state")
	if err := manager.callback(context.Background(), provider.GoogleAuthCallback{RedirectURI: "http://127.0.0.1:1234/api/v1/auth/google/callback", State: state, Code: "code"}); err != nil {
		t.Fatal(err)
	}
	if store.token != (storedRefreshToken{RefreshToken: "refresh-token", Scope: oauthScope}) {
		t.Fatalf("stored token = %#v", store.token)
	}
	if manager.validated.refreshToken != "" {
		t.Fatalf("validated token cache = %#v", manager.validated)
	}
}

func TestOAuthCallbackRequiresGrantedScope(t *testing.T) {
	tests := []struct {
		name    string
		scope   any
		present bool
		want    bool
	}{
		{name: "missing scope", want: true},
		{name: "wrong scope", scope: "https://www.googleapis.com/auth/cloud-platform.read-only", present: true, want: false},
		{name: "exact scope", scope: oauthScope, present: true, want: true},
		{name: "exact scope among granted scopes", scope: "https://www.googleapis.com/auth/userinfo.email " + oauthScope, present: true, want: true},
		{name: "malformed present scope", scope: 1, present: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryTokenStore{}
			manager := newOAuthManager("client-id", "client-secret", store)
			manager.exchange = func(context.Context, oauth2.Config, string, string) (*oauth2.Token, error) {
				token := &oauth2.Token{RefreshToken: "refresh-token"}
				if test.present {
					token = token.WithExtra(map[string]any{"scope": test.scope})
				}
				return token, nil
			}
			start := manager.start(context.Background(), "http://127.0.0.1:1234/api/v1/auth/google/callback")
			state := mustQuery(t, start.AuthorizationURL).Get("state")
			err := manager.callback(context.Background(), provider.GoogleAuthCallback{RedirectURI: "http://127.0.0.1:1234/api/v1/auth/google/callback", State: state, Code: "code"})
			if (err == nil) != test.want {
				t.Fatalf("callback() success = %t, want %t", err == nil, test.want)
			}
			if test.want {
				if store.token.Scope != oauthScope || store.token.RefreshToken == "" {
					t.Fatalf("stored token = %#v, want refresh token with required scope", store.token)
				}
			} else if store.token != (storedRefreshToken{}) {
				t.Fatalf("callback saved a token without the required scope")
			}
		})
	}
}

func TestOAuthTokenSourceCachesUntilAccessTokenExpiry(t *testing.T) {
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	manager := newOAuthManager("client-id", "client-secret", &memoryTokenStore{token: storedRefreshToken{RefreshToken: "refresh-token", Scope: oauthScope}})
	manager.now = func() time.Time { return now }
	creations := 0
	manager.newTokenSource = func(context.Context, string) oauth2.TokenSource {
		creations++
		return tokenSourceFunc(func() (*oauth2.Token, error) {
			return &oauth2.Token{AccessToken: "access-token", Expiry: now.Add(time.Hour)}, nil
		})
	}
	if _, ok := manager.tokenSource(context.Background()); !ok || creations != 1 {
		t.Fatalf("initial token source: ok=%t creations=%d", ok, creations)
	}
	now = now.Add(2 * time.Minute)
	if _, ok := manager.tokenSource(context.Background()); !ok || creations != 1 {
		t.Fatalf("cached token source: ok=%t creations=%d", ok, creations)
	}
	now = now.Add(56 * time.Minute)
	if _, ok := manager.tokenSource(context.Background()); !ok || creations != 1 {
		t.Fatalf("pre-leeway token source: ok=%t creations=%d", ok, creations)
	}
	now = now.Add(time.Minute)
	if _, ok := manager.tokenSource(context.Background()); !ok || creations != 2 {
		t.Fatalf("leeway-expired token source: ok=%t creations=%d", ok, creations)
	}
}

func TestOAuthStatusRequiresUsableToken(t *testing.T) {
	manager := newOAuthManager("client-id", "client-secret", &memoryTokenStore{token: storedRefreshToken{RefreshToken: "invalid-refresh", Scope: oauthScope}})
	manager.newTokenSource = func(context.Context, string) oauth2.TokenSource {
		return oauth2.ReuseTokenSource(nil, tokenSourceFunc(func() (*oauth2.Token, error) { return nil, errors.New("invalid") }))
	}
	status := manager.status(context.Background())
	if status.State != provider.AuthFailed {
		t.Fatalf("status = %#v", status)
	}
	if _, ok := manager.tokenSource(context.Background()); ok {
		t.Fatal("invalid refresh token supplied a token source")
	}
}

func TestInvalidOAuthRefreshTokenFallsBackToADC(t *testing.T) {
	p := New(Config{ClientID: "client-id", ClientSecret: "client-secret", TokenStore: &memoryTokenStore{token: storedRefreshToken{RefreshToken: "invalid-refresh", Scope: oauthScope}}})
	p.oauth.newTokenSource = func(context.Context, string) oauth2.TokenSource {
		return tokenSourceFunc(func() (*oauth2.Token, error) { return nil, errors.New("invalid") })
	}
	p.adcAvailable = func(context.Context) bool { return true }
	available, message := p.ADCStatus(context.Background())
	if !available || message != "Application Default Credentials are available." {
		t.Fatalf("ADCStatus() = %t, %q", available, message)
	}
	if options := p.clientOptions(context.Background()); len(options) != 0 {
		t.Fatalf("client options = %#v, want ADC fallback", options)
	}
}

func TestInvalidOAuthRefreshTokenDoesNotFallBackToUnavailableADC(t *testing.T) {
	p := New(Config{ClientID: "client-id", ClientSecret: "client-secret", TokenStore: &memoryTokenStore{token: storedRefreshToken{RefreshToken: "invalid-refresh", Scope: oauthScope}}})
	p.oauth.newTokenSource = func(context.Context, string) oauth2.TokenSource {
		return tokenSourceFunc(func() (*oauth2.Token, error) { return nil, errors.New("invalid") })
	}
	p.adcAvailable = func(context.Context) bool { return false }

	available, _ := p.ADCStatus(context.Background())
	if available {
		t.Fatal("ADCStatus() reported unavailable credentials as available")
	}
}

type tokenSourceFunc func() (*oauth2.Token, error)

func (f tokenSourceFunc) Token() (*oauth2.Token, error) { return f() }

func mustQuery(t *testing.T, rawURL string) url.Values {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query()
}
