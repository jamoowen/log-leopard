package gcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jamoowen/log-leopard/internal/provider"
)

func TestLiveQuery(t *testing.T) {
	projectID := os.Getenv("LOG_LEOPARD_GCP_PROJECT")
	if projectID == "" {
		t.Skip("set LOG_LEOPARD_GCP_PROJECT to run the read-only live query smoke test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	end := time.Now().UTC()
	filter := fmt.Sprintf(
		`resource.type = "cloud_run_revision" AND timestamp >= %q AND timestamp < %q`,
		end.Add(-5*time.Minute).Format(time.RFC3339Nano),
		end.Format(time.RFC3339Nano),
	)
	_, err := New().Query(ctx, provider.QueryRequest{
		ProjectID: projectID,
		Filter:    filter,
		PageSize:  1,
		Order:     provider.OrderDescending,
	})
	if err != nil {
		category := "unknown"
		for name, target := range map[string]error{
			"authentication":    provider.ErrAuthentication,
			"permission_denied": provider.ErrPermissionDenied,
			"rate_limited":      provider.ErrRateLimited,
			"unavailable":       provider.ErrUnavailable,
			"invalid_query":     provider.ErrInvalidQuery,
			"configuration":     provider.ErrConfiguration,
		} {
			if errors.Is(err, target) {
				category = name
				break
			}
		}
		t.Fatalf("live query failed: category=%s", category)
	}
}
