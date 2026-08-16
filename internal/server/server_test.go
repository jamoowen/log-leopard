package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jamoowen/log-leopard/internal/auth"
	"github.com/jamoowen/log-leopard/internal/cursor"
	"github.com/jamoowen/log-leopard/internal/profile"
	providerapi "github.com/jamoowen/log-leopard/internal/provider"
	"github.com/jamoowen/log-leopard/internal/provider/fake"
)

const testOrigin = "http://127.0.0.1:8787"

func newTestServer(t *testing.T) (*Server, string) {
	return newTestServerWithProvider(t, fake.New())
}

type testProvider interface {
	LogProvider
	HealthProvider
}

func newTestServerWithProvider(t *testing.T, p testProvider) (*Server, string) {
	t.Helper()
	sessions, pairing := auth.NewManager(time.Minute, time.Hour)
	cursors := cursor.New(time.Minute)
	s, err := New(Config{
		Host: "127.0.0.1:8787", Origin: testOrigin,
		Profiles: profile.NewStore(filepath.Join(t.TempDir(), "connections.json")),
		Provider: p, HealthProvider: p, Sessions: sessions, Cursors: cursors,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, pairing
}

func createTestProfile(t *testing.T, s *Server, cookie *http.Cookie) profile.Profile {
	t.Helper()
	w := request(t, s, http.MethodPost, "/api/v1/profiles", map[string]string{"name": "Synthetic", "projectId": "synthetic-project-123"}, cookie, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("create profile status %d: %s", w.Code, w.Body.String())
	}
	var created profile.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return created
}

func request(t *testing.T, s *Server, method, path string, body any, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	r := httptest.NewRequest(method, testOrigin+path, reader)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, r)
	return w
}

func pair(t *testing.T, s *Server, token string) *http.Cookie {
	t.Helper()
	w := request(t, s, http.MethodPost, "/api/v1/session/pair", map[string]string{"token": token}, nil, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("pair status %d: %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("pair cookies: %#v", cookies)
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("insecure cookie: %#v", cookie)
	}
	return cookie
}

func TestSecurityPairingAndAuthenticatedProfiles(t *testing.T) {
	s, pairing := newTestServer(t)
	if w := request(t, s, http.MethodGet, "/api/v1/profiles", nil, nil, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status %d", w.Code)
	}
	if w := request(t, s, http.MethodPost, "/api/v1/session/pair", map[string]string{"token": pairing}, nil, "http://evil.invalid"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin pair status %d", w.Code)
	}
	cookie := pair(t, s, pairing)
	if w := request(t, s, http.MethodPost, "/api/v1/session/pair", map[string]string{"token": pairing}, nil, testOrigin); w.Code != http.StatusUnauthorized {
		t.Fatalf("pair replay status %d", w.Code)
	}
	w := request(t, s, http.MethodPost, "/api/v1/profiles", map[string]string{"name": "Synthetic", "projectId": "synthetic-project-123"}, cookie, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("permissive CORS header: %q", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("missing CSP")
	}
}

func TestLogoutInvalidatesLocalPairingSession(t *testing.T) {
	s, pairing := newTestServer(t)
	cookie := pair(t, s, pairing)
	w := request(t, s, http.MethodPost, "/api/v1/session/logout", nil, cookie, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("logout status %d: %s", w.Code, w.Body.String())
	}
	if cookie := w.Result().Cookies(); len(cookie) != 1 || cookie[0].MaxAge != -1 {
		t.Fatalf("logout cookie = %#v", cookie)
	}
	if w := request(t, s, http.MethodGet, "/api/v1/profiles", nil, cookie, testOrigin); w.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out session status %d", w.Code)
	}
}

func TestProfileWritesExposeOnlyValidationErrors(t *testing.T) {
	s, pairing := newTestServer(t)
	cookie := pair(t, s, pairing)
	w := request(t, s, http.MethodPost, "/api/v1/profiles", map[string]string{"name": " ", "projectId": "invalid"}, cookie, testOrigin)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "name must contain") {
		t.Fatalf("validation response %d: %s", w.Code, w.Body.String())
	}

	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.cfg.Profiles = profile.NewStore(filepath.Join(blockedParent, "connections.json"))
	w = request(t, s, http.MethodPost, "/api/v1/profiles", map[string]string{"name": "Synthetic", "projectId": "synthetic-project-123"}, cookie, testOrigin)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "request could not be completed") {
		t.Fatalf("persistence response %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), blockedParent) || strings.Contains(w.Body.String(), "not a directory") {
		t.Fatalf("persistence details leaked: %s", w.Body.String())
	}
}

func TestHostOriginAndOpenAPI(t *testing.T) {
	s, _ := newTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "http://localhost:8787/openapi.json", nil)
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("wrong host status %d", w.Code)
	}
	w = request(t, s, http.MethodGet, "/openapi.json", nil, nil, "")
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"openapi"`)) {
		t.Fatalf("OpenAPI status %d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{
		`"summary":"Query Cloud Run logs"`, `"summary":"Get request context"`, `"summary":"Get Cloud Run service health"`, `"summary":"Get Cloud Run fleet overview"`,
		`"description":"Exact payload path selected for message."`, `"description":"Exact structured-field comparison operator."`,
		`"X-LogLeopard-Warning"`, `"X-LogLeopard-Warning-Code"`,
	} {
		if !bytes.Contains(w.Body.Bytes(), []byte(want)) {
			t.Errorf("OpenAPI is missing %s", want)
		}
	}
}

type warningProvider struct{ *fake.Provider }

func (warningProvider) Discover(context.Context, string) providerapi.Discovery {
	return providerapi.Discovery{
		Services:    []providerapi.Service{{ID: "", Name: "All logs"}},
		Warning:     "Google Cloud authentication is unavailable or expired.",
		WarningCode: providerapi.DiscoveryWarningAuthentication,
	}
}

type deadlineDiscoveryProvider struct {
	*recordingProvider
	deadline time.Time
}

func (p *deadlineDiscoveryProvider) Discover(ctx context.Context, _ string) providerapi.Discovery {
	p.deadline, _ = ctx.Deadline()
	return providerapi.Discovery{Services: []providerapi.Service{{ID: "", Name: "All logs"}}}
}

func TestDiscoveryHasDeadlineBelowHTTPWriteTimeout(t *testing.T) {
	p := &deadlineDiscoveryProvider{recordingProvider: &recordingProvider{Provider: fake.New()}}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	response := request(t, s, http.MethodGet, "/api/v1/sources?profileId="+created.ID, nil, cookie, testOrigin)
	if response.Code != http.StatusOK || p.deadline.IsZero() || time.Until(p.deadline) > 30*time.Second {
		t.Fatalf("discovery deadline = %v, response = %d", p.deadline, response.Code)
	}
}

func TestDiscoveryPreservesSanitizedWarningWithoutChangingBodyShape(t *testing.T) {
	s, pairing := newTestServer(t)
	s.cfg.Provider = warningProvider{fake.New()}
	cookie := pair(t, s, pairing)
	w := request(t, s, http.MethodPost, "/api/v1/profiles", map[string]string{"name": "Synthetic", "projectId": "synthetic-project-123"}, cookie, testOrigin)
	var created profile.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w = request(t, s, http.MethodGet, "/api/v1/sources?profileId="+created.ID, nil, cookie, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("discover status %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-LogLeopard-Warning"); got == "" {
		t.Fatal("discovery warning header was dropped")
	}
	if got := w.Header().Get("X-LogLeopard-Warning-Code"); got != "authentication" {
		t.Fatalf("discovery warning code = %q", got)
	}
	var sources []sourceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sources); err != nil || len(sources) != 1 {
		t.Fatalf("discovery body shape changed: %#v, %v", sources, err)
	}
}

func TestQueryCursorIsBoundToRequest(t *testing.T) {
	s, pairing := newTestServer(t)
	cookie := pair(t, s, pairing)
	w := request(t, s, http.MethodPost, "/api/v1/profiles", map[string]string{"name": "Synthetic", "projectId": "synthetic-project-123"}, cookie, testOrigin)
	var created profile.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC)
	body := map[string]any{
		"profileId": created.ID, "mode": "leopard", "query": "", "start": now.Add(-2 * time.Hour),
		"end": now, "limit": 2,
	}
	w = request(t, s, http.MethodPost, "/api/v1/query", body, cookie, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("query status %d: %s", w.Code, w.Body.String())
	}
	var page struct {
		Entries []json.RawMessage `json:"entries"`
		Cursor  string            `json:"nextCursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 || page.Cursor == "" {
		t.Fatalf("unexpected page: %#v", page)
	}
	body["cursor"] = page.Cursor
	body["query"] = "different"
	w = request(t, s, http.MethodPost, "/api/v1/query", body, cookie, testOrigin)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("rebound cursor status %d: %s", w.Code, w.Body.String())
	}
}

