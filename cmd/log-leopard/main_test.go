package main

import (
	"reflect"
	"testing"
)

func TestMakePairingURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
		ok   bool
	}{
		{name: "backend default", want: "http://127.0.0.1:8787/#pair=token", ok: true},
		{name: "development UI", base: "http://127.0.0.1:5173", want: "http://127.0.0.1:5173/#pair=token", ok: true},
		{name: "IPv6 loopback", base: "http://[::1]:5173/ui", want: "http://[::1]:5173/ui/#pair=token", ok: true},
		{name: "remote host", base: "https://example.com", ok: false},
		{name: "hostname loopback", base: "http://localhost:5173", ok: false},
		{name: "existing fragment", base: "http://127.0.0.1:5173/#other", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := makePairingURL(tt.base, "127.0.0.1:8787", "token")
			if (err == nil) != tt.ok {
				t.Fatalf("makePairingURL() error = %v, want success %t", err, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("makePairingURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBrowserCommandUsesURLAsAnArgument(t *testing.T) {
	url := `http://127.0.0.1:8787/#pair=token&x=$(unsafe);still-one-argument`
	tests := []struct {
		goos    string
		browser string
		name    string
		args    []string
	}{
		{goos: "darwin", browser: "default", name: "open", args: []string{url}},
		{goos: "darwin", browser: "brave", name: "open", args: []string{"-a", "Brave Browser", url}},
		{goos: "linux", browser: "default", name: "xdg-open", args: []string{url}},
		{goos: "linux", browser: "brave", name: "brave-browser", args: []string{url}},
		{goos: "windows", browser: "default", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", url}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, err := browserCommand(tt.goos, tt.browser, url)
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.name || !reflect.DeepEqual(args, tt.args) {
				t.Fatalf("got %q %#v, want %q %#v", name, args, tt.name, tt.args)
			}
		})
	}
	if _, _, err := browserCommand("plan9", "brave", url); err == nil {
		t.Fatal("unsupported platform was accepted")
	}
	if _, _, err := browserCommand("darwin", "unknown", url); err == nil {
		t.Fatal("unknown browser was accepted")
	}
}
