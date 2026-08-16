package gcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/jamoowen/log-leopard/internal/provider"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const maxRawHealthPoints = provider.MaxHealthBuckets * 10

var cloudRunServiceName = regexp.MustCompile(`^[a-z](?:[a-z0-9-]{0,47}[a-z0-9])?$`)

func validateHealthRequest(req provider.ServiceHealthRequest) error {
	if req.ProjectID == "" || !cloudRunServiceName.MatchString(req.Service) {
		return provider.ErrConfiguration
	}
	if req.Start.IsZero() || req.End.IsZero() || !req.Start.Before(req.End) || req.End.Sub(req.Start) > provider.MaxHealthWindow {
		return provider.ErrInvalidQuery
	}
	if req.Alignment < time.Minute || int64(req.End.Sub(req.Start)/req.Alignment)+1 > provider.MaxHealthBuckets+1 {
		return provider.ErrInvalidQuery
	}
	return nil
}

func (p *Provider) queryServiceHealth(ctx context.Context, req provider.ServiceHealthRequest) (provider.ServiceHealthResult, error) {
	return queryServiceHealthWithOptions(ctx, req, nil)
}

func queryServiceHealthWithOptions(ctx context.Context, req provider.ServiceHealthRequest, options []option.ClientOption) (provider.ServiceHealthResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	options = append(options, option.WithScopes("https://www.googleapis.com/auth/monitoring.read"))
	client, err := monitoring.NewMetricClient(ctx, options...)
	if err != nil {
		return provider.ServiceHealthResult{}, fmt.Errorf("create monitoring client: %w", classifyClientConstructionError(err))
	}
	defer func() { _ = client.Close() }()

	requestSeries, points, err := collectTimeSeries(client.ListTimeSeries(ctx, requestCountQuery(req)), maxRawHealthPoints)
	if err != nil {
		return provider.ServiceHealthResult{}, fmt.Errorf("list request-count time series: %w", classifyError(err))
	}
	latencySeries, _, err := collectTimeSeries(client.ListTimeSeries(ctx, requestLatencyP95Query(req)), maxRawHealthPoints-points)
	if err != nil {
		return provider.ServiceHealthResult{}, fmt.Errorf("list request-latency time series: %w", classifyError(err))
	}
	result, err := normalizeRequestCounts(req, requestSeries)
	if err != nil {
		return provider.ServiceHealthResult{}, err
	}
	latency, err := normalizeLatencyP95(req, latencySeries)
	if err != nil {
		return provider.ServiceHealthResult{}, err
	}
	result.Series = append(result.Series, latency)
	return result, nil
}

func collectTimeSeries(it *monitoring.TimeSeriesIterator, limit int) ([]*monitoringpb.TimeSeries, int, error) {
	series := []*monitoringpb.TimeSeries{}
	points := 0
	for {
		item, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, points, err
		}
		points += len(item.GetPoints())
		if points > limit {
			return nil, points, provider.ErrResponseTooLarge
		}
		series = append(series, item)
	}
	return series, points, nil
}

func requestLatencyP95Query(req provider.ServiceHealthRequest) *monitoringpb.ListTimeSeriesRequest {
	return &monitoringpb.ListTimeSeriesRequest{
		Name:     "projects/" + req.ProjectID,
		Filter:   `metric.type = "run.googleapis.com/request_latencies" AND resource.type = "cloud_run_revision" AND resource.labels.project_id = "` + req.ProjectID + `" AND resource.labels.service_name = "` + req.Service + `"`,
		Interval: &monitoringpb.TimeInterval{StartTime: timestamppb.New(req.Start), EndTime: timestamppb.New(req.End)},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    durationpb.New(req.Alignment),
			PerSeriesAligner:   monitoringpb.Aggregation_ALIGN_SUM,
			CrossSeriesReducer: monitoringpb.Aggregation_REDUCE_SUM,
		},
		SecondaryAggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  durationpb.New(req.Alignment),
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_PERCENTILE_95,
		},
		View:     monitoringpb.ListTimeSeriesRequest_FULL,
		PageSize: maxRawHealthPoints,
	}
}

func requestCountQuery(req provider.ServiceHealthRequest) *monitoringpb.ListTimeSeriesRequest {
	return &monitoringpb.ListTimeSeriesRequest{
		Name:     "projects/" + req.ProjectID,
		Filter:   `metric.type = "run.googleapis.com/request_count" AND resource.type = "cloud_run_revision" AND resource.labels.project_id = "` + req.ProjectID + `" AND resource.labels.service_name = "` + req.Service + `"`,
		Interval: &monitoringpb.TimeInterval{StartTime: timestamppb.New(req.Start), EndTime: timestamppb.New(req.End)},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    durationpb.New(req.Alignment),
			PerSeriesAligner:   monitoringpb.Aggregation_ALIGN_SUM,
			CrossSeriesReducer: monitoringpb.Aggregation_REDUCE_SUM,
			GroupByFields:      []string{"metric.labels.response_code_class"},
		},
		View:     monitoringpb.ListTimeSeriesRequest_FULL,
		PageSize: maxRawHealthPoints,
	}
}