type recordingProvider struct {
	*fake.Provider
	requests       []providerapi.QueryRequest
	queryDeadlines []time.Time
	result         providerapi.QueryResult
	results        []providerapi.QueryResult
	err            error
	healthRequests []providerapi.ServiceHealthRequest
	healthResult   providerapi.ServiceHealthResult
	healthErr      error
	fleetRequests  []providerapi.FleetOverviewRequest
	fleetResult    providerapi.FleetOverviewResult
	fleetErr       error
}

func (p *recordingProvider) ServiceHealth(_ context.Context, req providerapi.ServiceHealthRequest) (providerapi.ServiceHealthResult, error) {
	p.healthRequests = append(p.healthRequests, req)
	return p.healthResult, p.healthErr
}

func (p *recordingProvider) FleetOverview(_ context.Context, req providerapi.FleetOverviewRequest) (providerapi.FleetOverviewResult, error) {
	p.fleetRequests = append(p.fleetRequests, req)
	return p.fleetResult, p.fleetErr
}

func TestServiceHealthUsesStoredProjectAndServerBounds(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	p := &recordingProvider{Provider: fake.New()}
	p.healthResult = providerapi.ServiceHealthResult{
		Service: "checkout-api", Start: start, End: start.Add(time.Hour), Alignment: time.Minute,
		Series: []providerapi.HealthSeries{
			{Name: providerapi.HealthRequestCount, Unit: "1", Points: []providerapi.HealthPoint{}},
			{Name: providerapi.HealthServerErrorCount, Unit: "1", Points: []providerapi.HealthPoint{}},
		},
	}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	body := map[string]any{"profileId": created.ID, "service": "checkout-api", "start": start, "end": start.Add(time.Hour)}
	response := request(t, s, http.MethodPost, "/api/v1/service-health", body, cookie, testOrigin)
	if response.Code != http.StatusOK {
		t.Fatalf("health status %d: %s", response.Code, response.Body.String())
	}
	if len(p.healthRequests) != 1 {
		t.Fatalf("health requests = %d", len(p.healthRequests))
	}
	got := p.healthRequests[0]
	if got.ProjectID != "synthetic-project-123" || got.Service != "checkout-api" || got.Alignment != time.Minute {
		t.Fatalf("unexpected provider request: %#v", got)
	}
	if strings.Contains(response.Body.String(), "synthetic-project-123") {
		t.Fatalf("project leaked into response: %s", response.Body.String())
	}
}

