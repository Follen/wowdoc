package http

import (
	"encoding/json"
	"net/http/httptest"
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
