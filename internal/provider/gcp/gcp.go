package gcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"github.com/jamoowen/log-leopard/internal/provider"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type Provider struct {
	discoverServices func(context.Context, string) ([]provider.Service, error)
	serviceHealth    func(context.Context, provider.ServiceHealthRequest) (provider.ServiceHealthResult, error)
	fleetOverview    func(context.Context, provider.FleetOverviewRequest) (provider.FleetOverviewResult, error)
}

func New() *Provider {
	return &Provider{discoverServices: discoverServices, serviceHealth: queryServiceHealth, fleetOverview: queryFleetOverview}
}

func (*Provider) ADCStatus(ctx context.Context) (bool, string) {
	_, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform.read-only")
	if err != nil {
		return false, "Application Default Credentials are unavailable. Run gcloud auth application-default login."
	}
	return true, "Application Default Credentials are available."
}

func (p *Provider) Discover(ctx context.Context, projectID string) provider.Discovery {
	discovery := provider.Discovery{Services: []provider.Service{{ID: "", Name: "All logs"}}}
	services, err := p.discoverServices(ctx, projectID)
	if err != nil {
		if errors.Is(err, provider.ErrAuthentication) {
			discovery.Warning = "Google Cloud authentication is unavailable or expired. Refresh ADC with gcloud auth application-default login, or verify GOOGLE_APPLICATION_CREDENTIALS, then retry."
			discovery.WarningCode = provider.DiscoveryWarningAuthentication
		} else {
			discovery.Warning = "Cloud Run discovery is unavailable; manual and all-logs queries still work."
		}
		return discovery
	}
	if len(services) > provider.MaxDiscoveredServices {
		services = services[:provider.MaxDiscoveredServices]
		discovery.Warning = "Cloud Run discovery is limited to 100 unique service names; additional services were omitted."
	}
	discovery.Services = append(discovery.Services, services...)
	return discovery
}

func discoverServices(ctx context.Context, projectID string) ([]provider.Service, error) {
	client, err := run.NewServicesClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create Cloud Run client: %w", err)
	}
	defer func() { _ = client.Close() }()
	resourceNames := make([]string, 0, provider.MaxDiscoveredServices+1)
	seen := make(map[string]struct{}, provider.MaxDiscoveredServices+1)
	it := client.ListServices(ctx, &runpb.ListServicesRequest{Parent: "projects/" + projectID + "/locations/-"})
	for {
		service, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list Cloud Run services: %w", classifyError(err))
		}
		resourceName := service.GetName()
		name := resourceName
		if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
			name = name[slash+1:]
		}
		if name == "" {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		resourceNames = append(resourceNames, resourceName)
		if len(resourceNames) > provider.MaxDiscoveredServices {
			break
		}
	}
	return normalizeServiceNames(resourceNames), nil
}

