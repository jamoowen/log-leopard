package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/jamoowen/log-leopard/internal/auth"
	"github.com/jamoowen/log-leopard/internal/cursor"
	"github.com/jamoowen/log-leopard/internal/profile"
	"github.com/jamoowen/log-leopard/internal/provider"
	"github.com/jamoowen/log-leopard/internal/query"
	webassets "github.com/jamoowen/log-leopard/web"
)

type Config struct {
	Host     string
	Origin   string
	Profiles *profile.Store
	Provider provider.Provider
	Sessions *auth.Manager
	Cursors  *cursor.Signer
	Logger   *slog.Logger
}

type Server struct {
	Handler http.Handler
	API     huma.API
	cfg     Config
}

func New(cfg Config) (*Server, error) {
	if err := validateHost(cfg.Host); err != nil {
		return nil, err
	}
	if cfg.Origin != "http://"+cfg.Host && cfg.Origin != "https://"+cfg.Host {
		return nil, errors.New("origin must exactly match the listening host")
	}
	if cfg.Profiles == nil || cfg.Provider == nil || cfg.Sessions == nil || cfg.Cursors == nil {
		return nil, errors.New("server dependencies are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	mux := http.NewServeMux()
	humaConfig := huma.DefaultConfig("LogLeopard API", "0.1.0")
	humaConfig.OpenAPIPath = "/openapi"
	humaConfig.DocsPath = ""
	humaConfig.SchemasPath = ""
	humaConfig.RejectUnknownQueryParameters = true
	humaConfig.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"session": {Type: "apiKey", In: "cookie", Name: auth.CookieName, Description: "Local pairing session"},
	}
	api := humago.New(mux, humaConfig)
	s := &Server{API: api, cfg: cfg}
	s.register()
	mux.Handle("/", spaHandler(webassets.Assets()))
	s.Handler = s.security(s.authenticate(mux))
	return s, nil
}

func validateHost(hostport string) error {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return errors.New("host must include a numeric loopback address and port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("host must use a numeric loopback address")
	}
	return nil
}

type pairInput struct {
	Body struct {
		Token string `json:"token" minLength:"20" doc:"Single-use pairing token from the startup URL fragment."`
	}
}
type cookieOutput struct {
	SetCookie string `header:"Set-Cookie"`
	Body      struct {
		Paired bool `json:"paired" doc:"Whether pairing succeeded."`
	}
}

type emptyInput struct{}
type messageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

type profilesOutput struct {
	Body []profileResponse
}
type profileResponse struct {
	ID        string `json:"id" doc:"Stable local connection profile ID."`
	Name      string `json:"name" doc:"User-visible connection name."`
	ProjectID string `json:"projectId" doc:"GCP project ID."`
	Provider  string `json:"provider" enum:"gcp" doc:"Cloud provider."`
	Status    string `json:"status" enum:"ready,needs-auth" doc:"Current local credential status."`
}
type profileBody struct {
	Name      string `json:"name" minLength:"1" maxLength:"80" doc:"User-visible connection name."`
	ProjectID string `json:"projectId" minLength:"6" maxLength:"30" doc:"GCP project ID; credentials are never stored."`
}
type createProfileInput struct{ Body profileBody }
type updateProfileInput struct {
	ID   string `path:"id"`
	Body profileBody
}
type profileOutput struct{ Body profileResponse }
type idInput struct {
	ID string `path:"id"`
}
type sourcesInput struct {
	ProfileID string `query:"profileId" required:"true" doc:"Connection profile ID."`
}
type sourceResponse struct {
	ID    string `json:"id" doc:"Cloud Run service name; empty means all Cloud Run revision logs."`
	Label string `json:"label" doc:"User-visible source label."`
	Kind  string `json:"kind" enum:"cloud-run,log" doc:"Source selector kind."`
}
type servicesOutput struct {
	Warning string `header:"X-LogLeopard-Warning" doc:"Sanitized discovery fallback warning, when discovery was incomplete."`
	Body    []sourceResponse
}
type authStatusOutput struct {
	Body struct {
		Available bool   `json:"available" doc:"Whether Application Default Credentials are available."`
		Message   string `json:"message" doc:"Sanitized credential status message."`
	}
}
type queryInput struct {
	Body struct {
		ProfileID  string                 `json:"profileId" doc:"Connection profile ID."`
		Mode       string                 `json:"mode" enum:"leopard,structured,native" doc:"Query language mode; native passes query as a GCP Logging filter fragment."`
		Query      string                 `json:"query,omitempty" maxLength:"4000" doc:"Search expression or native GCP filter fragment, according to mode."`
		Predicates []query.FieldPredicate `json:"predicates,omitempty" doc:"Provider-neutral structured payload predicates. Only valid in structured mode; all predicates are combined with AND."`
		Sources    []string               `json:"sources,omitempty" maxItems:"100" doc:"Cloud Run service names to include."`
		Severities []string               `json:"severities,omitempty" maxItems:"9" doc:"Exact normalized severities to include; WARN is accepted as WARNING."`
		Start      time.Time              `json:"start" doc:"Inclusive absolute start timestamp."`
		End        time.Time              `json:"end" doc:"Exclusive absolute end timestamp, no more than seven days after start."`
		Cursor     string                 `json:"cursor,omitempty" maxLength:"8192" doc:"Opaque cursor returned by the preceding identical query."`
		Limit      int                    `json:"limit" minimum:"1" maximum:"200" doc:"Maximum entries to return."`
	}
}
type queryOutput struct {
	Body struct {
		Entries    []provider.Entry `json:"entries" doc:"Normalized log entries in descending timestamp order."`
		NextCursor string           `json:"nextCursor,omitempty" doc:"Opaque cursor for the next page."`
		ExpiresAt  time.Time        `json:"expiresAt" doc:"Cursor expiry timestamp."`
	}
}
type requestContextInput struct {
	Body struct {
		ProfileID      string    `json:"profileId" doc:"Connection profile ID."`
		EventTimestamp time.Time `json:"eventTimestamp" doc:"Selected event timestamp at the center of the context window."`
		RequestID      string    `json:"requestId,omitempty" maxLength:"2048" doc:"Exact request ID from the selected event. At least requestId or traceId is required."`
		TraceID        string    `json:"traceId,omitempty" maxLength:"2048" doc:"Exact trace resource name or trace ID from the selected event. At least requestId or traceId is required."`
	}
}
type requestContextOutput struct {
	Body struct {
		Entries []provider.Entry `json:"entries" doc:"Matching Cloud Run revision entries across all services, in ascending timestamp order."`
	}
}
type serviceHealthInput struct {
	Body struct {
		ProfileID string    `json:"profileId" minLength:"1" maxLength:"128" doc:"Connection profile ID; the project is resolved only from this stored profile."`
		Service   string    `json:"service" minLength:"1" maxLength:"49" pattern:"^[a-z](?:[a-z0-9-]{0,47}[a-z0-9])?$" doc:"Exact Cloud Run service name, aggregated across matching revisions and regions in the profile project."`
		Start     time.Time `json:"start" doc:"Inclusive absolute start timestamp."`
		End       time.Time `json:"end" doc:"Exclusive absolute end timestamp, no more than seven days after start."`
	}
}
type serviceHealthOutput struct {
	Body struct {
		Service          string                  `json:"service" doc:"Requested Cloud Run service name."`
		Start            time.Time               `json:"start" doc:"Inclusive absolute start timestamp."`
		End              time.Time               `json:"end" doc:"Exclusive absolute end timestamp."`
		AlignmentSeconds int64                   `json:"alignmentSeconds" minimum:"60" doc:"Server-selected metric bucket width in seconds."`
		Series           []provider.HealthSeries `json:"series" maxItems:"2" doc:"Fixed request and server-error count series."`
	}
}

func (s *Server) register() {
	session := []map[string][]string{{"session": {}}}
	huma.Register(s.API, operation("pair", http.MethodPost, "/api/v1/session/pair", "Pair browser session", "Exchange the single-use startup token for a local HttpOnly session cookie.", nil), s.pair)
	huma.Register(s.API, operation("logout", http.MethodPost, "/api/v1/session/logout", "Log out", "Invalidate the current local session and expire its cookie.", session), s.logout)
	huma.Register(s.API, operation("auth-status", http.MethodGet, "/api/v1/auth/status", "Get credential status", "Report sanitized GCP Application Default Credential availability.", session), s.authStatus)
	huma.Register(s.API, operation("list-profiles", http.MethodGet, "/api/v1/profiles", "List connection profiles", "List local GCP connection profiles without credentials.", session), s.listProfiles)
	huma.Register(s.API, operation("create-profile", http.MethodPost, "/api/v1/profiles", "Create connection profile", "Create a local profile containing a display name and GCP project ID.", session), s.createProfile)
	huma.Register(s.API, operation("update-profile", http.MethodPut, "/api/v1/profiles/{id}", "Update connection profile", "Replace the name and GCP project ID of a local connection profile.", session), s.updateProfile)
	huma.Register(s.API, operation("delete-profile", http.MethodDelete, "/api/v1/profiles/{id}", "Delete connection profile", "Delete a local connection profile; cloud resources are not changed.", session), s.deleteProfile)
	huma.Register(s.API, operation("discover-sources", http.MethodGet, "/api/v1/sources", "Discover Cloud Run services", "List Cloud Run services for a profile; an all-logs fallback and sanitized warning header are returned when discovery fails.", session), s.discover)
	huma.Register(s.API, operation("query-logs", http.MethodPost, "/api/v1/query", "Query Cloud Run logs", "Query a bounded time window of Cloud Run revision logs and return normalized entries plus an opaque cursor.", session), s.queryLogs)
	huma.Register(s.API, operation("request-context", http.MethodPost, "/api/v1/request-context", "Get request context", "Find up to 200 Cloud Run revision entries across all services in the profile project, within 15 minutes before or after the selected event, by exact known request ID or trace ID locations.", session), s.requestContext)
	huma.Register(s.API, operation("service-health", http.MethodPost, "/api/v1/service-health", "Get Cloud Run service health", "Return bounded request and server-error counts for one Cloud Run service using a fixed read-only Cloud Monitoring query. Metrics can be delayed by approximately two minutes.", session), s.serviceHealth)
}

func operation(id, method, path, summary, description string, security []map[string][]string) huma.Operation {
	return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Description: description, Security: security, Tags: []string{"LogLeopard"}}
}

