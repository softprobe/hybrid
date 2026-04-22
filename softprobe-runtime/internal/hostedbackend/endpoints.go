package hostedbackend

import (
	"log/slog"
	"net/http"
	"strings"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/gcs"
	"softprobe-runtime/internal/store"
)

// NewHostedEndpoints returns an http.ServeMux with the hosted-only endpoints:
//
//	GET /v1/cases/{caseId}   — download a stored case JSON
func NewHostedEndpoints(st store.Store, gcsClient *gcs.Client, bucket string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/v1/cases/", handleGetCase(st, gcsClient, bucket))
	return mux
}

func handleGetCase(st store.Store, gcsClient *gcs.Client, bucket string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		tenantInfo, ok := controlapi.TenantFromContext(r.Context())
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "missing_tenant", "authentication required")
			return
		}

		caseID := strings.TrimPrefix(r.URL.Path, "/v1/cases/")
		caseID = strings.TrimSuffix(caseID, "/")
		if caseID == "" {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "missing case id")
			return
		}

		objPath := caseObjectPath(tenantInfo.TenantID, caseID)
		data, err := gcsClient.Get(r.Context(), bucket, objPath)
		if err != nil {
			if gcs.IsNotFound(err) {
				writeAPIError(w, http.StatusNotFound, "not_found", "case not found")
				return
			}
			slog.Error("hosted get-case: read case failed", "caseId", caseID, "tenantId", tenantInfo.TenantID, "bucket", bucket, "object", objPath, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "storage_error", "failed to read case")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}