func TestServiceHealthRejectsInvalidScopeBeforeProviderCall(t *testing.T) {
	p := &recordingProvider{Provider: fake.New()}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for _, body := range []map[string]any{
		{"profileId": created.ID, "service": `api" OR true`, "start": start, "end": start.Add(time.Hour)},
		{"profileId": created.ID, "service": "checkout-api", "start": start, "end": start},
		{"profileId": created.ID, "service": "checkout-api", "start": start, "end": start.Add(8 * 24 * time.Hour)},
	} {
		response := request(t, s, http.MethodPost, "/api/v1/service-health", body, cookie, testOrigin)
		if response.Code != http.StatusBadRequest && response.Code != http.StatusUnprocessableEntity {
			t.Errorf("invalid scope status %d: %s", response.Code, response.Body.String())
		}
	}
	if len(p.healthRequests) != 0 {
		t.Fatalf("provider received invalid requests: %#v", p.healthRequests)
	}
}

func TestServiceHealthReturnsMonitoringSpecificProviderErrors(t *testing.T) {
	tests := []struct {
		err        error
		statusCode int
		detail     string
	}{
		{err: providerapi.ErrPermissionDenied, statusCode: http.StatusForbidden, detail: "roles/monitoring.viewer"},
		{err: providerapi.ErrUnavailable, statusCode: http.StatusServiceUnavailable, detail: "Cloud Monitoring"},
		{err: providerapi.ErrConfiguration, statusCode: http.StatusFailedDependency, detail: "Cloud Monitoring API"},
		{err: providerapi.ErrInvalidQuery, statusCode: http.StatusInternalServerError, detail: "service health could not be loaded"},
	}
	for _, test := range tests {
		p := &recordingProvider{Provider: fake.New(), healthErr: fmt.Errorf("private filter: %w", test.err)}
		s, pairing := newTestServerWithProvider(t, p)
		cookie := pair(t, s, pairing)
		created := createTestProfile(t, s, cookie)
		now := time.Now().UTC()
		response := request(t, s, http.MethodPost, "/api/v1/service-health", map[string]any{
			"profileId": created.ID, "service": "checkout-api", "start": now.Add(-time.Hour), "end": now,
		}, cookie, testOrigin)
		if response.Code != test.statusCode || !strings.Contains(response.Body.String(), test.detail) || strings.Contains(response.Body.String(), "private filter") {
			t.Errorf("error response %d: %s", response.Code, response.Body.String())
		}
	}
}