func (s *Server) pair(_ context.Context, input *pairInput) (*cookieOutput, error) {
	session, expires, err := s.cfg.Sessions.Exchange(input.Body.Token)
	if err != nil {
		return nil, huma.Error401Unauthorized("pairing token is invalid or expired")
	}
	out := &cookieOutput{}
	out.SetCookie = (&http.Cookie{Name: auth.CookieName, Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())}).String()
	out.Body.Paired = true
	return out, nil
}

func (s *Server) logout(ctx context.Context, _ *emptyInput) (*cookieOutput, error) {
	request, _ := requestFromContext(ctx)
	if cookie, err := request.Cookie(auth.CookieName); err == nil {
		s.cfg.Sessions.Logout(cookie.Value)
	}
	out := &cookieOutput{SetCookie: (&http.Cookie{Name: auth.CookieName, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1}).String()}
	return out, nil
}

func (s *Server) authStatus(ctx context.Context, _ *emptyInput) (*authStatusOutput, error) {
	available, message := s.cfg.Provider.ADCStatus(ctx)
	out := &authStatusOutput{}
	out.Body.Available, out.Body.Message = available, message
	return out, nil
}

func (s *Server) listProfiles(ctx context.Context, _ *emptyInput) (*profilesOutput, error) {
	profiles, err := s.cfg.Profiles.List()
	if err != nil {
		return nil, s.internal("list_profiles")
	}
	available, _ := s.cfg.Provider.ADCStatus(ctx)
	out := &profilesOutput{Body: make([]profileResponse, 0, len(profiles))}
	for _, p := range profiles {
		out.Body = append(out.Body, presentProfile(p, available))
	}
	return out, nil
}

