package gcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/jamoowen/log-leopard/internal/provider"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredres "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRequestCountQueryIsFixedAndBounded(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := provider.ServiceHealthRequest{
		ProjectID: "synthetic-project-123", Service: "checkout-api", Start: start, End: start.Add(time.Hour), Alignment: time.Minute,
	}
	got := requestCountQuery(req)
	if got.GetName() != "projects/synthetic-project-123" {
		t.Fatalf("name = %q", got.GetName())
	}
	for _, want := range []string{`metric.type = "run.googleapis.com/request_count"`, `resource.type = "cloud_run_revision"`, `resource.labels.project_id = "synthetic-project-123"`, `resource.labels.service_name = "checkout-api"`} {
		if !strings.Contains(got.GetFilter(), want) {
			t.Errorf("filter %q is missing %q", got.GetFilter(), want)
		}
	}
	aggregation := got.GetAggregation()
	if aggregation.GetAlignmentPeriod().AsDuration() != time.Minute || aggregation.GetPerSeriesAligner() != monitoringpb.Aggregation_ALIGN_SUM || aggregation.GetCrossSeriesReducer() != monitoringpb.Aggregation_REDUCE_SUM {
		t.Fatalf("unexpected aggregation: %#v", aggregation)
	}
	if len(aggregation.GetGroupByFields()) != 1 || aggregation.GetGroupByFields()[0] != "metric.labels.response_code_class" {
		t.Fatalf("unexpected grouping: %#v", aggregation.GetGroupByFields())
	}
	if got.GetView() != monitoringpb.ListTimeSeriesRequest_FULL || got.GetPageSize() != maxRawHealthPoints {
		t.Fatalf("unexpected response bounds: view=%v page=%d", got.GetView(), got.GetPageSize())
	}
}

func TestRequestLatencyP95QueryMergesDistributionsBeforePercentile(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := provider.ServiceHealthRequest{
		ProjectID: "synthetic-project-123", Service: "checkout-api", Start: start, End: start.Add(time.Hour), Alignment: time.Minute,
	}
	got := requestLatencyP95Query(req)
	for _, want := range []string{`metric.type = "run.googleapis.com/request_latencies"`, `resource.labels.project_id = "synthetic-project-123"`, `resource.labels.service_name = "checkout-api"`} {
		if !strings.Contains(got.GetFilter(), want) {
			t.Errorf("filter %q is missing %q", got.GetFilter(), want)
		}
	}
	primary := got.GetAggregation()
	if primary.GetPerSeriesAligner() != monitoringpb.Aggregation_ALIGN_SUM || primary.GetCrossSeriesReducer() != monitoringpb.Aggregation_REDUCE_SUM || len(primary.GetGroupByFields()) != 0 {
		t.Fatalf("latency distributions are not merged before percentile conversion: %#v", primary)
	}
	secondary := got.GetSecondaryAggregation()
	if secondary.GetPerSeriesAligner() != monitoringpb.Aggregation_ALIGN_PERCENTILE_95 || secondary.GetCrossSeriesReducer() != monitoringpb.Aggregation_REDUCE_NONE {
		t.Fatalf("unexpected latency percentile aggregation: %#v", secondary)
	}
}

func TestServiceHealthRejectsInvalidRequestsBeforeProviderCall(t *testing.T) {
	called := false
	p := &Provider{serviceHealth: func(context.Context, provider.ServiceHealthRequest) (provider.ServiceHealthResult, error) {
		called = true
		return provider.ServiceHealthResult{}, nil
	}}
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for _, service := range []string{"", "UPPER", "trailing-", `api" OR true`, strings.Repeat("a", 50)} {
		_, err := p.ServiceHealth(context.Background(), provider.ServiceHealthRequest{
			ProjectID: "synthetic-project", Service: service, Start: start, End: start.Add(time.Hour), Alignment: time.Minute,
		})
		if !errors.Is(err, provider.ErrConfiguration) {
			t.Errorf("service %q error = %v", service, err)
		}
	}
	if called {
		t.Fatal("provider function was called for invalid input")
	}
}