func TestHealthAlignmentCapsBuckets(t *testing.T) {
	tests := []struct {
		window time.Duration
		want   time.Duration
	}{
		{window: time.Hour, want: time.Minute},
		{window: 5 * time.Hour, want: time.Minute},
		{window: 5*time.Hour + time.Second, want: 2 * time.Minute},
		{window: 7 * 24 * time.Hour, want: 34 * time.Minute},
	}
	for _, test := range tests {
		if got := healthAlignment(test.window); got != test.want {
			t.Errorf("healthAlignment(%s) = %s, want %s", test.window, got, test.want)
		}
	}
}

func TestFleetOverviewUsesStoredProjectAndOneProviderCall(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	latency := 125.0
	p := &recordingProvider{Provider: fake.New()}
	p.fleetResult = providerapi.FleetOverviewResult{
		Start: start, End: start.Add(time.Hour),
		Services: []providerapi.FleetOverviewSummary{
			{Service: "checkout-api", RequestCount: 42, ServerErrorCount: 2, RequestLatencyP95Ms: &latency},
			{Service: "worker"},
		},
	}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	response := request(t, s, http.MethodPost, "/api/v1/fleet-overview", map[string]any{
		"profileId": created.ID, "services": []string{"worker", "checkout-api"}, "start": start, "end": start.Add(time.Hour),
	}, cookie, testOrigin)
	if response.Code != http.StatusOK {
		t.Fatalf("fleet status %d: %s", response.Code, response.Body.String())
	}
	if len(p.fleetRequests) != 1 {
		t.Fatalf("fleet requests = %d", len(p.fleetRequests))
	}
	got := p.fleetRequests[0]
	if got.ProjectID != "synthetic-project-123" || !slices.Equal(got.Services, []string{"worker", "checkout-api"}) || !got.Start.Equal(start) || !got.End.Equal(start.Add(time.Hour)) {
		t.Fatalf("unexpected provider request: %#v", got)
	}
	if strings.Contains(response.Body.String(), "synthetic-project-123") || !strings.Contains(response.Body.String(), `"requestLatencyP95Ms":null`) {
		t.Fatalf("unexpected fleet response: %s", response.Body.String())
	}
}