func normalizeRequestCounts(req provider.ServiceHealthRequest, input []*monitoringpb.TimeSeries) (provider.ServiceHealthResult, error) {
	totals := map[time.Time]float64{}
	errors5xx := map[time.Time]float64{}
	for _, series := range input {
		class := series.GetMetric().GetLabels()["response_code_class"]
		for _, point := range series.GetPoints() {
			timestamp := point.GetInterval().GetEndTime().AsTime().UTC()
			if timestamp.IsZero() || timestamp.Before(req.Start) || timestamp.After(req.End) {
				return provider.ServiceHealthResult{}, fmt.Errorf("invalid monitoring point timestamp: %w", provider.ErrInvalidQuery)
			}
			value, err := numericHealthValue(point.GetValue())
			if err != nil {
				return provider.ServiceHealthResult{}, err
			}
			totals[timestamp] += value
			if class == "5xx" {
				errors5xx[timestamp] += value
			}
		}
	}
	timestamps := make([]time.Time, 0, len(totals))
	for timestamp := range totals {
		timestamps = append(timestamps, timestamp)
	}
	slices.SortFunc(timestamps, func(a, b time.Time) int { return a.Compare(b) })
	if len(timestamps) > provider.MaxHealthBuckets {
		return provider.ServiceHealthResult{}, provider.ErrResponseTooLarge
	}
	requestPoints := make([]provider.HealthPoint, 0, len(timestamps))
	errorPoints := make([]provider.HealthPoint, 0, len(timestamps))
	for _, timestamp := range timestamps {
		requestPoints = append(requestPoints, provider.HealthPoint{Timestamp: timestamp, Value: totals[timestamp]})
		errorPoints = append(errorPoints, provider.HealthPoint{Timestamp: timestamp, Value: errors5xx[timestamp]})
	}
	return provider.ServiceHealthResult{
		Service: req.Service, Start: req.Start.UTC(), End: req.End.UTC(), Alignment: req.Alignment,
		Series: []provider.HealthSeries{
			{Name: provider.HealthRequestCount, Unit: "1", Points: requestPoints},
			{Name: provider.HealthServerErrorCount, Unit: "1", Points: errorPoints},
		},
	}, nil
}

func normalizeLatencyP95(req provider.ServiceHealthRequest, input []*monitoringpb.TimeSeries) (provider.HealthSeries, error) {
	values := map[time.Time]float64{}
	for _, series := range input {
		for _, point := range series.GetPoints() {
			timestamp := point.GetInterval().GetEndTime().AsTime().UTC()
			if timestamp.IsZero() || timestamp.Before(req.Start) || timestamp.After(req.End) {
				return provider.HealthSeries{}, fmt.Errorf("invalid latency point timestamp: %w", provider.ErrInvalidQuery)
			}
			if _, duplicate := values[timestamp]; duplicate {
				return provider.HealthSeries{}, fmt.Errorf("duplicate reduced latency point: %w", provider.ErrInvalidQuery)
			}
			value, err := numericHealthValue(point.GetValue())
			if err != nil {
				return provider.HealthSeries{}, err
			}
			values[timestamp] = value
		}
	}
	timestamps := make([]time.Time, 0, len(values))
	for timestamp := range values {
		timestamps = append(timestamps, timestamp)
	}
	slices.SortFunc(timestamps, func(a, b time.Time) int { return a.Compare(b) })
	if len(timestamps) > provider.MaxHealthBuckets {
		return provider.HealthSeries{}, provider.ErrResponseTooLarge
	}
	points := make([]provider.HealthPoint, 0, len(timestamps))
	for _, timestamp := range timestamps {
		points = append(points, provider.HealthPoint{Timestamp: timestamp, Value: values[timestamp]})
	}
	return provider.HealthSeries{Name: provider.HealthRequestLatencyP95, Unit: "ms", Points: points}, nil
}

func numericHealthValue(value *monitoringpb.TypedValue) (float64, error) {
	if value == nil {
		return 0, fmt.Errorf("missing monitoring value: %w", provider.ErrInvalidQuery)
	}
	switch typed := value.Value.(type) {
	case *monitoringpb.TypedValue_Int64Value:
		if typed.Int64Value < 0 {
			return 0, fmt.Errorf("negative monitoring count: %w", provider.ErrInvalidQuery)
		}
		return float64(typed.Int64Value), nil
	case *monitoringpb.TypedValue_DoubleValue:
		if typed.DoubleValue < 0 || math.IsNaN(typed.DoubleValue) || math.IsInf(typed.DoubleValue, 0) {
			return 0, fmt.Errorf("negative monitoring count: %w", provider.ErrInvalidQuery)
		}
		return typed.DoubleValue, nil
	default:
		return 0, fmt.Errorf("unexpected monitoring value type: %w", provider.ErrInvalidQuery)
	}
}
