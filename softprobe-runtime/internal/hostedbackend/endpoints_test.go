package hostedbackend_test

import (
	"context"
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

func newEndpointDeps(t *testing.T) (store.Store, *gcs.Client, string, authn.TenantInfo) {
	t.Helper()
	mr := miniredis.RunT(t)
	st, err := store.NewRedisStore(mr.Addr(), "", "tenantE", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewRedisStore: %v", err)
	}
	fakeSrv := fakestorage.NewServer([]fakestorage.Object{})
	t.Cleanup(fakeSrv.Stop)
	bucket := "tenantE-bucket"
	fakeSrv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: bucket})
	gcsClient := gcs.NewClientFromStorage(fakeSrv.Client())
	tenantInfo := authn.TenantInfo{TenantID: "tenantE", BucketName: bucket}
	return st, gcsClient, bucket, tenantInfo
}

// ── GET /v1/cases/{caseId} ────────────────────────────────────────────────────

func TestGetCase_Hit(t *testing.T) {
	st, gcsClient, bucket, tenantInfo := newEndpointDeps(t)

	// Seed a case file in GCS
	sess := st.Create("replay")
	caseBody := []byte(`{"version":"1.0.0","caseId":"` + sess.ID + `","traces":[]}`)
	_ = gcsClient.Put(t.Context(), bucket, "tenants/"+tenantInfo.TenantID+"/cases/"+sess.ID+".case.json", caseBody)

	mux := hostedbackend.NewHostedEndpoints(st, gcsClient, bucket)
	req := httptest.NewRequest(http.MethodGet, "/v1/cases/"+sess.ID, nil)
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), sess.ID) {
		t.Errorf("response does not contain caseId %q", sess.ID)
	}
}

func TestGetCase_NotFound(t *testing.T) {
	st, gcsClient, bucket, tenantInfo := newEndpointDeps(t)
	mux := hostedbackend.NewHostedEndpoints(st, gcsClient, bucket)

	req := httptest.NewRequest(http.MethodGet, "/v1/cases/sess_unknown", nil)
	req = req.WithContext(controlapi.WithTenant(req.Context(), tenantInfo))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetCase_RequiresTenantContext(t *testing.T) {
	st, gcsClient, bucket, _ := newEndpointDeps(t)
	mux := hostedbackend.NewHostedEndpoints(st, gcsClient, bucket)

	req := httptest.NewRequest(http.MethodGet, "/v1/cases/sess_x", nil)
	// no tenant context
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestGetCase_StorageErrorReturnsInternalError(t *testing.T) {
	st, gcsClient, bucket, tenantInfo := newEndpointDeps(t)
	sess := st.Create("replay")

	mux := hostedbackend.NewHostedEndpoints(st, gcsClient, bucket)
	req := httptest.NewRequest(http.MethodGet, "/v1/cases/"+sess.ID, nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(controlapi.WithTenant(ctx, tenantInfo))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