func TestFleetOverviewRejectsInvalidScopeBeforeProviderCall(t *testing.T) {
	p := &recordingProvider{Provider: fake.New()}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	tooMany := make([]string, providerapi.MaxFleetServices+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("service-%d", i)
	}
	tests := []map[string]any{
		{"profileId": created.ID, "services": []string{}, "start": start, "end": start.Add(time.Hour)},
		{"profileId": created.ID, "services": []string{"api", "api"}, "start": start, "end": start.Add(time.Hour)},
		{"profileId": created.ID, "services": []string{`api" OR true`}, "start": start, "end": start.Add(time.Hour)},
		{"profileId": created.ID, "services": tooMany, "start": start, "end": start.Add(time.Hour)},
		{"profileId": created.ID, "services": []string{"api"}, "start": start, "end": start.Add(8 * 24 * time.Hour)},
		{"profileId": created.ID, "services": []string{"api"}, "start": start, "end": start.Add(time.Hour), "metric": "custom.googleapis.com/private"},
	}
	for _, body := range tests {
		response := request(t, s, http.MethodPost, "/api/v1/fleet-overview", body, cookie, testOrigin)
		if response.Code != http.StatusBadRequest && response.Code != http.StatusUnprocessableEntity {
			t.Errorf("invalid fleet scope status %d: %s", response.Code, response.Body.String())
		}
	}
	if len(p.fleetRequests) != 0 {
		t.Fatalf("provider received invalid fleet requests: %#v", p.fleetRequests)
	}
}

func (p *recordingProvider) Query(ctx context.Context, req providerapi.QueryRequest) (providerapi.QueryResult, error) {
	p.requests = append(p.requests, req)
	deadline, _ := ctx.Deadline()
	p.queryDeadlines = append(p.queryDeadlines, deadline)
	if len(p.results) >= len(p.requests) {
		return p.results[len(p.requests)-1], p.err
	}
	return p.result, p.err
}

func TestQueryReturnsActionableProviderErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		detail     string
	}{
		{name: "authentication", err: providerapi.ErrAuthentication, statusCode: http.StatusFailedDependency, detail: "Refresh ADC"},
		{name: "permission", err: providerapi.ErrPermissionDenied, statusCode: http.StatusForbidden, detail: "roles/logging.viewer"},
		{name: "rate limit", err: providerapi.ErrRateLimited, statusCode: http.StatusTooManyRequests, detail: "rate-limited"},
		{name: "unavailable", err: providerapi.ErrUnavailable, statusCode: http.StatusServiceUnavailable, detail: "temporarily unavailable"},
		{name: "invalid query", err: providerapi.ErrInvalidQuery, statusCode: http.StatusBadRequest, detail: "rejected the query"},
		{name: "configuration", err: providerapi.ErrConfiguration, statusCode: http.StatusFailedDependency, detail: "Cloud Logging API"},
		{name: "deadline", err: context.DeadlineExceeded, statusCode: http.StatusServiceUnavailable, detail: "temporarily unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &recordingProvider{Provider: fake.New(), err: fmt.Errorf("provider detail must remain private: %w", test.err)}
			s, pairing := newTestServerWithProvider(t, p)
			cookie := pair(t, s, pairing)
			created := createTestProfile(t, s, cookie)
			now := time.Now().UTC()
			body := map[string]any{
				"profileId": created.ID,
				"mode":      "leopard",
				"start":     now.Add(-time.Minute),
				"end":       now,
				"limit":     20,
			}
			response := request(t, s, http.MethodPost, "/api/v1/query", body, cookie, testOrigin)
			if response.Code != test.statusCode || !strings.Contains(response.Body.String(), test.detail) {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "provider detail must remain private") {
				t.Fatal("provider error detail leaked to the response")
			}
		})
	}
}