func (s *Server) createProfile(ctx context.Context, input *createProfileInput) (*profileOutput, error) {
	p, err := s.cfg.Profiles.Save(profile.Profile{Name: input.Body.Name, ProjectID: input.Body.ProjectID})
	if err != nil {
		if message, ok := profile.ValidationMessage(err); ok {
			return nil, huma.Error400BadRequest(message)
		}
		return nil, s.internal("create_profile")
	}
	available, _ := s.cfg.Provider.ADCStatus(ctx)
	return &profileOutput{Body: presentProfile(p, available)}, nil
}

func (s *Server) updateProfile(ctx context.Context, input *updateProfileInput) (*profileOutput, error) {
	p, err := s.cfg.Profiles.Save(profile.Profile{ID: input.ID, Name: input.Body.Name, ProjectID: input.Body.ProjectID})
	if errors.Is(err, profile.ErrNotFound) {
		return nil, huma.Error404NotFound("connection not found")
	}
	if err != nil {
		if message, ok := profile.ValidationMessage(err); ok {
			return nil, huma.Error400BadRequest(message)
		}
		return nil, s.internal("update_profile")
	}
	available, _ := s.cfg.Provider.ADCStatus(ctx)
	return &profileOutput{Body: presentProfile(p, available)}, nil
}

