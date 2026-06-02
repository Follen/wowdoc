package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReportsPoolsAndDiagnostics(t *testing.T) {
	app := NewApp(DefaultConfig())
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	for _, key := range []string{"sources", "clients", "invalidDirectories", "pools", "recentErrors"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("health missing %q: %#v", key, body)
		}
	}
}

func TestReadOnlyRoutesRejectNonGETMethods(t *testing.T) {
	app := NewApp(DefaultConfig())
	for _, path := range []string{"/health", "/help"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		app.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s POST status = %d, want %d", path, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestMCPRouteReportsStreamableHTTPReadiness(t *testing.T) {
	app := NewApp(DefaultConfig())
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/mcp", nil)
		rec := httptest.NewRecorder()
		app.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s /mcp status = %d, want %d", method, rec.Code, http.StatusOK)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s /mcp invalid json: %v", method, err)
		}
		if body["transport"] != "streamable-http" || body["status"] != "ready" {
			t.Fatalf("%s /mcp body = %#v", method, body)
		}
	}
}

func TestMCPRouteRejectsUnsupportedMethods(t *testing.T) {
	app := NewApp(DefaultConfig())
	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE /mcp status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