func TestQueryRejectsOverlongCompiledFilterAndInvalidSourceBeforeProviderCall(t *testing.T) {
	p := &recordingProvider{Provider: fake.New()}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	now := time.Now().UTC()
	predicates := make([]map[string]any, 10)
	for i := range predicates {
		predicates[i] = map[string]any{"path": fmt.Sprintf("field%d", i), "operator": "equals", "value": strings.Repeat("x", 2048)}
	}
	tests := []struct {
		body map[string]any
		want string
	}{
		{body: map[string]any{"profileId": created.ID, "mode": "structured", "start": now.Add(-time.Hour), "end": now, "limit": 20, "predicates": predicates}, want: "compiled filter exceeds the 19000-byte limit; shorten the query or select fewer filters"},
		{body: map[string]any{"profileId": created.ID, "mode": "leopard", "start": now.Add(-time.Hour), "end": now, "limit": 20, "sources": []string{"INVALID"}}, want: "valid Cloud Run service name"},
	}
	for _, test := range tests {
		response := request(t, s, http.MethodPost, "/api/v1/query", test.body, cookie, testOrigin)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.want) {
			t.Errorf("response %d: %s", response.Code, response.Body.String())
		}
	}
	if len(p.requests) != 0 {
		t.Fatalf("provider received invalid filters: %#v", p.requests)
	}
}

func TestQueryCursorExpiryIsOnlyReturnedWithNextCursor(t *testing.T) {
	p := &recordingProvider{Provider: fake.New()}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	body := map[string]any{"profileId": created.ID, "mode": "leopard", "start": time.Now().Add(-time.Hour), "end": time.Now(), "limit": 20}
	response := request(t, s, http.MethodPost, "/api/v1/query", body, cookie, testOrigin)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "expiresAt") {
		t.Fatalf("terminal page response %d: %s", response.Code, response.Body.String())
	}
	p.result.NextPageToken = "next-page"
	response = request(t, s, http.MethodPost, "/api/v1/query", body, cookie, testOrigin)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "nextCursor") || !strings.Contains(response.Body.String(), "expiresAt") {
		t.Fatalf("paginated response %d: %s", response.Code, response.Body.String())
	}
}

func TestOversizedLogPageGuidanceUsesLimit(t *testing.T) {
	p := &recordingProvider{Provider: fake.New(), err: providerapi.ErrResponseTooLarge}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	response := request(t, s, http.MethodPost, "/api/v1/query", map[string]any{
		"profileId": created.ID, "mode": "leopard", "start": time.Now().Add(-time.Hour), "end": time.Now(), "limit": 20,
	}, cookie, testOrigin)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "smaller limit") || strings.Contains(response.Body.String(), "pageSize") {
		t.Fatalf("oversized page response %d: %s", response.Code, response.Body.String())
	}
}

func TestLogOperationsHaveDeadlineBelowHTTPWriteTimeout(t *testing.T) {
	p := &recordingProvider{Provider: fake.New()}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	now := time.Now()
	response := request(t, s, http.MethodPost, "/api/v1/query", map[string]any{
		"profileId": created.ID, "mode": "leopard", "start": now.Add(-time.Hour), "end": now, "limit": 20,
	}, cookie, testOrigin)
	if response.Code != http.StatusOK || len(p.queryDeadlines) != 1 || p.queryDeadlines[0].IsZero() || time.Until(p.queryDeadlines[0]) > 30*time.Second {
		t.Fatalf("query deadline = %v, response = %d", p.queryDeadlines, response.Code)
	}
}

