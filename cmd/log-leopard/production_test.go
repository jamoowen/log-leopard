//go:build production

package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamoowen/log-leopard/internal/auth"
	"github.com/jamoowen/log-leopard/internal/cursor"
	"github.com/jamoowen/log-leopard/internal/provider/fake"
)

func TestProductionServerServesEmbeddedApplication(t *testing.T) {
	sessions, _ := auth.NewManager(time.Minute, time.Hour)
	app, err := newServer(
		"127.0.0.1:8787",
		filepath.Join(t.TempDir(), "connections.json"),
		fake.New(),
		sessions,
		cursor.New(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/", nil)
	response := httptest.NewRecorder()
	app.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<title>LogLeopard</title>") {
		t.Fatalf("production application response: status=%d body=%q", response.Code, response.Body.String())
	}
}
