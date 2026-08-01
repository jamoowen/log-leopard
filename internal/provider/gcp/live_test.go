package gcp

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jamoowen/log-leopard/internal/provider"
	"github.com/jamoowen/log-leopard/internal/query"
)

func TestLiveQuery(t *testing.T) {
	projectID := os.Getenv("LOG_LEOPARD_GCP_PROJECT")
	if projectID == "" {
		t.Skip("set LOG_LEOPARD_GCP_PROJECT to run the read-only live query smoke test")
	}

	end := time.Now().UTC()
	for name, input := range map[string]query.CompileInput{
		"blank":      {Start: end.Add(-15 * time.Minute), End: end},
		"severities": {Severities: []string{"WARNING", "ERROR"}, Start: end.Add(-15 * time.Minute), End: end},
		"keyword":    {Text: "starting", Start: end.Add(-15 * time.Minute), End: end},
		"structured": {Predicates: []query.FieldPredicate{{Path: "jsonPayload.level", Operator: "equals", Value: "INFO"}}, Start: end.Add(-15 * time.Minute), End: end},
	} {
		t.Run(name, func(t *testing.T) {
			filter, err := query.Compile(input)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, err = New().Query(ctx, provider.QueryRequest{
				ProjectID: projectID,
				Filter:    filter,
				PageSize:  1,
				Order:     provider.OrderDescending,
			})
			if err != nil {
				t.Fatalf("live query failed: category=%s", errorCategory(err))
			}
		})
	}
}

func errorCategory(err error) string {
	for name, target := range map[string]error{
		"authentication":    provider.ErrAuthentication,
		"permission_denied": provider.ErrPermissionDenied,
		"rate_limited":      provider.ErrRateLimited,
		"unavailable":       provider.ErrUnavailable,
		"invalid_query":     provider.ErrInvalidQuery,
		"configuration":     provider.ErrConfiguration,
	} {
		if errors.Is(err, target) {
			return name
		}
	}
	return "unknown"
}
