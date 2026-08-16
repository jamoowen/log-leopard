package fake

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jamoowen/log-leopard/internal/provider"
	"github.com/jamoowen/log-leopard/internal/query"
)

func TestQueryOrderingAndContextIdentifiers(t *testing.T) {
	p := New()
	newest, err := p.Query(context.Background(), provider.QueryRequest{PageSize: provider.MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	oldest, err := p.Query(context.Background(), provider.QueryRequest{PageSize: provider.MaxPageSize, Order: provider.OrderAscending})
	if err != nil {
		t.Fatal(err)
	}
	if len(newest.Entries) == 0 || len(oldest.Entries) != len(newest.Entries) {
		t.Fatalf("unexpected fake pages: %d and %d", len(newest.Entries), len(oldest.Entries))
	}
	if !newest.Entries[0].Timestamp.After(newest.Entries[len(newest.Entries)-1].Timestamp) {
		t.Fatal("default fake order is not newest-first")
	}
	if !oldest.Entries[0].Timestamp.Before(oldest.Entries[len(oldest.Entries)-1].Timestamp) {
		t.Fatal("ascending fake order is not chronological")
	}
	for _, entry := range oldest.Entries {
		if entry.RequestID == "" || entry.Trace == "" {
			t.Fatalf("fake context identifiers missing from %#v", entry)
		}
	}
}

func TestQueryExcludesEntriesOutsideCompiledTimeRange(t *testing.T) {
	filter, err := query.Compile(query.CompileInput{
		Start: time.Date(2026, 1, 15, 11, 58, 30, 0, time.UTC),
		End:   time.Date(2026, 1, 15, 11, 59, 30, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := New().Query(context.Background(), provider.QueryRequest{Filter: filter, PageSize: provider.MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != "synthetic-2" {
		t.Fatalf("entries = %#v, want only synthetic-2", result.Entries)
	}
}

func TestQueryReturnsNoEntriesForNonexistentCompiledSource(t *testing.T) {
	filter, err := query.Compile(query.CompileInput{
		Sources: []string{"missing-service"},
		Start:   time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := New().Query(context.Background(), provider.QueryRequest{Filter: filter, PageSize: provider.MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("entries = %#v, want none", result.Entries)
	}
}

func TestQueryFiltersLeopardServiceAlias(t *testing.T) {
	filter, err := query.Compile(query.CompileInput{
		Text:  "service:checkout-api",
		Start: time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := New().Query(context.Background(), provider.QueryRequest{Filter: filter, PageSize: provider.MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(result.Entries); !reflect.DeepEqual(got, []string{"synthetic-1", "synthetic-2"}) {
		t.Fatalf("entry IDs = %v, want checkout-api fixtures", got)
	}
}

func TestQueryFiltersCanonicalTopLevelSeverities(t *testing.T) {
	filter, err := query.Compile(query.CompileInput{
		Severities: []string{"ERROR", "INFO"},
		Start:      time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC),
		End:        time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := New().Query(context.Background(), provider.QueryRequest{Filter: filter, PageSize: provider.MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(result.Entries); !reflect.DeepEqual(got, []string{"synthetic-1", "synthetic-3"}) {
		t.Fatalf("entry IDs = %v, want ERROR and INFO fixtures", got)
	}
}

func TestQueryFiltersFreeAndQuotedTextAcrossMessagePayloads(t *testing.T) {
	filter, err := query.Compile(query.CompileInput{
		Text:  `scheduled "export completed"`,
		Start: time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := New().Query(context.Background(), provider.QueryRequest{Filter: filter, PageSize: provider.MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != "synthetic-3" || result.Entries[0].MessageSource != "jsonPayload.message" {
		t.Fatalf("entries = %#v, want synthetic JSON message", result.Entries)
	}
}

func TestQueryPaginatesFilteredEntriesInAscendingOrder(t *testing.T) {
	filter, err := query.Compile(query.CompileInput{
		Sources: []string{"checkout-api"},
		Start:   time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := provider.QueryRequest{Filter: filter, PageSize: 1, Order: provider.OrderAscending}

	first, err := New().Query(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(first.Entries); !reflect.DeepEqual(got, []string{"synthetic-2"}) || first.NextPageToken == "" {
		t.Fatalf("first page = %v, token %q", got, first.NextPageToken)
	}
	req.PageToken = first.NextPageToken
	second, err := New().Query(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(second.Entries); !reflect.DeepEqual(got, []string{"synthetic-1"}) || second.NextPageToken != "" {
		t.Fatalf("second page = %v, token %q", got, second.NextPageToken)
	}
}

func TestQueryPreservesAscendingRequestContextResults(t *testing.T) {
	filter, err := query.CompileRequestContext(
		time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
		"synthetic-request-123",
		"projects/synthetic-project/traces/synthetic-trace-123",
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := New().Query(context.Background(), provider.QueryRequest{
		Filter: filter, PageSize: provider.MaxPageSize, Order: provider.OrderAscending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := entryIDs(result.Entries); !reflect.DeepEqual(got, []string{"synthetic-4", "synthetic-3", "synthetic-2", "synthetic-1"}) {
		t.Fatalf("entry IDs = %v, want ascending context fixtures", got)
	}
}

func TestQueryRejectsUnsupportedCompiledConstructs(t *testing.T) {
	windowStart := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)
	inputs := map[string]query.CompileInput{
		"native": {
			NativeFilter: `logName = "projects/synthetic/logs/custom"`,
			Start:        windowStart,
			End:          windowStart.Add(time.Hour),
		},
		"structured": {
			Predicates: []query.FieldPredicate{{Path: "custom.id", Operator: "equals", Value: "synthetic"}},
			Start:      windowStart,
			End:        windowStart.Add(time.Hour),
		},
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			filter, err := query.Compile(input)
			if err != nil {
				t.Fatal(err)
			}
			_, err = New().Query(context.Background(), provider.QueryRequest{Filter: filter, PageSize: provider.MaxPageSize})
			if !errors.Is(err, provider.ErrInvalidQuery) {
				t.Fatalf("error = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

func entryIDs(entries []provider.Entry) []string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.ID
	}
	return ids
}

func TestServiceHealthIsDeterministicAndBounded(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := provider.ServiceHealthRequest{Service: "checkout-api", Start: start, End: start.Add(time.Hour), Alignment: time.Minute}
	first, err := New().ServiceHealth(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New().ServiceHealth(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("fake health result is not deterministic")
	}
	if len(first.Series) != 3 || first.Series[0].Name != provider.HealthRequestCount || first.Series[1].Name != provider.HealthServerErrorCount || first.Series[2].Name != provider.HealthRequestLatencyP95 {
		t.Fatalf("unexpected series: %#v", first.Series)
	}
	for _, series := range first.Series {
		if len(series.Points) != 60 {
			t.Fatalf("points = %d, want 60", len(series.Points))
		}
		for i, point := range series.Points {
			if point.Timestamp.Before(start) || point.Timestamp.After(req.End) || i > 0 && !series.Points[i-1].Timestamp.Before(point.Timestamp) {
				t.Fatalf("invalid point order or bounds: %#v", series.Points)
			}
		}
	}
}

func TestFleetOverviewIsDeterministicAndOrdered(t *testing.T) {
	start := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	req := provider.FleetOverviewRequest{Services: []string{"worker", "checkout-api"}, Start: start, End: start.Add(time.Hour)}
	first, err := New().FleetOverview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New().FleetOverview(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Services) != 2 || first.Services[0].Service != "checkout-api" || first.Services[1].Service != "worker" {
		t.Fatalf("unexpected fake fleet result: %#v", first)
	}
}
