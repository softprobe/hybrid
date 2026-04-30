package controlapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"softprobe-runtime/internal/store"
)

func TestOpenAPIYAML(t *testing.T) {
	mux := NewMux(store.NewStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /openapi.yaml: status %d, body %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "yaml") {
		t.Fatalf("Content-Type = %q, want yaml", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "openapi: 3.0.3") || !strings.Contains(body, "/v1/sessions") {
		t.Fatalf("unexpected openapi body prefix: %q", truncate(body, 200))
	}
}

func TestSwaggerUI(t *testing.T) {
	mux := NewMux(store.NewStore())
	for _, path := range []string{"/docs", "/docs/"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", path, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("GET %s: Content-Type = %q", path, rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), "SwaggerUIBundle") {
			t.Fatalf("GET %s: missing SwaggerUIBundle", path)
		}
	}
}

func TestOpenAPIYAMLMethodNotAllowed(t *testing.T) {
	mux := NewMux(store.NewStore())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/openapi.yaml", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /openapi.yaml: status %d, want 405", rec.Code)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