func (s *Server) deleteProfile(_ context.Context, input *idInput) (*messageOutput, error) {
	if err := s.cfg.Profiles.Delete(input.ID); errors.Is(err, profile.ErrNotFound) {
		return nil, huma.Error404NotFound("connection not found")
	} else if err != nil {
		return nil, s.internal("delete_profile")
	}
	out := &messageOutput{}
	out.Body.Message = "connection deleted"
	return out, nil
}

func (s *Server) discover(ctx context.Context, input *sourcesInput) (*servicesOutput, error) {
	p, err := s.findProfile(input.ProfileID)
	if err != nil {
		return nil, err
	}
	discovery := s.cfg.Provider.Discover(ctx, p.ProjectID)
	out := &servicesOutput{Warning: discovery.Warning, Body: make([]sourceResponse, 0, len(discovery.Services))}
	for _, service := range discovery.Services {
		kind := "cloud-run"
		if service.ID == "" {
			kind = "log"
		}
		out.Body = append(out.Body, sourceResponse{ID: service.ID, Label: service.Name, Kind: kind})
	}
	return out, nil
}

func (s *Server) queryLogs(ctx context.Context, input *queryInput) (*queryOutput, error) {
	p, err := s.findProfile(input.Body.ProfileID)
	if err != nil {
		return nil, err
	}
	if input.Body.Limit < 1 || input.Body.Limit > provider.MaxPageSize {
		return nil, huma.Error400BadRequest("limit must be between 1 and 200")
	}
	text, native := "", ""
	switch input.Body.Mode {
	case "leopard":
		if len(input.Body.Predicates) != 0 {
			return nil, huma.Error400BadRequest("predicates are incompatible with leopard mode")
		}
		text = input.Body.Query
	case "structured":
		if input.Body.Query != "" {
			return nil, huma.Error400BadRequest("query is incompatible with structured mode; use predicates")
		}
	case "native":
		if len(input.Body.Predicates) != 0 {
			return nil, huma.Error400BadRequest("predicates are incompatible with native mode")
		}
		native = input.Body.Query
	default:
		return nil, huma.Error400BadRequest("mode must be leopard, structured, or native")
	}
	filter, err := query.Compile(query.CompileInput{
		Text: text, Sources: input.Body.Sources, Severities: input.Body.Severities,
		Start: input.Body.Start, End: input.Body.End, NativeFilter: native, Predicates: input.Body.Predicates,
	})
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	fingerprint := cursor.Fingerprint(p.ID, p.ProjectID, filter, strconv.Itoa(input.Body.Limit))
	pageToken := ""
	if input.Body.Cursor != "" {
		pageToken, err = s.cfg.Cursors.Decode(input.Body.Cursor, fingerprint)
		if err != nil {
			return nil, huma.Error400BadRequest("cursor is invalid or expired")
		}
	}
	result, err := s.cfg.Provider.Query(ctx, provider.QueryRequest{ProjectID: p.ProjectID, Filter: filter, PageSize: input.Body.Limit, PageToken: pageToken})
	if err != nil {
		if errors.Is(err, provider.ErrResponseTooLarge) {
			return nil, huma.Error422UnprocessableEntity("log page exceeds the response size limit; use a smaller pageSize")
		}
		return nil, s.providerFailure("query_logs", err)
	}
	out := &queryOutput{}
	out.Body.Entries = result.Entries
	out.Body.ExpiresAt = s.cfg.Cursors.ExpiresAt().UTC()
	if result.NextPageToken != "" {
		out.Body.NextCursor, err = s.cfg.Cursors.Encode(result.NextPageToken, fingerprint)
		if err != nil {
			return nil, s.internal("encode_cursor")
		}
	}
	if err := s.validateResponseSize(out.Body, "log page exceeds the response size limit; use a smaller pageSize"); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) requestContext(ctx context.Context, input *requestContextInput) (*requestContextOutput, error) {
	p, err := s.findProfile(input.Body.ProfileID)
	if err != nil {
		return nil, err
	}
	filter, err := query.CompileRequestContext(input.Body.EventTimestamp, input.Body.RequestID, input.Body.TraceID)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	result, err := s.cfg.Provider.Query(ctx, provider.QueryRequest{
		ProjectID: p.ProjectID, Filter: filter, PageSize: provider.MaxPageSize, Order: provider.OrderAscending,
	})
	if err != nil {
		if errors.Is(err, provider.ErrResponseTooLarge) {
			return nil, huma.Error422UnprocessableEntity("request context exceeds the response size limit")
		}
		return nil, s.providerFailure("request_context", err)
	}
	out := &requestContextOutput{}
	out.Body.Entries = result.Entries
	if len(out.Body.Entries) > provider.MaxPageSize {
		out.Body.Entries = out.Body.Entries[:provider.MaxPageSize]
	}
	if err := s.validateResponseSize(out.Body, "request context exceeds the response size limit"); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) serviceHealth(ctx context.Context, input *serviceHealthInput) (*serviceHealthOutput, error) {
	p, err := s.findProfile(input.Body.ProfileID)
	if err != nil {
		return nil, err
	}
	if input.Body.Start.IsZero() || input.Body.End.IsZero() || !input.Body.Start.Before(input.Body.End) {
		return nil, huma.Error400BadRequest("start must be before end")
	}
	window := input.Body.End.Sub(input.Body.Start)
	if window > provider.MaxHealthWindow {
		return nil, huma.Error400BadRequest("time window cannot exceed seven days")
	}
	alignment := healthAlignment(window)
	result, err := s.cfg.Provider.ServiceHealth(ctx, provider.ServiceHealthRequest{
		ProjectID: p.ProjectID, Service: input.Body.Service, Start: input.Body.Start, End: input.Body.End, Alignment: alignment,
	})
	if err != nil {
		if errors.Is(err, provider.ErrResponseTooLarge) {
			return nil, huma.Error422UnprocessableEntity("service health response exceeds the size limit; use a shorter time window")
		}
		return nil, s.monitoringFailure("service_health", err)
	}
	out := &serviceHealthOutput{}
	out.Body.Service = result.Service
	out.Body.Start = result.Start.UTC()
	out.Body.End = result.End.UTC()
	out.Body.AlignmentSeconds = int64(result.Alignment / time.Second)
	out.Body.Series = result.Series
	if err := s.validateResponseSize(out.Body, "service health response exceeds the size limit; use a shorter time window"); err != nil {
		return nil, err
	}
	return out, nil
}

