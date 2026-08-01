package fake

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jamoowen/log-leopard/internal/provider"
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
	if len(first.Series) != 2 || first.Series[0].Name != provider.HealthRequestCount || first.Series[1].Name != provider.HealthServerErrorCount {
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
