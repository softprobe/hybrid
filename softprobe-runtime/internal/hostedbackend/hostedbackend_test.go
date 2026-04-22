package hostedbackend_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/fsouza/fake-gcs-server/fakestorage"

	"softprobe-runtime/internal/authn"
	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/gcs"
	"softprobe-runtime/internal/hostedbackend"
	"softprobe-runtime/internal/store"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newTestDeps(t *testing.T) (store.Store, *gcs.Client, string) {
	t.Helper()
	mr := miniredis.RunT(t)
	st, err := store.NewRedisStore(mr.Addr(), "", "tenant1", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}

	fakeSrv := fakestorage.NewServer([]fakestorage.Object{})
	t.Cleanup(fakeSrv.Stop)
	bucket := "tenant1-bucket"
	fakeSrv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: bucket})
	gcsClient := gcs.NewClientFromStorage(fakeSrv.Client())

	return st, gcsClient, bucket
}

func fakeAuthResolver(t *testing.T, tenantID, bucket string) *authn.Resolver {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ APIKey string `json:"apiKey"` }
		_ = json.NewDecoder(r.Body).Decode(&req)
		configJSON, _ := json.Marshal(map[string]string{"dataset_id": "ds", "bucket_name": bucket})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"tenantId": tenantID,
				"resources": []map[string]any{
					{"resourceType": "BIGQUERY_DATASET", "configJson": string(configJSON)},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return authn.NewResolver(srv.URL, 60*time.Second)
}

// ── HD3.2: extract writes to GCS ─────────────────────────────────────────────

func TestHostedTracesHandler_WritesExtractToGCS(t *testing.T) {
	st, gcsClient, bucket := newTestDeps(t)

	// Create a capture session first
	tenantInfo := authn.TenantInfo{TenantID: "tenant1", BucketName: bucket}
	sess := st.Create("capture")

	handler := hostedbackend.HandleTraces(st, gcsClient, bucket)

	otlpPayload := buildExtractPayload(t, sess.ID)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(otlpPayload))
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}

	// The extract should be stored in GCS and refs appended to Redis list
	refs := st.(*store.RedisStore).GetExtractRefs(sess.ID)
	if len(refs) != 1 {
		t.Fatalf("extract refs = %d, want 1", len(refs))
	}
	if !strings.HasPrefix(refs[0], "tenants/"+tenantInfo.TenantID+"/extracts/"+sess.ID+"/") {
		t.Errorf("ref = %q, want tenants/{tenantID}/extracts/{sessID}/... prefix", refs[0])
	}
	// Verify the object exists in GCS
	_, err := gcsClient.Get(context.Background(), bucket, refs[0])
	if err != nil {
		t.Errorf("GCS object not found: %v", err)
	}
}

// ── HD3.3: case file on close ─────────────────────────────────────────────────

func TestHostedCloseSession_WritesCaseToGCS(t *testing.T) {
	st, gcsClient, bucket := newTestDeps(t)
	tenantInfo := authn.TenantInfo{TenantID: "tenant1", BucketName: bucket}

	// Create capture session and buffer one extract
	sess := st.Create("capture")
	handler := hostedbackend.HandleTraces(st, gcsClient, bucket)
	otlpPayload := buildExtractPayload(t, sess.ID)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(otlpPayload))
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Close the session — should merge extracts into a case JSON and write to GCS
	closer := hostedbackend.HandleClose(st, gcsClient, bucket)
	closeReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sess.ID+"/close", nil)
	closeReq = closeReq.WithContext(controlapi.WithTenant(closeReq.Context(), tenantInfo))
	closeRec := httptest.NewRecorder()
	closer(closeRec, closeReq, sess.ID)

	if closeRec.Code != http.StatusOK {
		t.Fatalf("close status = %d, body: %s", closeRec.Code, closeRec.Body.String())
	}

	// Case file must exist in GCS
	caseObj := "tenants/" + tenantInfo.TenantID + "/cases/" + sess.ID + ".case.json"
	data, err := gcsClient.Get(context.Background(), bucket, caseObj)
	if err != nil {
		t.Fatalf("case file not found in GCS: %v", err)
	}
	var caseDoc struct {
		CaseID string            `json:"caseId"`
		Traces []json.RawMessage `json:"traces"`
	}
	if err := json.Unmarshal(data, &caseDoc); err != nil {
		t.Fatalf("invalid case JSON: %v", err)
	}
	if caseDoc.CaseID != sess.ID {
		t.Errorf("caseId = %q, want %q", caseDoc.CaseID, sess.ID)
	}
	if len(caseDoc.Traces) == 0 {
		t.Error("case file has no traces")
	}

	// Session must be closed (removed from store)
	if _, ok := st.Get(sess.ID); ok {
		t.Error("session still present after close")
	}
}