func healthAlignment(window time.Duration) time.Duration {
	buckets := time.Duration(provider.MaxHealthBuckets)
	alignment := window / buckets
	if window%buckets != 0 {
		alignment++
	}
	if alignment < time.Minute {
		return time.Minute
	}
	return ((alignment + time.Minute - 1) / time.Minute) * time.Minute
}

func (s *Server) validateResponseSize(body any, message string) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return s.internal("encode_response")
	}
	if len(encoded) > provider.MaxResponseBytes {
		return huma.Error422UnprocessableEntity(message)
	}
	return nil
}

func presentProfile(p profile.Profile, available bool) profileResponse {
	status := "needs-auth"
	if available {
		status = "ready"
	}
	return profileResponse{ID: p.ID, Name: p.Name, ProjectID: p.ProjectID, Provider: "gcp", Status: status}
}

func (s *Server) findProfile(id string) (profile.Profile, error) {
	profiles, err := s.cfg.Profiles.List()
	if err != nil {
		return profile.Profile{}, s.internal("read_profiles")
	}
	for _, p := range profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return profile.Profile{}, huma.Error404NotFound("connection not found")
}

func (s *Server) internal(operation string) error {
	// Operation names are fixed metadata; provider errors may contain sensitive request details.
	s.cfg.Logger.Error("request failed", "operation", operation)
	return huma.Error500InternalServerError("request could not be completed")
}

