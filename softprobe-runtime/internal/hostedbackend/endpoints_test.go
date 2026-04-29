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
	"softprobe-runtime/internal/datalake"
	"softprobe-runtime/internal/hostedbackend"
	"softprobe-runtime/internal/store"
)

type fakeSQLClient struct {
	resp datalake.SQLQueryResponse
	err  error
}

func (f *fakeSQLClient) QuerySQL(_ context.Context, _ string) (datalake.SQLQueryResponse, error) {
	return f.resp, f.err
}

func TestGetCapture_Hit(t *testing.T) {
	st := store.NewStore()
	client := &fakeSQLClient{
		resp: datalake.SQLQueryResponse{
			Columns: []string{"trace_id", "http_request_path"},
			Rows: [][]json.RawMessage{
				{json.RawMessage(`"trace1"`), json.RawMessage(`"/checkout"`)},
			},
			RowCount: 1,
		},
	}
	mux := hostedbackend.NewHostedEndpoints(st, client)
	req := httptest.NewRequest(http.MethodGet, "/v1/captures/cap_123", nil)
	req = req.WithContext(controlapi.WithTenant(req.Context(), authn.TenantInfo{TenantID: "tenantE"}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"captureId": "cap_123"`) {
		t.Fatalf("missing captureId in response: %s", rec.Body.String())
	}
}

func TestGetCapture_NotFound(t *testing.T) {
	st := store.NewStore()
	client := &fakeSQLClient{
		resp: datalake.SQLQueryResponse{Columns: []string{"trace_id"}, Rows: nil, RowCount: 0},
	}
	mux := hostedbackend.NewHostedEndpoints(st, client)
	req := httptest.NewRequest(http.MethodGet, "/v1/captures/cap_unknown", nil)
	req = req.WithContext(controlapi.WithTenant(req.Context(), authn.TenantInfo{TenantID: "tenantE"}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetCapture_RequiresTenantContext(t *testing.T) {
	st := store.NewStore()
	client := &fakeSQLClient{}
	mux := hostedbackend.NewHostedEndpoints(st, client)
	req := httptest.NewRequest(http.MethodGet, "/v1/captures/cap_x", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
