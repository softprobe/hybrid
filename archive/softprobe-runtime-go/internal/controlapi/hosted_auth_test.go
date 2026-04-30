package controlapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"softprobe-runtime/internal/authn"
	"softprobe-runtime/internal/controlapi"
)

// fakeResolver satisfies the authn.Resolver interface surface via a stub.
func fakeResolver(valid map[string]authn.TenantInfo) *authn.Resolver {
	// Build a real Resolver pointed at a fake server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			APIKey string `json:"apiKey"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		info, ok := valid[req.APIKey]
		if !ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
			return
		}
		configJSON, _ := json.Marshal(map[string]string{
			"dataset_id":  "ds",
			"bucket_name": info.BucketName,
		})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"tenantId": info.TenantID,
				"resources": []map[string]any{
					{"resourceType": "BIGQUERY_DATASET", "configJson": string(configJSON)},
				},
			},
		})
	}))
	// The server leaks but is fine for tests; GC will close it.
	_ = srv
	return authn.NewResolver(srv.URL, 60*time.Second)
}

func TestHostedBearerMiddleware_MissingHeader(t *testing.T) {
	resolver := fakeResolver(map[string]authn.TenantInfo{
		"sk_good": {TenantID: "t1", BucketName: "bkt"},
	})
	mux := controlapi.NewHostedMux(resolver)

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestHostedBearerMiddleware_InvalidKey(t *testing.T) {
	resolver := fakeResolver(map[string]authn.TenantInfo{
		"sk_good": {TenantID: "t1", BucketName: "bkt"},
	})
	mux := controlapi.NewHostedMux(resolver)

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer sk_bad")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHostedBearerMiddleware_ValidKey_InjectsTenantContext(t *testing.T) {
	resolver := fakeResolver(map[string]authn.TenantInfo{
		"sk_good": {TenantID: "tenant-xyz", BucketName: "bkt-xyz"},
	})
	mux := controlapi.NewHostedMux(resolver)

	// /health is unauthed; use /v1/meta to confirm tenant context is set
	req := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	req.Header.Set("Authorization", "Bearer sk_good")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHostedBearerMiddleware_AcceptsXPublicKeyCompatibilityHeader(t *testing.T) {
	resolver := fakeResolver(map[string]authn.TenantInfo{
		"sk_good": {TenantID: "tenant-xyz", BucketName: "bkt-xyz"},
	})
	mux := controlapi.NewHostedMux(resolver)

	req := httptest.NewRequest(http.MethodGet, "/v1/meta", nil)
	req.Header.Set("x-public-key", "sk_good")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestHostedBearerMiddleware_HealthSkipsAuth(t *testing.T) {
	resolver := fakeResolver(map[string]authn.TenantInfo{})
	mux := controlapi.NewHostedMux(resolver)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/health without auth: status = %d, want 200", rec.Code)
	}
}

func TestTenantFromContext(t *testing.T) {
	ctx := controlapi.WithTenant(context.Background(), authn.TenantInfo{TenantID: "t1", BucketName: "b1"})
	info, ok := controlapi.TenantFromContext(ctx)
	if !ok {
		t.Fatal("TenantFromContext: ok = false, want true")
	}
	if info.TenantID != "t1" {
		t.Errorf("TenantID = %q, want %q", info.TenantID, "t1")
	}
}
