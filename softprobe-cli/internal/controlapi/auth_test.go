package controlapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"softprobe-runtime/internal/store"
)

func TestProtectedControlRoutesRequireBearerTokenWhenConfigured(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "secret-token")

	mux := NewMux(store.NewStore())
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"replay"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	assertAPIErrorCode(t, rec.Body.Bytes(), "missing_bearer_token")
}

func TestProtectedOTLPRoutesRequireBearerTokenWhenConfigured(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "secret-token")

	st := store.NewStore()
	session := st.Create("capture")
	mux := NewMux(st)

	tracePayload := []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}}
					]
				}]
			}]
		}]
	}`, session.ID))

	missingReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(tracePayload))
	missingRec := httptest.NewRecorder()
	mux.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing-token status = %d, want 401", missingRec.Code)
	}
	assertAPIErrorCode(t, missingRec.Body.Bytes(), "missing_bearer_token")

	wrongReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(tracePayload))
	wrongReq.Header.Set("Authorization", "Bearer wrong-token")
	wrongRec := httptest.NewRecorder()
	mux.ServeHTTP(wrongRec, wrongReq)
	if wrongRec.Code != http.StatusForbidden {
		t.Fatalf("wrong-token status = %d, want 403", wrongRec.Code)
	}
	assertAPIErrorCode(t, wrongRec.Body.Bytes(), "invalid_bearer_token")

	validReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(tracePayload))
	validReq.Header.Set("Authorization", "Bearer secret-token")
	validRec := httptest.NewRecorder()
	mux.ServeHTTP(validRec, validReq)
	if validRec.Code != http.StatusNoContent {
		t.Fatalf("valid-token status = %d, want 204: %s", validRec.Code, validRec.Body.String())
	}
}

func TestProtectedRoutesAcceptValidBearerToken(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "secret-token")

	mux := NewMux(store.NewStore())
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"replay"}`))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestHealthRouteSkipsBearerAuth(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "secret-token")

	mux := NewMux(store.NewStore())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestMalformedControlRequestsUseJSONErrorEnvelope(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "secret-token")

	mux := NewMux(store.NewStore())
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":`))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error body: %v", err)
	}
	if body.Error.Code != "invalid_request" {
		t.Fatalf("error code = %q, want invalid_request", body.Error.Code)
	}
	if body.Error.Message != "invalid create session request" {
		t.Fatalf("error message = %q, want invalid create session request", body.Error.Message)
	}
}

func TestUnknownSessionKeepsJSONErrorEnvelopeWithAuthEnabled(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "secret-token")

	mux := NewMux(store.NewStore())
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/missing/stats", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	assertAPIErrorCode(t, rec.Body.Bytes(), "unknown_session")
}

func assertAPIErrorCode(t *testing.T, body []byte, code string) {
	t.Helper()

	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Code != code {
		t.Fatalf("error code = %q, want %q", resp.Error.Code, code)
	}
}