// ── HD3.4: load-case stores to GCS ───────────────────────────────────────────

func TestHostedLoadCase_StoresToGCS(t *testing.T) {
	st, gcsClient, bucket := newTestDeps(t)
	tenantInfo := authn.TenantInfo{TenantID: "tenant1", BucketName: bucket}

	sess := st.Create("replay")
	caseBody := []byte(`{"version":"1.0.0","caseId":"test-case","traces":[]}`)

	loader := hostedbackend.HandleLoadCase(st, gcsClient, bucket)
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sess.ID+"/load-case",
		strings.NewReader(string(caseBody)))
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	loader(rec, req, sess.ID)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}

	// Case must be in GCS
	caseObj := "tenants/" + tenantInfo.TenantID + "/cases/" + sess.ID + ".case.json"
	data, err := gcsClient.Get(context.Background(), bucket, caseObj)
	if err != nil {
		t.Fatalf("case file not in GCS: %v", err)
	}
	if string(data) != string(caseBody) {
		t.Errorf("GCS case = %q, want %q", data, caseBody)
	}

	// Session LoadedCase must still be set (for the inject hot path which reads it in-process)
	updated, ok := st.Get(sess.ID)
	if !ok {
		t.Fatal("session not found after load-case")
	}
	if string(updated.LoadedCase) != string(caseBody) {
		t.Errorf("session LoadedCase = %q, want %q", updated.LoadedCase, caseBody)
	}
}

func TestHostedLoadCase_GCSWriteFailureReturnsInternalError(t *testing.T) {
	st, gcsClient, _ := newTestDeps(t)
	tenantInfo := authn.TenantInfo{TenantID: "tenant1", BucketName: "missing-bucket"}

	sess := st.Create("replay")
	caseBody := []byte(`{"version":"1.0.0","caseId":"test-case","traces":[]}`)

	loader := hostedbackend.HandleLoadCase(st, gcsClient, "missing-bucket")
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sess.ID+"/load-case",
		strings.NewReader(string(caseBody)))
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	loader(rec, req, sess.ID)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}

	// Fail fast: do not mutate session state when durable write fails.
	updated, ok := st.Get(sess.ID)
	if !ok {
		t.Fatal("session not found after failed load-case")
	}
	if len(updated.LoadedCase) != 0 {
		t.Errorf("session LoadedCase unexpectedly set after failed write: %q", updated.LoadedCase)
	}
	if updated.Revision != 0 {
		t.Errorf("session revision = %d, want 0", updated.Revision)
	}
}

func TestHostedTracesHandler_GCSWriteFailureReturnsInternalError(t *testing.T) {
	st, gcsClient, _ := newTestDeps(t)
	tenantInfo := authn.TenantInfo{TenantID: "tenant1", BucketName: "missing-bucket"}

	sess := st.Create("capture")
	handler := hostedbackend.HandleTraces(st, gcsClient, "missing-bucket")

	otlpPayload := buildExtractPayload(t, sess.ID)
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(otlpPayload))
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}

	refs := st.(*store.RedisStore).GetExtractRefs(sess.ID)
	if len(refs) != 0 {
		t.Fatalf("extract refs = %d, want 0", len(refs))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildExtractPayload returns a minimal OTLP JSON payload with one extract span.
func buildExtractPayload(t *testing.T, sessionID string) string {
	t.Helper()
	return `{
  "resourceSpans": [{
    "scopeSpans": [{
      "spans": [{
        "traceId": "AAAAAAAAAAAAAAAAAAAAAA==",
        "spanId": "AAAAAAAAAAA=",
        "name": "sp.extract",
        "attributes": [
          {"key": "sp.span.type", "value": {"stringValue": "extract"}},
          {"key": "sp.session.id", "value": {"stringValue": "` + sessionID + `"}},
          {"key": "http.response.status_code", "value": {"intValue": "200"}}
        ]
      }]
    }]
  }]
}`
}
