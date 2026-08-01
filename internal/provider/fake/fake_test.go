package fake

import (
	"context"
	"testing"

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