func normalizeServiceNames(resourceNames []string) []provider.Service {
	names := make(map[string]struct{}, len(resourceNames))
	for _, resourceName := range resourceNames {
		name := resourceName
		if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
			name = name[slash+1:]
		}
		if name != "" {
			names[name] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	slices.Sort(ordered)
	services := make([]provider.Service, 0, len(ordered))
	for _, name := range ordered {
		services = append(services, provider.Service{ID: name, Name: name})
	}
	return services
}

func (*Provider) Query(ctx context.Context, req provider.QueryRequest) (provider.QueryResult, error) {
	client, err := logging.NewClient(ctx)
	if err != nil {
		return provider.QueryResult{}, fmt.Errorf("create logging client: %w", err)
	}
	defer func() { _ = client.Close() }()
	it := client.ListLogEntries(ctx, &loggingpb.ListLogEntriesRequest{
		ResourceNames: []string{"projects/" + req.ProjectID}, Filter: req.Filter,
		OrderBy: loggingOrder(req.Order), PageSize: int32(req.PageSize), PageToken: req.PageToken,
	})
	page := make([]*loggingpb.LogEntry, 0, req.PageSize)
	next, err := iterator.NewPager(it, req.PageSize, req.PageToken).NextPage(&page)
	if err != nil {
		return provider.QueryResult{}, fmt.Errorf("list log entries: %w", classifyError(err))
	}
	result := provider.QueryResult{Entries: make([]provider.Entry, 0, len(page)), NextPageToken: next}
	totalBytes := 0
	for _, item := range page {
		entry, size, err := normalize(item)
		if err != nil {
			return provider.QueryResult{}, fmt.Errorf("normalize log entry: %w", err)
		}
		totalBytes += size
		if totalBytes > provider.MaxResponseBytes {
			return provider.QueryResult{}, provider.ErrResponseTooLarge
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

func (p *Provider) ServiceHealth(ctx context.Context, req provider.ServiceHealthRequest) (provider.ServiceHealthResult, error) {
	if err := validateHealthRequest(req); err != nil {
		return provider.ServiceHealthResult{}, err
	}
	return p.serviceHealth(ctx, req)
}

func (p *Provider) FleetOverview(ctx context.Context, req provider.FleetOverviewRequest) (provider.FleetOverviewResult, error) {
	if err := validateFleetOverviewRequest(req); err != nil {
		return provider.FleetOverviewResult{}, err
	}
	return p.fleetOverview(ctx, req)
}

func classifyError(err error) error {
	var category error
	switch status.Code(err) {
	case codes.Unauthenticated:
		category = provider.ErrAuthentication
	case codes.PermissionDenied:
		category = provider.ErrPermissionDenied
	case codes.ResourceExhausted:
		category = provider.ErrRateLimited
	case codes.Unavailable, codes.DeadlineExceeded:
		category = provider.ErrUnavailable
	case codes.InvalidArgument:
		category = provider.ErrInvalidQuery
	case codes.FailedPrecondition, codes.NotFound:
		category = provider.ErrConfiguration
	default:
		return err
	}
	return category
}

func loggingOrder(order provider.Order) string {
	if order == provider.OrderAscending {
		return "timestamp asc"
	}
	return "timestamp desc"
}

func normalize(item *loggingpb.LogEntry) (provider.Entry, int, error) {
	raw, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(item)
	if err != nil {
		return provider.Entry{}, 0, err
	}
	message, messageSource := "", ""
	var structured map[string]any
	if payload := item.GetJsonPayload(); payload != nil {
		structured = payload.AsMap()
	}
	for _, candidate := range []struct {
		path  string
		value any
	}{
		{path: "jsonPayload.message", value: structured["message"]},
		{path: "jsonPayload.msg", value: structured["msg"]},
		{path: "jsonPayload.error.message", value: nestedValue(structured, "error", "message")},
		{path: "jsonPayload.exception.message", value: nestedValue(structured, "exception", "message")},
	} {
		if value, ok := nonEmptyString(candidate.value); ok {
			message, messageSource = value, candidate.path
			break
		}
	}
	if message == "" {
		if text := item.GetTextPayload(); text != "" {
			message, messageSource = text, "textPayload"
		} else if structured != nil {
			compact, marshalErr := json.Marshal(structured)
			if marshalErr != nil {
				return provider.Entry{}, 0, marshalErr
			}
			message, messageSource = string(compact), "jsonPayload"
		}
	}
	if message == "" {
		if payload := item.GetProtoPayload(); payload != nil {
			message, messageSource = payload.GetTypeUrl(), "protoPayload"
		}
	}
	source := ""
	if resource := item.GetResource(); resource != nil {
		source = resource.GetLabels()["service_name"]
		if source == "" {
			source = resource.GetType()
		}
	}
	if value, ok := firstString(structured, []string{"service"}, []string{"service_name"}, []string{"service", "name"}); source == "" && ok {
		source = value
	}
	severityOriginal := item.GetSeverity().String()
	severity := severityOriginal
	if severityOriginal == "DEFAULT" {
		if level, ok := firstString(structured, []string{"level"}); ok {
			severityOriginal = level
			severity = normalizeSeverity(level)
		}
	}
	trace := item.GetTrace()
	if value, ok := firstString(structured, []string{"trace"}, []string{"logging.googleapis.com/trace"}); trace == "" && ok {
		trace = value
	}
	requestID, _ := firstString(structured,
		[]string{"requestId"}, []string{"request_id"}, []string{"request", "id"}, []string{"httpRequest", "requestId"})
	sum := sha256.Sum256(raw)
	id := fmt.Sprintf("gcp-%x", sum[:16])
	labels := item.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	receiveTimestamp := time.Time{}
	if timestamp := item.GetReceiveTimestamp(); timestamp != nil {
		receiveTimestamp = timestamp.AsTime()
	}
	return provider.Entry{
		ID:               id,
		Timestamp:        item.GetTimestamp().AsTime(),
		ReceiveTimestamp: receiveTimestamp,
		Severity:         severity,
		SeverityOriginal: severityOriginal,
		Message:          message,
		MessageSource:    messageSource,
		Source:           source,
		SourceOriginal:   item.GetLogName(),
		RequestID:        requestID,
		Trace:            trace,
		SpanID:           item.GetSpanId(),
		HTTPRequest:      normalizeHTTPRequest(item, structured),
		Labels:           labels,
		Structured:       structured,
		Raw:              raw,
	}, len(raw), nil
}

func normalizeSeverity(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "TRACE":
		return "DEBUG"
	case "WARN", "WARNING":
		return "WARNING"
	case "FATAL", "PANIC":
		return "CRITICAL"
	case "DEFAULT", "DEBUG", "INFO", "NOTICE", "ERROR", "CRITICAL", "ALERT", "EMERGENCY":
		return normalized
	default:
		return "DEFAULT"
	}
}

func normalizeHTTPRequest(item *loggingpb.LogEntry, structured map[string]any) *provider.HTTPRequest {
	request := item.GetHttpRequest()
	if request != nil {
		result := &provider.HTTPRequest{
			Method: request.GetRequestMethod(), URL: request.GetRequestUrl(), Status: int(request.GetStatus()),
			RemoteIP: request.GetRemoteIp(), UserAgent: request.GetUserAgent(), Referer: request.GetReferer(), Protocol: request.GetProtocol(),
		}
		if request.GetLatency() != nil {
			result.Latency = request.GetLatency().AsDuration().String()
		}
		return result
	}
	httpPayload, ok := nestedMap(structured, "httpRequest")
	if !ok {
		return nil
	}
	result := &provider.HTTPRequest{}
	result.Method, _ = firstString(httpPayload, []string{"requestMethod"}, []string{"method"})
	result.URL, _ = firstString(httpPayload, []string{"requestUrl"}, []string{"url"})
	result.RemoteIP, _ = firstString(httpPayload, []string{"remoteIp"}, []string{"remoteIP"})
	result.UserAgent, _ = firstString(httpPayload, []string{"userAgent"})
	result.Referer, _ = firstString(httpPayload, []string{"referer"}, []string{"referrer"})
	result.Protocol, _ = firstString(httpPayload, []string{"protocol"})
	result.Latency, _ = firstString(httpPayload, []string{"latency"})
	if status, ok := httpPayload["status"].(float64); ok {
		result.Status = int(status)
	}
	return result
}

func nonEmptyString(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok && strings.TrimSpace(text) != ""
}

func nestedMap(object map[string]any, key string) (map[string]any, bool) {
	if object == nil {
		return nil, false
	}
	value, ok := object[key].(map[string]any)
	return value, ok
}

func nestedValue(object map[string]any, path ...string) any {
	var value any = object
	for _, part := range path {
		current, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = current[part]
	}
	return value
}

func firstString(object map[string]any, paths ...[]string) (string, bool) {
	for _, path := range paths {
		if value, ok := nonEmptyString(nestedValue(object, path...)); ok {
			return value, true
		}
	}
	return "", false
}