func TestStructuredQueryCompilesPredicatesAndRejectsIncompatibleFields(t *testing.T) {
	p := &recordingProvider{Provider: fake.New()}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	now := time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC)
	base := map[string]any{
		"profileId": created.ID, "mode": "structured", "start": now.Add(-time.Hour), "end": now, "limit": 20,
		"sources": []string{"api"}, "severities": []string{"error"},
		"predicates": []map[string]any{{"path": "request.id", "operator": "equals", "value": `req-"quoted"`}},
	}
	w := request(t, s, http.MethodPost, "/api/v1/query", base, cookie, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("structured query status %d: %s", w.Code, w.Body.String())
	}
	if len(p.requests) != 1 {
		t.Fatalf("provider requests: %d", len(p.requests))
	}
	for _, want := range []string{`resource.labels.service_name = "api"`, `severity = "ERROR"`, `jsonPayload.request.id = "req-\"quoted\""`} {
		if !strings.Contains(p.requests[0].Filter, want) {
			t.Errorf("compiled filter missing %q: %s", want, p.requests[0].Filter)
		}
	}

	for name, change := range map[string]func(map[string]any){
		"structured query text": func(body map[string]any) { body["query"] = "timeout" },
		"leopard predicates":    func(body map[string]any) { body["mode"] = "leopard" },
		"native predicates":     func(body map[string]any) { body["mode"] = "native"; body["query"] = `severity="ERROR"` },
	} {
		t.Run(name, func(t *testing.T) {
			body := maps.Clone(base)
			change(body)
			got := request(t, s, http.MethodPost, "/api/v1/query", body, cookie, testOrigin)
			if got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "incompatible") {
				t.Fatalf("status %d: %s", got.Code, got.Body.String())
			}
		})
	}
}

