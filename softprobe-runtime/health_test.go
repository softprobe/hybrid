package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"softprobe-runtime/internal/controlapi"
)

func TestHealthHandlerReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	controlapi.NewMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Status        string `json:"status"`
		SpecVersion   string `json:"specVersion"`
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal health body: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
	if body.SpecVersion == "" {
		t.Fatal("specVersion is empty")
	}
	if body.SchemaVersion == "" {
		t.Fatal("schemaVersion is empty")
	}
}