func TestNormalizeRequestCountsOrdersAndCombinesClasses(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	point := func(end time.Time, value int64) *monitoringpb.Point {
		return &monitoringpb.Point{
			Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(end)},
			Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{Int64Value: value}},
		}
	}
	series := func(class string, points ...*monitoringpb.Point) *monitoringpb.TimeSeries {
		return &monitoringpb.TimeSeries{Metric: &metricpb.Metric{Labels: map[string]string{"response_code_class": class}}, Points: points}
	}
	result, err := normalizeRequestCounts(provider.ServiceHealthRequest{
		Service: "api", Start: start, End: start.Add(3 * time.Minute), Alignment: time.Minute,
	}, []*monitoringpb.TimeSeries{
		series("2xx", point(start.Add(2*time.Minute), 20), point(start.Add(time.Minute), 10)),
		series("5xx", point(start.Add(2*time.Minute), 3)),
		series("4xx", point(start.Add(time.Minute), 2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 2 || result.Series[0].Name != provider.HealthRequestCount || result.Series[1].Name != provider.HealthServerErrorCount {
		t.Fatalf("unexpected fixed series: %#v", result.Series)
	}
	requests, serverErrors := result.Series[0].Points, result.Series[1].Points
	if len(requests) != 2 || !requests[0].Timestamp.Before(requests[1].Timestamp) || requests[0].Value != 12 || requests[1].Value != 23 {
		t.Fatalf("unexpected requests: %#v", requests)
	}
	if serverErrors[0].Value != 0 || serverErrors[1].Value != 3 {
		t.Fatalf("unexpected server errors: %#v", serverErrors)
	}
}

func TestNormalizeRequestCountsRejectsUnexpectedValues(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	_, err := normalizeRequestCounts(provider.ServiceHealthRequest{
		Service: "api", Start: start, End: start.Add(time.Hour), Alignment: time.Minute,
	}, []*monitoringpb.TimeSeries{{
		Metric: &metricpb.Metric{Labels: map[string]string{"response_code_class": "2xx"}},
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(start.Add(time.Minute))},
			Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_StringValue{StringValue: "sensitive"}},
		}},
	}})
	if !errors.Is(err, provider.ErrInvalidQuery) {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeLatencyP95OrdersReducedPoints(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	point := func(end time.Time, value float64) *monitoringpb.Point {
		return &monitoringpb.Point{
			Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(end)},
			Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: value}},
		}
	}
	result, err := normalizeLatencyP95(provider.ServiceHealthRequest{
		Start: start, End: start.Add(3 * time.Minute), Alignment: time.Minute,
	}, []*monitoringpb.TimeSeries{{Points: []*monitoringpb.Point{
		point(start.Add(2*time.Minute), 240), point(start.Add(time.Minute), 120),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != provider.HealthRequestLatencyP95 || result.Unit != "ms" || len(result.Points) != 2 || result.Points[0].Value != 120 || result.Points[1].Value != 240 {
		t.Fatalf("unexpected latency series: %#v", result)
	}
}

func TestNormalizeLatencyP95RejectsUnreducedDuplicates(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	point := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(start.Add(time.Minute))},
		Value:    &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 120}},
	}
	_, err := normalizeLatencyP95(provider.ServiceHealthRequest{
		Start: start, End: start.Add(time.Hour), Alignment: time.Minute,
	}, []*monitoringpb.TimeSeries{{Points: []*monitoringpb.Point{point}}, {Points: []*monitoringpb.Point{point}}})
	if !errors.Is(err, provider.ErrInvalidQuery) {
		t.Fatalf("error = %v", err)
	}
}

