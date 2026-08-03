package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging/apiv2/loggingpb"
	"github.com/jamoowen/log-leopard/internal/provider"
	monitoredres "google.golang.org/genproto/googleapis/api/monitoredres"
	logtype "google.golang.org/genproto/googleapis/logging/type"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		code codes.Code
		want error
	}{
		{code: codes.Unauthenticated, want: provider.ErrAuthentication},
		{code: codes.PermissionDenied, want: provider.ErrPermissionDenied},
		{code: codes.ResourceExhausted, want: provider.ErrRateLimited},
		{code: codes.Unavailable, want: provider.ErrUnavailable},
		{code: codes.InvalidArgument, want: provider.ErrInvalidQuery},
		{code: codes.FailedPrecondition, want: provider.ErrConfiguration},
	}
	for _, test := range tests {
		t.Run(test.code.String(), func(t *testing.T) {
			err := classifyError(status.Error(test.code, "sensitive provider detail"))
			if !errors.Is(err, test.want) {
				t.Fatalf("classifyError() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestLoggingOrderDefaultsNewestFirst(t *testing.T) {
	if got := loggingOrder(""); got != "timestamp desc" {
		t.Fatalf("default order = %q", got)
	}
	if got := loggingOrder(provider.OrderDescending); got != "timestamp desc" {
		t.Fatalf("descending order = %q", got)
	}
	if got := loggingOrder(provider.OrderAscending); got != "timestamp asc" {
		t.Fatalf("ascending order = %q", got)
	}
}

func TestNormalizePreservesOriginalsAndRaw(t *testing.T) {
	payload, _ := structpb.NewStruct(map[string]any{"message": "synthetic message", "nested": map[string]any{"value": 42.0}})
	ts := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	entry, _, err := normalize(&loggingpb.LogEntry{
		LogName: "projects/synthetic/logs/run.googleapis.com%2Fstdout", InsertId: "fixture-1",
		Timestamp: timestamppb.New(ts), Severity: logtype.LogSeverity_WARNING,
		Resource: &monitoredres.MonitoredResource{Type: "cloud_run_revision", Labels: map[string]string{"service_name": "api"}},
		Payload:  &loggingpb.LogEntry_JsonPayload{JsonPayload: payload},
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Message != "synthetic message" || entry.MessageSource != "jsonPayload.message" || entry.SeverityOriginal != "WARNING" || entry.Source != "api" {
		t.Fatalf("unexpected normalization: %#v", entry)
	}
	if len(entry.Raw) == 0 {
		t.Fatal("raw entry is empty")
	}
}

func TestDiscoverDistinguishesEmptySuccessFromFailure(t *testing.T) {
	manyServices := make([]provider.Service, provider.MaxDiscoveredServices+1)
	for i := range manyServices {
		name := fmt.Sprintf("service-%03d", i)
		manyServices[i] = provider.Service{ID: name, Name: name}
	}
	tests := []struct {
		name        string
		services    []provider.Service
		err         error
		wantWarning bool
		wantCount   int
	}{
		{name: "no deployed services", services: []provider.Service{}, wantCount: 1},
		{name: "services discovered", services: []provider.Service{{ID: "api", Name: "api"}}, wantCount: 2},
		{name: "services limited", services: manyServices, wantWarning: true, wantCount: provider.MaxDiscoveredServices + 1},
		{name: "discovery failed", err: errors.New("permission denied for secret project"), wantWarning: true, wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{discoverServices: func(context.Context, string) ([]provider.Service, error) {
				return tt.services, tt.err
			}}
			got := p.Discover(context.Background(), "synthetic-project")
			if len(got.Services) != tt.wantCount || got.Services[0].ID != "" || got.Services[0].Name != "All logs" {
				t.Fatalf("unexpected explicit options: %#v", got.Services)
			}
			if (got.Warning != "") != tt.wantWarning {
				t.Fatalf("warning = %q", got.Warning)
			}
			if strings.Contains(got.Warning, "secret project") {
				t.Fatalf("discovery error leaked through warning: %q", got.Warning)
			}
		})
	}
}

func TestDiscoverExplainsAuthenticationFailure(t *testing.T) {
	p := &Provider{discoverServices: func(context.Context, string) ([]provider.Service, error) {
		return nil, provider.ErrAuthentication
	}}

	got := p.Discover(context.Background(), "synthetic-project")
	want := "Google Cloud authentication is unavailable or expired. Refresh ADC with gcloud auth application-default login, or verify GOOGLE_APPLICATION_CREDENTIALS, then retry."
	if got.Warning != want {
		t.Fatalf("warning = %q, want %q", got.Warning, want)
	}
	if got.WarningCode != provider.DiscoveryWarningAuthentication {
		t.Fatalf("warning code = %q, want %q", got.WarningCode, provider.DiscoveryWarningAuthentication)
	}
}

func TestNormalizeServiceNamesDeduplicatesRegionsAndSorts(t *testing.T) {
	got := normalizeServiceNames([]string{
		"projects/synthetic/locations/us-central1/services/worker",
		"projects/synthetic/locations/europe-west1/services/api",
		"projects/synthetic/locations/europe-west1/services/worker",
		"",
	})
	want := []provider.Service{{ID: "api", Name: "api"}, {ID: "worker", Name: "worker"}}
	if !slices.Equal(got, want) {
		t.Fatalf("services = %#v, want %#v", got, want)
	}
}

func TestNormalizeAbsentLabelsAsEmptyMap(t *testing.T) {
	entry, _, err := normalize(&loggingpb.LogEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Labels == nil || len(entry.Labels) != 0 {
		t.Fatalf("labels = %#v, want non-nil empty map", entry.Labels)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"labels":{}`) {
		t.Fatalf("labels did not serialize as an object: %s", encoded)
	}
}

func TestNormalizeMessagePrecedenceAndTypedValues(t *testing.T) {
	tests := []struct {
		name, text, wantMessage, wantSource string
		payload                             map[string]any
	}{
		{name: "message", text: "text", payload: map[string]any{"message": "primary", "msg": "secondary"}, wantMessage: "primary", wantSource: "jsonPayload.message"},
		{name: "msg after wrong message type", text: "text", payload: map[string]any{"message": 42.0, "msg": "secondary"}, wantMessage: "secondary", wantSource: "jsonPayload.msg"},
		{name: "nested error", text: "text", payload: map[string]any{"message": " ", "error": map[string]any{"message": "nested"}}, wantMessage: "nested", wantSource: "jsonPayload.error.message"},
		{name: "text payload", text: "plain", payload: map[string]any{}, wantMessage: "plain", wantSource: "textPayload"},
		{name: "compact fallback", payload: map[string]any{"code": 7.0}, wantMessage: `{"code":7}`, wantSource: "jsonPayload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := structpb.NewStruct(tt.payload)
			if err != nil {
				t.Fatal(err)
			}
			item := &loggingpb.LogEntry{Payload: &loggingpb.LogEntry_JsonPayload{JsonPayload: payload}}
			// LogEntry payloads are a oneof, so text is exercised independently.
			if tt.wantSource == "textPayload" {
				item.Payload = &loggingpb.LogEntry_TextPayload{TextPayload: tt.text}
			}
			entry, _, err := normalize(item)
			if err != nil {
				t.Fatal(err)
			}
			if entry.Message != tt.wantMessage || entry.MessageSource != tt.wantSource {
				t.Fatalf("got message %q from %q", entry.Message, entry.MessageSource)
			}
		})
	}
}

func TestNormalizeSlogSeverityAndRequestMetadata(t *testing.T) {
	payload, _ := structpb.NewStruct(map[string]any{
		"level": "WARN", "msg": "slow request", "requestId": "request-123",
		"logging.googleapis.com/trace": "projects/synthetic/traces/trace-123",
	})
	entry, _, err := normalize(&loggingpb.LogEntry{
		Severity: logtype.LogSeverity_DEFAULT,
		Payload:  &loggingpb.LogEntry_JsonPayload{JsonPayload: payload},
		HttpRequest: &logtype.HttpRequest{
			RequestMethod: "GET", RequestUrl: "https://example.invalid/path", Status: 503,
			Latency: durationpb.New(1500 * time.Millisecond), RemoteIp: "192.0.2.1", UserAgent: "synthetic-agent", Protocol: "HTTP/2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Severity != "WARNING" || entry.SeverityOriginal != "WARN" {
		t.Fatalf("severity was not normalized while preserving the original: %#v", entry)
	}
	if entry.RequestID != "request-123" || !strings.HasSuffix(entry.Trace, "/trace-123") {
		t.Fatalf("request identifiers not exposed: %#v", entry)
	}
	if entry.HTTPRequest == nil || entry.HTTPRequest.Method != "GET" || entry.HTTPRequest.Status != 503 || entry.HTTPRequest.Latency != "1.5s" {
		t.Fatalf("HTTP metadata not exposed: %#v", entry.HTTPRequest)
	}
}

func TestNormalizeGeneratesStableCollisionSafeID(t *testing.T) {
	item := &loggingpb.LogEntry{
		InsertId:  "shared-insert-id",
		LogName:   "projects/synthetic/logs/stdout",
		Timestamp: timestamppb.New(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)),
		Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: "message"},
	}

	first, _, err := normalize(item)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := normalize(item)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || !strings.HasPrefix(first.ID, "gcp-") {
		t.Fatalf("expected a stable generated ID, got %q and %q", first.ID, second.ID)
	}
	item.LogName = "projects/synthetic/logs/stderr"
	collision, _, err := normalize(item)
	if err != nil {
		t.Fatal(err)
	}
	if collision.ID == first.ID {
		t.Fatal("entries with the same insert ID but different log names collided")
	}
}

func TestNormalizeOmitsMissingReceiveTimestamp(t *testing.T) {
	entry, _, err := normalize(&loggingpb.LogEntry{
		Timestamp: timestamppb.New(time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)),
		Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: "message"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if _, exists := object["receiveTimestamp"]; exists {
		t.Fatalf("missing receive timestamp was serialized: %s", encoded)
	}
}
