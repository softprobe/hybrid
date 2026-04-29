package hostedbackend_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"softprobe-runtime/internal/authn"
	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/hostedbackend"
	"softprobe-runtime/internal/store"
)

type fakeDatalakeSink struct {
	payloads [][]byte
	err      error
}

func (f *fakeDatalakeSink) IngestTraces(_ context.Context, payload []byte) error {
	if f.err != nil {
		return f.err
	}
	f.payloads = append(f.payloads, payload)
	return nil
}

func TestHostedTracesHandler_IngestsWithoutSession(t *testing.T) {
	st := store.NewStore()
	sink := &fakeDatalakeSink{}
	handler := hostedbackend.HandleTraces(st, sink)
	tenantInfo := authn.TenantInfo{TenantID: "tenant1"}

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(buildExtractPayload("")))
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if len(sink.payloads) != 1 {
		t.Fatalf("ingested payloads = %d, want 1", len(sink.payloads))
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["captureId"] == "" {
		t.Fatalf("captureId missing in response body: %v", body)
	}
}

func TestHostedTracesHandler_RequiresTenant(t *testing.T) {
	st := store.NewStore()
	sink := &fakeDatalakeSink{}
	handler := hostedbackend.HandleTraces(st, sink)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(buildExtractPayload("")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHostedLoadCase_StoresInSessionForReplay(t *testing.T) {
	st := store.NewStore()
	sess := st.Create("replay")
	loader := hostedbackend.HandleLoadCase(st)

	caseBody := []byte(`{"version":"1.0.0","caseId":"test-case","traces":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sess.ID+"/load-case", strings.NewReader(string(caseBody)))
	rec := httptest.NewRecorder()
	loader(rec, req, sess.ID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	updated, ok := st.Get(sess.ID)
	if !ok {
		t.Fatal("session not found after load-case")
	}
	if string(updated.LoadedCase) != string(caseBody) {
		t.Fatalf("loadedCase mismatch: got %q want %q", string(updated.LoadedCase), string(caseBody))
	}
}

func TestHostedClose_DeletesSession(t *testing.T) {
	st := store.NewStore()
	sess := st.Create("replay")
	closeFn := hostedbackend.HandleClose(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sess.ID+"/close", nil)
	rec := httptest.NewRecorder()
	closeFn(rec, req, sess.ID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if _, ok := st.Get(sess.ID); ok {
		t.Fatal("session still exists after close")
	}
}

func buildExtractPayload(sessionID string) string {
	sessionAttr := ""
	if sessionID != "" {
		sessionAttr = `,{"key":"sp.session.id","value":{"stringValue":"` + sessionID + `"}}`
	}
	return `{
  "resourceSpans": [{
    "scopeSpans": [{
      "spans": [{
        "traceId": "AAAAAAAAAAAAAAAAAAAAAA==",
        "spanId": "AAAAAAAAAAA=",
        "name": "sp.extract",
        "attributes": [
          {"key": "sp.span.type", "value": {"stringValue": "extract"}}` + sessionAttr + `
        ]
      }]
    }]
  }]
}`
}