func TestFleetQueriesUseFixedAggregationAndExactORFilter(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := provider.FleetOverviewRequest{
		ProjectID: "synthetic-project-123", Services: []string{"worker", "checkout-api"}, Start: start, End: start.Add(90 * time.Minute),
	}
	count := fleetRequestCountQuery(req)
	wantFilter := `metric.type = "run.googleapis.com/request_count" AND resource.type = "cloud_run_revision" AND resource.labels.project_id = "synthetic-project-123" AND (resource.labels.service_name = "checkout-api" OR resource.labels.service_name = "worker")`
	if count.GetFilter() != wantFilter {
		t.Fatalf("count filter = %q", count.GetFilter())
	}
	aggregation := count.GetAggregation()
	if aggregation.GetAlignmentPeriod().AsDuration() != 90*time.Minute || aggregation.GetPerSeriesAligner() != monitoringpb.Aggregation_ALIGN_SUM || aggregation.GetCrossSeriesReducer() != monitoringpb.Aggregation_REDUCE_SUM {
		t.Fatalf("unexpected count aggregation: %#v", aggregation)
	}
	if got := aggregation.GetGroupByFields(); len(got) != 2 || got[0] != "resource.labels.service_name" || got[1] != "metric.labels.response_code_class" {
		t.Fatalf("count grouping = %#v", got)
	}
	latency := fleetRequestLatencyP95Query(req)
	if !strings.Contains(latency.GetFilter(), `metric.type = "run.googleapis.com/request_latencies"`) {
		t.Fatalf("latency filter = %q", latency.GetFilter())
	}
	primary := latency.GetAggregation()
	if primary.GetCrossSeriesReducer() != monitoringpb.Aggregation_REDUCE_SUM || len(primary.GetGroupByFields()) != 1 || primary.GetGroupByFields()[0] != "resource.labels.service_name" {
		t.Fatalf("latency distributions are not merged by service: %#v", primary)
	}
	if latency.GetSecondaryAggregation().GetPerSeriesAligner() != monitoringpb.Aggregation_ALIGN_PERCENTILE_95 {
		t.Fatalf("unexpected secondary aggregation: %#v", latency.GetSecondaryAggregation())
	}
	if count.GetPageSize() != maxRawFleetPoints || latency.GetPageSize() != maxRawFleetPoints {
		t.Fatalf("fleet query page sizes = %d, %d", count.GetPageSize(), latency.GetPageSize())
	}
	short := req
	short.End = short.Start.Add(30 * time.Second)
	if got := fleetRequestCountQuery(short).GetAggregation().GetAlignmentPeriod().AsDuration(); got != time.Minute {
		t.Fatalf("short-window alignment = %s", got)
	}
}

func TestFleetOverviewRejectsInvalidServicesBeforeProviderCall(t *testing.T) {
	called := false
	p := &Provider{fleetOverview: func(context.Context, provider.FleetOverviewRequest) (provider.FleetOverviewResult, error) {
		called = true
		return provider.FleetOverviewResult{}, nil
	}}
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	for _, services := range [][]string{{}, {"api", "api"}, {`api" OR true`}} {
		_, err := p.FleetOverview(context.Background(), provider.FleetOverviewRequest{
			ProjectID: "synthetic-project", Services: services, Start: start, End: start.Add(time.Hour),
		})
		if !errors.Is(err, provider.ErrInvalidQuery) {
			t.Errorf("services %#v error = %v", services, err)
		}
	}
	if called {
		t.Fatal("provider function was called for invalid input")
	}
}

func TestNormalizeFleetOverviewIncludesSparseServicesInNameOrder(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	series := func(service, class string, value float64) *monitoringpb.TimeSeries {
		return &monitoringpb.TimeSeries{
			Resource: &monitoredres.MonitoredResource{Labels: map[string]string{"service_name": service}},
			Metric:   &metricpb.Metric{Labels: map[string]string{"response_code_class": class}},
			Points:   []*monitoringpb.Point{{Value: &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: value}}}},
		}
	}
	result, err := normalizeFleetOverview(provider.FleetOverviewRequest{
		Services: []string{"worker", "checkout-api", "empty"}, Start: start, End: start.Add(time.Hour),
	}, []*monitoringpb.TimeSeries{
		series("checkout-api", "2xx", 40), series("checkout-api", "5xx", 3), series("worker", "2xx", 12),
	}, []*monitoringpb.TimeSeries{series("checkout-api", "", 125)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 3 || result.Services[0].Service != "checkout-api" || result.Services[1].Service != "empty" || result.Services[2].Service != "worker" {
		t.Fatalf("services = %#v", result.Services)
	}
	checkout, empty := result.Services[0], result.Services[1]
	if checkout.RequestCount != 43 || checkout.ServerErrorCount != 3 || checkout.RequestLatencyP95Ms == nil || *checkout.RequestLatencyP95Ms != 125 {
		t.Fatalf("checkout summary = %#v", checkout)
	}
	if empty.RequestCount != 0 || empty.ServerErrorCount != 0 || empty.RequestLatencyP95Ms != nil {
		t.Fatalf("empty summary = %#v", empty)
	}
}