func TestStructuredQueryRejectsMalformedAndTooManyPredicates(t *testing.T) {
	s, pairing := newTestServer(t)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	now := time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC)
	predicates := make([]map[string]any, 51)
	for i := range predicates {
		predicates[i] = map[string]any{"path": "value", "operator": "equals", "value": "x"}
	}
	for name, values := range map[string][]map[string]any{
		"malformed":     {{"path": "request..id", "operator": "equals", "value": "x"}},
		"wrong type":    {{"path": "duration", "operator": "gt", "value": "10"}},
		"missing value": {{"path": "request.id", "operator": "equals"}},
		"too many":      predicates,
	} {
		t.Run(name, func(t *testing.T) {
			body := map[string]any{"profileId": created.ID, "mode": "structured", "start": now.Add(-time.Hour), "end": now, "limit": 20, "predicates": values}
			w := request(t, s, http.MethodPost, "/api/v1/query", body, cookie, testOrigin)
			if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRequestContextQueriesAllServicesInExactWindowAndAscendingOrder(t *testing.T) {
	event := time.Date(2026, 3, 4, 12, 30, 0, 0, time.UTC)
	beforeEntries := make([]providerapi.Entry, providerapi.MaxPageSize/2)
	afterEntries := make([]providerapi.Entry, providerapi.MaxPageSize/2)
	for i := range beforeEntries {
		beforeEntries[i] = providerapi.Entry{ID: strconv.Itoa(i), Timestamp: event.Add(time.Duration(i-100) * time.Second)}
		afterEntries[i] = providerapi.Entry{ID: strconv.Itoa(i + 100), Timestamp: event.Add(time.Duration(i) * time.Second)}
	}
	p := &recordingProvider{Provider: fake.New(), results: []providerapi.QueryResult{{Entries: beforeEntries}, {Entries: afterEntries}}}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	body := map[string]any{"profileId": created.ID, "eventTimestamp": event, "requestId": "request-123", "traceId": "trace-123"}
	w := request(t, s, http.MethodPost, "/api/v1/request-context", body, cookie, testOrigin)
	if w.Code != http.StatusOK {
		t.Fatalf("context status %d: %s", w.Code, w.Body.String())
	}
	if len(p.requests) != 2 {
		t.Fatalf("provider requests: %d", len(p.requests))
	}
	if p.requests[0].Order != providerapi.OrderDescending || p.requests[1].Order != providerapi.OrderAscending {
		t.Fatalf("context query orders = %q, %q", p.requests[0].Order, p.requests[1].Order)
	}
	for _, deadline := range p.queryDeadlines {
		if deadline.IsZero() || time.Until(deadline) > 30*time.Second {
			t.Fatalf("request-context deadline = %v", deadline)
		}
	}
	for _, req := range p.requests {
		if req.ProjectID != "synthetic-project-123" || req.PageSize != 100 || req.PageToken != "" {
			t.Fatalf("unexpected context provider request: %#v", req)
		}
		for _, want := range []string{`resource.type = "cloud_run_revision"`, `jsonPayload.requestId = "request-123"`, `trace = "trace-123"`} {
			if !strings.Contains(req.Filter, want) {
				t.Errorf("filter missing %q: %s", want, req.Filter)
			}
		}
		if strings.Contains(req.Filter, "service_name") || strings.Contains(req.Filter, "severity") {
			t.Fatalf("context query was not project-wide: %s", req.Filter)
		}
	}
	if !strings.Contains(p.requests[0].Filter, `timestamp < "2026-03-04T12:30:00Z"`) || !strings.Contains(p.requests[1].Filter, `timestamp >= "2026-03-04T12:30:00Z"`) {
		t.Fatalf("context queries were not split around event: %#v", p.requests)
	}
	var output struct {
		Entries []providerapi.Entry `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &output); err != nil || len(output.Entries) != providerapi.MaxPageSize || output.Entries[0].ID != "0" || output.Entries[len(output.Entries)-1].ID != "199" {
		t.Fatalf("unexpected context response: %#v, %v", output, err)
	}
}

func TestRequestContextValidatesIdentifiersAndAuthentication(t *testing.T) {
	s, pairing := newTestServer(t)
	event := time.Date(2026, 3, 4, 12, 30, 0, 0, time.UTC)
	if w := request(t, s, http.MethodPost, "/api/v1/request-context", map[string]any{"profileId": "missing", "eventTimestamp": event, "requestId": "request-1"}, nil, testOrigin); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status %d", w.Code)
	}
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	for _, body := range []map[string]any{
		{"profileId": created.ID, "eventTimestamp": event},
		{"profileId": created.ID, "eventTimestamp": event, "requestId": " ", "traceId": ""},
	} {
		w := request(t, s, http.MethodPost, "/api/v1/request-context", body, cookie, testOrigin)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("missing identifiers status %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestRequestContextSanitizesProviderAndResponseSizeErrors(t *testing.T) {
	for name, p := range map[string]*recordingProvider{
		"provider": {Provider: fake.New(), err: errors.New("secret-project filter denied")},
		"size":     {Provider: fake.New(), result: providerapi.QueryResult{Entries: []providerapi.Entry{{Raw: json.RawMessage(`"` + strings.Repeat("x", providerapi.MaxResponseBytes) + `"`)}}}},
	} {
		t.Run(name, func(t *testing.T) {
			s, pairing := newTestServerWithProvider(t, p)
			cookie := pair(t, s, pairing)
			created := createTestProfile(t, s, cookie)
			w := request(t, s, http.MethodPost, "/api/v1/request-context", map[string]any{
				"profileId": created.ID, "eventTimestamp": time.Now(), "requestId": "request-1",
			}, cookie, testOrigin)
			if name == "provider" {
				if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "secret-project") {
					t.Fatalf("unsanitized provider response %d: %s", w.Code, w.Body.String())
				}
			} else if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("oversized response status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRequestContextDeadlineIsSanitizedUnavailable(t *testing.T) {
	p := &recordingProvider{Provider: fake.New(), err: context.DeadlineExceeded}
	s, pairing := newTestServerWithProvider(t, p)
	cookie := pair(t, s, pairing)
	created := createTestProfile(t, s, cookie)
	response := request(t, s, http.MethodPost, "/api/v1/request-context", map[string]any{
		"profileId": created.ID, "eventTimestamp": time.Now(), "requestId": "request-1",
	}, cookie, testOrigin)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "temporarily unavailable") || strings.Contains(response.Body.String(), "deadline") {
		t.Fatalf("deadline response %d: %s", response.Code, response.Body.String())
	}
}
