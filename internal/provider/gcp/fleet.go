package gcp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/jamoowen/log-leopard/internal/provider"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const maxRawFleetPoints = provider.MaxFleetServices * 10

func validateFleetOverviewRequest(req provider.FleetOverviewRequest) error {
	if req.ProjectID == "" {
		return provider.ErrConfiguration
	}
	if len(req.Services) < 1 || len(req.Services) > provider.MaxFleetServices || req.Start.IsZero() || req.End.IsZero() || !req.Start.Before(req.End) || req.End.Sub(req.Start) > provider.MaxHealthWindow {
		return provider.ErrInvalidQuery
	}
	seen := make(map[string]struct{}, len(req.Services))
	for _, service := range req.Services {
		if !cloudRunServiceName.MatchString(service) {
			return provider.ErrInvalidQuery
		}
		if _, duplicate := seen[service]; duplicate {
			return provider.ErrInvalidQuery
		}
		seen[service] = struct{}{}
	}
	return nil
}

func queryFleetOverview(ctx context.Context, req provider.FleetOverviewRequest) (provider.FleetOverviewResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client, err := monitoring.NewMetricClient(ctx, option.WithScopes("https://www.googleapis.com/auth/monitoring.read"))
	if err != nil {
		return provider.FleetOverviewResult{}, fmt.Errorf("create monitoring client: %w", classifyError(err))
	}
	defer func() { _ = client.Close() }()

	requestSeries, points, err := collectTimeSeries(client.ListTimeSeries(ctx, fleetRequestCountQuery(req)), maxRawFleetPoints)
	if err != nil {
		return provider.FleetOverviewResult{}, fmt.Errorf("list fleet request-count time series: %w", classifyError(err))
	}
	latencySeries, _, err := collectTimeSeries(client.ListTimeSeries(ctx, fleetRequestLatencyP95Query(req)), maxRawFleetPoints-points)
	if err != nil {
		return provider.FleetOverviewResult{}, fmt.Errorf("list fleet request-latency time series: %w", classifyError(err))
	}
	return normalizeFleetOverview(req, requestSeries, latencySeries)
}

func fleetRequestCountQuery(req provider.FleetOverviewRequest) *monitoringpb.ListTimeSeriesRequest {
	return &monitoringpb.ListTimeSeriesRequest{
		Name:     "projects/" + req.ProjectID,
		Filter:   fleetMetricFilter(req, "run.googleapis.com/request_count"),
		Interval: &monitoringpb.TimeInterval{StartTime: timestamppb.New(req.Start), EndTime: timestamppb.New(req.End)},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    durationpb.New(fleetAlignment(req)),
			PerSeriesAligner:   monitoringpb.Aggregation_ALIGN_SUM,
			CrossSeriesReducer: monitoringpb.Aggregation_REDUCE_SUM,
			GroupByFields:      []string{"resource.labels.service_name", "metric.labels.response_code_class"},
		},
		View:     monitoringpb.ListTimeSeriesRequest_FULL,
		PageSize: maxRawFleetPoints,
	}
}

func fleetRequestLatencyP95Query(req provider.FleetOverviewRequest) *monitoringpb.ListTimeSeriesRequest {
	alignment := durationpb.New(fleetAlignment(req))
	return &monitoringpb.ListTimeSeriesRequest{
		Name:     "projects/" + req.ProjectID,
		Filter:   fleetMetricFilter(req, "run.googleapis.com/request_latencies"),
		Interval: &monitoringpb.TimeInterval{StartTime: timestamppb.New(req.Start), EndTime: timestamppb.New(req.End)},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    alignment,
			PerSeriesAligner:   monitoringpb.Aggregation_ALIGN_SUM,
			CrossSeriesReducer: monitoringpb.Aggregation_REDUCE_SUM,
			GroupByFields:      []string{"resource.labels.service_name"},
		},
		SecondaryAggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  alignment,
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_PERCENTILE_95,
		},
		View:     monitoringpb.ListTimeSeriesRequest_FULL,
		PageSize: maxRawFleetPoints,
	}
}

func fleetAlignment(req provider.FleetOverviewRequest) time.Duration {
	return max(time.Minute, req.End.Sub(req.Start))
}

func fleetMetricFilter(req provider.FleetOverviewRequest, metric string) string {
	services := slices.Clone(req.Services)
	slices.Sort(services)
	terms := make([]string, 0, len(services))
	for _, service := range services {
		terms = append(terms, `resource.labels.service_name = "`+service+`"`)
	}
	return `metric.type = "` + metric + `" AND resource.type = "cloud_run_revision" AND resource.labels.project_id = "` + req.ProjectID + `" AND (` + strings.Join(terms, " OR ") + `)`
}

func normalizeFleetOverview(req provider.FleetOverviewRequest, requestSeries, latencySeries []*monitoringpb.TimeSeries) (provider.FleetOverviewResult, error) {
	services := slices.Clone(req.Services)
	slices.Sort(services)
	summaries := make(map[string]*provider.FleetOverviewSummary, len(services))
	for _, service := range services {
		summaries[service] = &provider.FleetOverviewSummary{Service: service}
	}
	for _, series := range requestSeries {
		service := series.GetResource().GetLabels()["service_name"]
		summary, ok := summaries[service]
		if !ok {
			return provider.FleetOverviewResult{}, fmt.Errorf("unexpected fleet service: %w", provider.ErrInvalidQuery)
		}
		for _, point := range series.GetPoints() {
			value, err := numericHealthValue(point.GetValue())
			if err != nil {
				return provider.FleetOverviewResult{}, err
			}
			summary.RequestCount += value
			if series.GetMetric().GetLabels()["response_code_class"] == "5xx" {
				summary.ServerErrorCount += value
			}
		}
	}
	seenLatency := make(map[string]struct{}, len(latencySeries))
	for _, series := range latencySeries {
		service := series.GetResource().GetLabels()["service_name"]
		summary, ok := summaries[service]
		if !ok {
			return provider.FleetOverviewResult{}, fmt.Errorf("unexpected fleet latency service: %w", provider.ErrInvalidQuery)
		}
		for _, point := range series.GetPoints() {
			if _, duplicate := seenLatency[service]; duplicate {
				return provider.FleetOverviewResult{}, fmt.Errorf("duplicate fleet latency point: %w", provider.ErrInvalidQuery)
			}
			value, err := numericHealthValue(point.GetValue())
			if err != nil {
				return provider.FleetOverviewResult{}, err
			}
			summary.RequestLatencyP95Ms = &value
			seenLatency[service] = struct{}{}
		}
	}
	result := provider.FleetOverviewResult{Start: req.Start.UTC(), End: req.End.UTC(), Services: make([]provider.FleetOverviewSummary, 0, len(services))}
	for _, service := range services {
		result.Services = append(result.Services, *summaries[service])
	}
	return result, nil
}
