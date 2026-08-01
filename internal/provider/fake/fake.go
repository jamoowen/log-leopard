package fake

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jamoowen/log-leopard/internal/provider"
)

type Provider struct{}

var errInvalidPageToken = errors.New("invalid fake page token")

func New() *Provider { return &Provider{} }

func (*Provider) ADCStatus(context.Context) (bool, string) { return true, "Fake provider enabled" }

func (*Provider) Discover(context.Context, string) provider.Discovery {
	return provider.Discovery{Services: []provider.Service{
		{ID: "", Name: "All logs"},
		{ID: "checkout-api", Name: "checkout-api"},
		{ID: "worker", Name: "worker"},
	}}
}

func (*Provider) Query(_ context.Context, req provider.QueryRequest) (provider.QueryResult, error) {
	start := 0
	if req.PageToken != "" {
		b, err := base64.RawURLEncoding.DecodeString(req.PageToken)
		if err != nil {
			return provider.QueryResult{}, errInvalidPageToken
		}
		start, err = strconv.Atoi(string(b))
		if err != nil {
			return provider.QueryResult{}, errInvalidPageToken
		}
	}
	fixtures := entries()
	if req.Order == provider.OrderAscending {
		slices.Reverse(fixtures)
	}
	end := min(start+req.PageSize, len(fixtures))
	if start < 0 || start > end {
		return provider.QueryResult{}, errInvalidPageToken
	}
	next := ""
	if end < len(fixtures) {
		next = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(end)))
	}
	return provider.QueryResult{Entries: fixtures[start:end], NextPageToken: next}, nil
}

func entries() []provider.Entry {
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	items := []struct {
		severity, message, source string
	}{
		{"ERROR", "payment authorization timed out", "checkout-api"},
		{"WARNING", "retrying request after upstream reset", "checkout-api"},
		{"INFO", "scheduled export completed", "worker"},
		{"DEBUG", "cache lookup completed", "worker"},
	}
	out := make([]provider.Entry, 0, len(items))
	for i, item := range items {
		ts := base.Add(-time.Duration(i) * time.Minute)
		raw, _ := json.Marshal(map[string]any{
			"insertId": fmt.Sprintf("synthetic-%d", i+1), "timestamp": ts.Format(time.RFC3339Nano),
			"severity": item.severity, "textPayload": item.message,
			"resource": map[string]any{"type": "cloud_run_revision", "labels": map[string]string{"service_name": item.source}},
		})
		out = append(out, provider.Entry{
			ID: fmt.Sprintf("synthetic-%d", i+1), Timestamp: ts, Severity: normalizeSeverity(item.severity),
			SeverityOriginal: item.severity, Message: item.message, MessageSource: "textPayload",
			Source: item.source, SourceOriginal: "projects/synthetic-project/logs/run.googleapis.com%2Fstdout",
			RequestID: "synthetic-request-123", Trace: "projects/synthetic-project/traces/synthetic-trace-123",
			Labels: map[string]string{"fixture": "synthetic"}, Structured: nil, Raw: raw,
		})
	}
	return out
}

func normalizeSeverity(s string) string {
	switch s {
	case "DEFAULT", "DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL", "ALERT", "EMERGENCY":
		return s
	default:
		return "DEFAULT"
	}
}
