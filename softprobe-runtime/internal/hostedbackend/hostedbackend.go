// Package hostedbackend wires GCS persistence into the extract, close, and load-case paths.
// It is only active when SOFTPROBE_HOSTED=true. OSS paths in proxybackend and controlapi
// are unchanged.
package hostedbackend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/gcs"
	"softprobe-runtime/internal/proxybackend"
	"softprobe-runtime/internal/store"
)

// HandleTraces returns an http.Handler that, for capture sessions, writes the
// OTLP extract payload to GCS and appends the object path to the Redis extracts
// list. For non-capture sessions it behaves identically to the OSS handler.
func HandleTraces(st store.Store, gcsClient *gcs.Client, bucket string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid traces payload", http.StatusBadRequest)
			return
		}

		normalized, err := normalizeOTLP(body)
		if err != nil {
			http.Error(w, "invalid traces payload", http.StatusBadRequest)
			return
		}

		req, err := proxybackend.ParseExtractRequest(normalized)
		if err != nil || req.SessionID == "" {
			http.Error(w, "invalid traces payload", http.StatusBadRequest)
			return
		}

		session, ok := st.Get(req.SessionID)
		if !ok {
			http.Error(w, "unknown session", http.StatusNotFound)
			return
		}

		if session.Mode == "capture" {
			tenantInfo, hasTenant := controlapi.TenantFromContext(r.Context())
			if hasTenant {
				objPath := extractObjectPath(tenantInfo.TenantID, req.SessionID, uuid.New().String())
				if err := gcsClient.Put(r.Context(), bucket, objPath, normalized); err != nil {
					slog.Error("hosted traces: persist extract failed", "sessionId", req.SessionID, "tenantId", tenantInfo.TenantID, "bucket", bucket, "object", objPath, "error", err)
					writeAPIError(w, http.StatusInternalServerError, "storage_error", "failed to persist extract")
					return
				}
				if !st.BufferExtract(req.SessionID, []byte(objPath)) {
					slog.Error("hosted traces: append extract ref failed", "sessionId", req.SessionID, "object", objPath)
					writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to persist extract ref")
					return
				}
				if _, ok := st.RecordExtractedSpans(req.SessionID, req.SpanCount); !ok {
					slog.Error("hosted traces: record extracted spans failed", "sessionId", req.SessionID, "spanCount", req.SpanCount)
					writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to update extract stats")
					return
				}
			} else {
				// OSS fallback: buffer raw payload in store
				st.BufferExtract(req.SessionID, normalized)
				_, _ = st.RecordExtractedSpans(req.SessionID, req.SpanCount)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// HandleClose handles session close for the hosted path. For capture sessions it:
//  1. Reads all extract GCS object paths from the Redis list.
//  2. Fetches each object from GCS.
//  3. Merges them into a case JSON document.
//  4. Writes the case to gs://{bucket}/cases/{sessionID}.case.json.
//  5. Deletes the session.
func HandleClose(st store.Store, gcsClient *gcs.Client, bucket string) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, sessionID string) {
		session, ok := st.Get(sessionID)
		if !ok {
			writeAPIError(w, http.StatusNotFound, "unknown_session", "unknown session")
			return
		}

		resp := map[string]any{"sessionId": sessionID, "closed": true}

		if session.Mode == "capture" {
			tenantInfo, hasTenant := controlapi.TenantFromContext(r.Context())
			if hasTenant {
				caseRef, err := writeCaseToGCS(r.Context(), st, gcsClient, session, bucket, tenantInfo.TenantID)
				if err != nil {
					slog.Error("hosted close: write case failed", "sessionId", sessionID, "tenantId", tenantInfo.TenantID, "bucket", bucket, "error", err)
					writeAPIError(w, http.StatusInternalServerError, "internal_error", "write case failed")
					return
				}
				resp["caseRef"] = caseRef
			}
		}

		if !st.Close(sessionID) {
			writeAPIError(w, http.StatusNotFound, "unknown_session", "unknown session")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// HandleLoadCase handles POST /v1/sessions/{id}/load-case for the hosted path.
// It stores the case body to GCS and also keeps it in the session for the inject hot path.
func HandleLoadCase(st store.Store, gcsClient *gcs.Client, bucket string) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, sessionID string) {
		if _, ok := st.Get(sessionID); !ok {
			writeAPIError(w, http.StatusNotFound, "unknown_session", "unknown session")
			return
		}

		caseBody, err := io.ReadAll(r.Body)
		if err != nil || len(caseBody) == 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid load-case request")
			return
		}

		// Persist to GCS first so failures are surfaced immediately.
		if tenantInfo, hasTenant := controlapi.TenantFromContext(r.Context()); hasTenant {
			objPath := caseObjectPath(tenantInfo.TenantID, sessionID)
			if err := gcsClient.Put(r.Context(), bucket, objPath, caseBody); err != nil {
				slog.Error("hosted load-case: persist case failed", "sessionId", sessionID, "tenantId", tenantInfo.TenantID, "bucket", bucket, "object", objPath, "error", err)
				writeAPIError(w, http.StatusInternalServerError, "storage_error", "failed to persist case")
				return
			}
		}

		// Store in session for inject hot path.
		session, ok := st.LoadCase(sessionID, caseBody)
		if !ok {
			writeAPIError(w, http.StatusNotFound, "unknown_session", "unknown session")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sessionId":       session.ID,
			"sessionRevision": session.Revision,
		})
	}
}

func writeCaseToGCS(ctx context.Context, st store.Store, gcsClient *gcs.Client, session store.Session, bucket, tenantID string) (string, error) {
	rs, ok := st.(*store.RedisStore)
	if !ok {
		return "", fmt.Errorf("hostedbackend: store is not a RedisStore")
	}

	refs := rs.GetExtractRefs(session.ID)
	payloads := make([][]byte, 0, len(refs))
	for _, ref := range refs {
		data, err := gcsClient.Get(ctx, bucket, ref)
		if err != nil {
			return "", fmt.Errorf("hostedbackend: fetch extract %s: %w", ref, err)
		}
		payloads = append(payloads, data)
	}

	caseJSON, err := proxybackend.BuildCaseJSON(session.ID, payloads)
	if err != nil {
		return "", err
	}

	objPath := caseObjectPath(tenantID, session.ID)
	if err := gcsClient.Put(ctx, bucket, objPath, caseJSON); err != nil {
		return "", err
	}
	return objPath, nil
}

func caseObjectPath(tenantID, sessionID string) string {
	return fmt.Sprintf("tenants/%s/cases/%s.case.json", tenantID, sessionID)
}

func extractObjectPath(tenantID, sessionID, extractID string) string {
	return fmt.Sprintf("tenants/%s/extracts/%s/%s.otlp.json", tenantID, sessionID, extractID)
}

func normalizeOTLP(body []byte) ([]byte, error) {
	return proxybackend.NormalizeOTLPJSON(body)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}

// closedAt is used in BuildCaseJSON doc.
var _ = time.Now