func (s *Server) providerFailure(operation string, err error) error {
	category := "unknown"
	statusCode := http.StatusInternalServerError
	message := "cloud request could not be completed"
	switch {
	case errors.Is(err, provider.ErrAuthentication):
		category = "authentication"
		statusCode = http.StatusFailedDependency
		message = "Google Cloud authentication is unavailable or expired. Refresh ADC with gcloud auth application-default login, or verify GOOGLE_APPLICATION_CREDENTIALS, then retry."
	case errors.Is(err, provider.ErrPermissionDenied):
		category = "permission_denied"
		statusCode = http.StatusForbidden
		message = "The active Google Cloud identity cannot read logs for this project. Grant roles/logging.viewer, then retry."
	case errors.Is(err, provider.ErrRateLimited):
		category = "rate_limited"
		statusCode = http.StatusTooManyRequests
		message = "Google Cloud temporarily rate-limited the request. Wait briefly, then retry."
	case errors.Is(err, provider.ErrUnavailable):
		category = "unavailable"
		statusCode = http.StatusServiceUnavailable
		message = "Google Cloud Logging is temporarily unavailable. Retry shortly."
	case errors.Is(err, provider.ErrInvalidQuery):
		category = "invalid_query"
		statusCode = http.StatusBadRequest
		message = "Cloud Logging rejected the query. Check the native filter syntax or adjust the selected filters."
	case errors.Is(err, provider.ErrConfiguration):
		category = "configuration"
		statusCode = http.StatusFailedDependency
		message = "Cloud Logging is unavailable for this project. Verify the project ID and that the Cloud Logging API is enabled."
	}
	s.cfg.Logger.Error("provider request failed", "operation", operation, "category", category)
	return huma.NewError(statusCode, message)
}

func (s *Server) monitoringFailure(operation string, err error) error {
	category := "unknown"
	statusCode := http.StatusInternalServerError
	message := "service health could not be loaded"
	switch {
	case errors.Is(err, provider.ErrAuthentication):
		category = "authentication"
		statusCode = http.StatusFailedDependency
		message = "Google Cloud authentication is unavailable or expired. Refresh ADC with gcloud auth application-default login, or verify GOOGLE_APPLICATION_CREDENTIALS, then retry."
	case errors.Is(err, provider.ErrPermissionDenied):
		category = "permission_denied"
		statusCode = http.StatusForbidden
		message = "The active Google Cloud identity cannot read metrics for this project. Grant roles/monitoring.viewer, then retry."
	case errors.Is(err, provider.ErrRateLimited):
		category = "rate_limited"
		statusCode = http.StatusTooManyRequests
		message = "Google Cloud temporarily rate-limited the metrics request. Wait briefly, then retry."
	case errors.Is(err, provider.ErrUnavailable):
		category = "unavailable"
		statusCode = http.StatusServiceUnavailable
		message = "Cloud Monitoring is temporarily unavailable. Retry shortly."
	case errors.Is(err, provider.ErrConfiguration):
		category = "configuration"
		statusCode = http.StatusFailedDependency
		message = "Cloud Monitoring is unavailable for this project. Verify the project ID and that the Cloud Monitoring API is enabled."
	case errors.Is(err, provider.ErrInvalidQuery):
		category = "invalid_fixed_query"
	}
	s.cfg.Logger.Error("provider request failed", "operation", operation, "category", category)
	return huma.NewError(statusCode, message)
}

type requestContextKey struct{}

func requestFromContext(ctx context.Context) (*http.Request, bool) {
	r, ok := ctx.Value(requestContextKey{}).(*http.Request)
	return r, ok
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/api/v1/session/pair" {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestContextKey{}, r)))
			return
		}
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil || !s.cfg.Sessions.Valid(cookie.Value) {
			writeProblem(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestContextKey{}, r)))
	})
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w.Header())
		if r.Host != s.cfg.Host {
			writeProblem(w, http.StatusMisdirectedRequest, "invalid host")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && origin != s.cfg.Origin {
			writeProblem(w, http.StatusForbidden, "invalid origin")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.Method != http.MethodGet && r.Method != http.MethodHead && origin != s.cfg.Origin {
			writeProblem(w, http.StatusForbidden, "origin is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cache-Control", "no-store")
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"title": http.StatusText(status), "status": status, "detail": detail})
}

func spaHandler(assets fs.FS) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(assets, path); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(assets, "index.html"); err == nil {
			http.ServeFileFS(w, r, assets, "index.html")
			return
		}
		http.Error(w, "frontend is served by the development server", http.StatusNotFound)
	})
}
