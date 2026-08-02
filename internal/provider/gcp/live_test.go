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

func TestLiveServiceHealth(t *testing.T) {
	projectID := os.Getenv("LOG_LEOPARD_GCP_PROJECT")
	if projectID == "" {
		t.Skip("set LOG_LEOPARD_GCP_PROJECT to run the read-only live service-health smoke test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	services, err := discoverServices(ctx, projectID)
	if err != nil {
		t.Fatalf("live service discovery failed: category=%s", errorCategory(err))
	}
	if len(services) == 0 {
		t.Skip("project has no Cloud Run services")
	}
	service := services[0].ID
	requestedService := os.Getenv("LOG_LEOPARD_GCP_SERVICE")
	if requestedService != "" {
		service = ""
		for _, candidate := range services {
			if candidate.ID == requestedService {
				service = candidate.ID
				break
			}
		}
		if service == "" {
			t.Fatal("LOG_LEOPARD_GCP_SERVICE was not discovered in the configured project")
		}
	}
	end := time.Now().UTC()
	result, err := New().ServiceHealth(ctx, provider.ServiceHealthRequest{
		ProjectID: projectID, Service: service, Start: end.Add(-24 * time.Hour), End: end, Alignment: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("live service health failed: category=%s", errorCategory(err))
	}
	if len(result.Series) != 3 || result.Series[0].Name != provider.HealthRequestCount || result.Series[1].Name != provider.HealthServerErrorCount || result.Series[2].Name != provider.HealthRequestLatencyP95 {
		t.Fatal("live service health returned an unexpected shape")
	}
	if requestedService != "" {
		for _, series := range result.Series {
			if len(series.Points) == 0 {
				t.Fatalf("explicit live service returned no %s points", series.Name)
			}
		}
	}
}

func TestLiveFleetOverview(t *testing.T) {
	projectID := os.Getenv("LOG_LEOPARD_GCP_PROJECT")
	if projectID == "" {
		t.Skip("set LOG_LEOPARD_GCP_PROJECT to run the read-only live fleet-overview smoke test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	services, err := discoverServices(ctx, projectID)
	if err != nil {
		t.Fatalf("live service discovery failed: category=%s", errorCategory(err))
	}
	if len(services) == 0 {
		t.Skip("project has no Cloud Run services")
	}
	service := services[0].ID
	requestedService := os.Getenv("LOG_LEOPARD_GCP_SERVICE")
	if requestedService != "" {
		service = requestedService
	}
	end := time.Now().UTC().Truncate(time.Minute)
	result, err := New().FleetOverview(ctx, provider.FleetOverviewRequest{
		ProjectID: projectID,
		Services:  []string{service},
		Start:     end.Add(-24 * time.Hour),
		End:       end,
	})
	if err != nil {
		t.Fatalf("live fleet overview failed: category=%s", errorCategory(err))
	}
	if len(result.Services) != 1 || result.Services[0].Service != service {
		t.Fatal("live fleet overview returned an unexpected shape")
	}
	if requestedService != "" && (result.Services[0].RequestCount == 0 || result.Services[0].RequestLatencyP95Ms == nil) {
		t.Fatal("explicit live service returned incomplete fleet metrics")
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
