package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jamoowen/log-leopard/internal/auth"
	"github.com/jamoowen/log-leopard/internal/cursor"
	"github.com/jamoowen/log-leopard/internal/profile"
	providerapi "github.com/jamoowen/log-leopard/internal/provider"
	"github.com/jamoowen/log-leopard/internal/provider/fake"
	"github.com/jamoowen/log-leopard/internal/provider/gcp"
	"github.com/jamoowen/log-leopard/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "log-leopard:", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:0", "numeric loopback listen address")
	configPath := flag.String("config", "", "connection profile JSON path")
	fakeMode := flag.Bool("fake", false, "use synthetic data without GCP")
	open := flag.Bool("open", false, "open the pairing URL in the default browser")
	browserURL := flag.String("browser-url", "", "loopback URL hosting the browser UI (defaults to the backend)")
	openAPIPath := flag.String("write-openapi", "", "write OpenAPI JSON and exit")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	path := *configPath
	if path == "" {
		var err error
		path, err = profile.DefaultPath()
		if err != nil {
			return err
		}
	}
	sessions, pairingToken := auth.NewManager(10*time.Minute, 12*time.Hour)
	cursors := cursor.New(15 * time.Minute)
	var backend providerapi.Provider = gcp.New()
	if *fakeMode {
		backend = fake.New()
	}

	if *openAPIPath != "" {
		app, err := newServer("127.0.0.1:1", path, backend, sessions, cursors)
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(app.API.OpenAPI(), "", "  ")
		if err != nil {
			return fmt.Errorf("encode OpenAPI: %w", err)
		}
		data = append(data, '\n')
		if err := atomicWrite(*openAPIPath, data); err != nil {
			return err
		}
		return nil
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = listener.Close() }()
	host := listener.Addr().String()
	app, err := newServer(host, path, backend, sessions, cursors)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Handler:           app.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()
	// This URL is the only intentional disclosure of the short-lived pairing secret.
	pairingURL, err := makePairingURL(*browserURL, host, pairingToken)
	if err != nil {
		return err
	}
	fmt.Printf("LogLeopard: %s\n", pairingURL)
	if *open {
		if err := openBrowser(ctx, pairingURL); err != nil {
			slog.Warn("could not open default browser", "error", err)
		}
	}
	slog.Info("server started", "address", host, "provider", providerName(*fakeMode))
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	}
}

func makePairingURL(base, host, token string) (string, error) {
	if base == "" {
		return fmt.Sprintf("http://%s/#pair=%s", host, token), nil
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse browser URL: %w", err)
	}
	ip := net.ParseIP(parsed.Hostname())
	if parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || ip == nil || !ip.IsLoopback() {
		return "", errors.New("browser URL must be an HTTP URL on a numeric loopback address without credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/"
	return parsed.String() + "#pair=" + token, nil
}

func openBrowser(ctx context.Context, url string) error {
	name, args, err := browserCommand(runtime.GOOS, url)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, name, args...)
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func browserCommand(goos, url string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "linux":
		return "xdg-open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	default:
		return "", nil, fmt.Errorf("opening a browser is unsupported on %s", goos)
	}
}

func newServer(host, path string, backend providerapi.Provider, sessions *auth.Manager, cursors *cursor.Signer) (*server.Server, error) {
	return server.New(server.Config{
		Host:     host,
		Origin:   "http://" + host,
		Profiles: profile.NewStore(path),
		Provider: backend,
		Sessions: sessions,
		Cursors:  cursors,
		Logger:   slog.Default(),
	})
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".openapi-*")
	if err != nil {
		return fmt.Errorf("create OpenAPI artifact: %w", err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write OpenAPI artifact: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close OpenAPI artifact: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace OpenAPI artifact: %w", err)
	}
	return nil
}

func providerName(fakeMode bool) string {
	if fakeMode {
		return "fake"
	}
	return "gcp"
}
