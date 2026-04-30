package authn_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"softprobe-runtime/internal/authn"
)

// fakeAuthServer returns a test server that validates a known key and rejects others.
func fakeAuthServer(t *testing.T, validKey, tenantID, bucketName string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			APIKey string `json:"apiKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.APIKey != validKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "invalid key"})
			return
		}
		configJSON, _ := json.Marshal(map[string]string{
			"dataset_id":  "ds_" + tenantID,
			"bucket_name": bucketName,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"tenantId":   tenantID,
				"tenantName": "Test Tenant",
				"resources": []map[string]any{
					{
						"resourceType": "BIGQUERY_DATASET",
						"configJson":   string(configJSON),
					},
				},
				"storageExceeded": false,
			},
		})
	}))
}

func TestResolve_ValidKey(t *testing.T) {
	srv := fakeAuthServer(t, "sk_valid", "tenant-abc", "bucket-abc")
	defer srv.Close()

	r := authn.NewResolver(srv.URL, 60*time.Second)
	info, err := r.Resolve(context.Background(), "sk_valid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.TenantID != "tenant-abc" {
		t.Errorf("TenantID = %q, want %q", info.TenantID, "tenant-abc")
	}
	if info.BucketName != "bucket-abc" {
		t.Errorf("BucketName = %q, want %q", info.BucketName, "bucket-abc")
	}
}

func TestResolve_InvalidKey(t *testing.T) {
	srv := fakeAuthServer(t, "sk_valid", "tenant-abc", "bucket-abc")
	defer srv.Close()

	r := authn.NewResolver(srv.URL, 60*time.Second)
	_, err := r.Resolve(context.Background(), "sk_wrong")
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}
}

func TestResolve_EmptyKey(t *testing.T) {
	srv := fakeAuthServer(t, "sk_valid", "tenant-abc", "bucket-abc")
	defer srv.Close()

	r := authn.NewResolver(srv.URL, 60*time.Second)
	_, err := r.Resolve(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
}

func TestResolve_CachesResult(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		configJSON, _ := json.Marshal(map[string]string{"dataset_id": "ds", "bucket_name": "bkt"})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"tenantId": "t1",
				"resources": []map[string]any{
					{"resourceType": "BIGQUERY_DATASET", "configJson": string(configJSON)},
				},
			},
		})
	}))
	defer srv.Close()

	r := authn.NewResolver(srv.URL, 60*time.Second)
	_, _ = r.Resolve(context.Background(), "sk_x")
	_, _ = r.Resolve(context.Background(), "sk_x")
	_, _ = r.Resolve(context.Background(), "sk_x")

	if calls != 1 {
		t.Errorf("auth service called %d times, want 1 (should be cached)", calls)
	}
}

func TestResolve_CacheExpires(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		configJSON, _ := json.Marshal(map[string]string{"dataset_id": "ds", "bucket_name": "bkt"})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"tenantId": "t1",
				"resources": []map[string]any{
					{"resourceType": "BIGQUERY_DATASET", "configJson": string(configJSON)},
				},
			},
		})
	}))
	defer srv.Close()

	r := authn.NewResolver(srv.URL, 50*time.Millisecond)
	_, _ = r.Resolve(context.Background(), "sk_y")
	time.Sleep(100 * time.Millisecond)
	_, _ = r.Resolve(context.Background(), "sk_y")

	if calls != 2 {
		t.Errorf("auth service called %d times after cache expiry, want 2", calls)
	}
}

func TestResolve_AuthServiceDown(t *testing.T) {
	r := authn.NewResolver("http://127.0.0.1:1", 60*time.Second)
	_, err := r.Resolve(context.Background(), "sk_any")
	if err == nil {
		t.Fatal("expected error when auth service is unreachable, got nil")
	}
}
