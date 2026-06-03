package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestRunReturnsListenAndServeError(t *testing.T) {
	want := errors.New("bind failed")
	got := run(func(addr string, handler http.Handler) error {
		if addr == "" || handler == nil {
			t.Fatalf("server called with addr=%q handler=%v", addr, handler)
		}
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("run error = %v, want %v", got, want)
	}
}

func TestRunUsesConfigFromEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wowdoc.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  host: 127.0.0.1
  port: 9791
sources:
  root: testdata/sources
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("WOWDOC_CONFIG", path)

	want := errors.New("stop")
	var gotAddr string
	got := run(func(addr string, handler http.Handler) error {
		gotAddr = addr
		if handler == nil {
			t.Fatalf("handler is nil")
		}
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("run error = %v, want %v", got, want)
	}
	if gotAddr != "127.0.0.1:9791" {
		t.Fatalf("addr = %q, want 127.0.0.1:9791", gotAddr)
	}
}

func TestRunWithArgsAcceptsMCPHTTPSubcommand(t *testing.T) {
	want := errors.New("stop")
	called := false
	got := runWithArgs([]string{"mcp", "http"}, func(addr string, handler http.Handler) error {
		called = true
		if addr == "" || handler == nil {
			t.Fatalf("server called with addr=%q handler=%v", addr, handler)
		}
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("run error = %v, want %v", got, want)
	}
	if !called {
		t.Fatalf("listenAndServe was not called")
	}
}

func TestRunWithArgsRejectsUnknownSubcommands(t *testing.T) {
	got := runWithArgs([]string{"clients", "list"}, func(addr string, handler http.Handler) error {
		t.Fatalf("listenAndServe should not be called for invalid args")
		return nil
	})
	if got == nil || got.Error() != "unsupported command: clients list" {
		t.Fatalf("run error = %v", got)
	}
}
