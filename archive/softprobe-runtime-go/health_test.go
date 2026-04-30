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

func TestMetaHandlerReturnsCompatibilityFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	rec := httptest.NewRecorder()

	controlapi.NewMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		RuntimeVersion string `json:"runtimeVersion"`
		SpecVersion    string `json:"specVersion"`
		SchemaVersion  string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal meta body: %v", err)
	}
	if body.RuntimeVersion == "" {
		t.Fatal("runtimeVersion is empty")
	}
	if body.SpecVersion == "" {
		t.Fatal("specVersion is empty")
	}
	if body.SchemaVersion == "" {
		t.Fatal("schemaVersion is empty")
	}
}

func TestMetaHandlerRejectsNonGet(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/meta", nil)
	rec := httptest.NewRecorder()

	controlapi.NewMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
