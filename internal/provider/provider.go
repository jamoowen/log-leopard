package provider

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	MaxPageSize           = 200
	MaxResponseBytes      = 4 << 20
	MaxDiscoveredServices = 100
	MaxHealthBuckets      = 300
	MaxHealthWindow       = 7 * 24 * time.Hour
	MaxFleetServices      = 20
)

var (
	ErrAuthentication   = errors.New("cloud provider authentication failed")
	ErrPermissionDenied = errors.New("cloud provider permission denied")
	ErrRateLimited      = errors.New("cloud provider rate limited")
	ErrUnavailable      = errors.New("cloud provider unavailable")
	ErrInvalidQuery     = errors.New("cloud provider rejected query")
	ErrConfiguration    = errors.New("cloud provider configuration failed")
	ErrResponseTooLarge = errors.New("provider response exceeds response byte limit")
)

type Service struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DiscoveryWarningCode string

const DiscoveryWarningAuthentication DiscoveryWarningCode = "authentication"

type Discovery struct {
	Services    []Service            `json:"services"`
	Warning     string               `json:"warning,omitempty"`
	WarningCode DiscoveryWarningCode `json:"warningCode,omitempty"`
}

type HTTPRequest struct {
	Method    string `json:"method,omitempty" doc:"HTTP request method."`
	URL       string `json:"url,omitempty" doc:"Requested URL."`
	Status    int    `json:"status,omitempty" doc:"HTTP response status code."`
	Latency   string `json:"latency,omitempty" doc:"Request latency as a protobuf duration."`
	RemoteIP  string `json:"remoteIp,omitempty" doc:"Originating client IP as reported by Cloud Logging."`
	UserAgent string `json:"userAgent,omitempty" doc:"HTTP user agent."`
	Referer   string `json:"referer,omitempty" doc:"HTTP referrer."`
	Protocol  string `json:"protocol,omitempty" doc:"HTTP protocol used for the request."`
}

type Entry struct {
	ID               string            `json:"id" doc:"Stable provider-derived identity for this exact log entry."`
	Timestamp        time.Time         `json:"timestamp" doc:"Event timestamp."`
	ReceiveTimestamp time.Time         `json:"receiveTimestamp,omitzero" doc:"Cloud Logging receive timestamp."`
	Severity         string            `json:"severity" doc:"Normalized GCP severity name."`
	SeverityOriginal string            `json:"severityOriginal" doc:"Original top-level GCP severity, or original JSON level when it supplies the normalized severity."`
	Message          string            `json:"message" doc:"Display message selected from the structured or text payload."`
	MessageSource    string            `json:"messageSource" doc:"Exact payload path selected for message."`
	Source           string            `json:"source" doc:"Normalized service or monitored resource name."`
	SourceOriginal   string            `json:"sourceOriginal" doc:"Original GCP log name."`
	RequestID        string            `json:"requestId,omitempty" doc:"Request ID found in a common structured payload location."`
	Trace            string            `json:"trace,omitempty" doc:"Trace resource name or trace identifier."`
	SpanID           string            `json:"spanId,omitempty" doc:"Cloud Trace span ID."`
	HTTPRequest      *HTTPRequest      `json:"httpRequest,omitempty" doc:"Common HTTP request metadata."`
	Labels           map[string]string `json:"labels" doc:"GCP log entry labels."`
	Structured       map[string]any    `json:"structured" nullable:"true" doc:"Decoded jsonPayload, or null for non-JSON entries."`
	Raw              json.RawMessage   `json:"raw" doc:"Original GCP LogEntry JSON."`
}

type QueryRequest struct {
	ProjectID string
	Filter    string
	PageSize  int
	PageToken string
	Order     Order
}

type Order string

const (
	OrderDescending Order = "descending"
	OrderAscending  Order = "ascending"
)

type QueryResult struct {
	Entries       []Entry
	NextPageToken string
}

type HealthSeriesName string

const (
	HealthRequestCount      HealthSeriesName = "request_count"
	HealthServerErrorCount  HealthSeriesName = "server_error_count"
	HealthRequestLatencyP95 HealthSeriesName = "request_latency_p95"
)

type HealthPoint struct {
	Timestamp time.Time `json:"timestamp" doc:"End of the aligned metric interval."`
	Value     float64   `json:"value" doc:"Observed metric value during the aligned interval, in the series unit."`
}

type HealthSeries struct {
	Name   HealthSeriesName `json:"name" enum:"request_count,server_error_count,request_latency_p95" doc:"Fixed semantic metric series."`
	Unit   string           `json:"unit" enum:"1,ms" doc:"UCUM metric unit; 1 denotes a count and ms denotes milliseconds."`
	Points []HealthPoint    `json:"points" doc:"Observed points in ascending timestamp order; absent intervals are not fabricated."`
}

type ServiceHealthRequest struct {
	ProjectID string
	Service   string
	Start     time.Time
	End       time.Time
	Alignment time.Duration
}

type ServiceHealthResult struct {
	Service   string
	Start     time.Time
	End       time.Time
	Alignment time.Duration
	Series    []HealthSeries
}

type FleetOverviewRequest struct {
	ProjectID string
	Services  []string
	Start     time.Time
	End       time.Time
}

type FleetOverviewSummary struct {
	Service             string   `json:"service" doc:"Requested Cloud Run service name."`
	RequestCount        float64  `json:"requestCount" minimum:"0" doc:"Total requests observed in the window."`
	ServerErrorCount    float64  `json:"serverErrorCount" minimum:"0" doc:"Total 5xx requests observed in the window."`
	RequestLatencyP95Ms *float64 `json:"requestLatencyP95Ms" nullable:"true" minimum:"0" doc:"Merged p95 request latency in milliseconds, or null when no latency data is available."`
}

type FleetOverviewResult struct {
	Start    time.Time
	End      time.Time
	Services []FleetOverviewSummary
}
