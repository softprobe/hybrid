// Package hostedbackend wires hosted capture ingest/export through datalake.
package hostedbackend

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/proxybackend"
	"softprobe-runtime/internal/store"
)

// HandleTraces ingests extract spans directly into datalake without requiring
// a pre-created runtime session.
func HandleTraces(st store.Store, sink interface {
	IngestTraces(context.Context, []byte) error
}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = st
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
		if err != nil {
			http.Error(w, "invalid traces payload", http.StatusBadRequest)
			return
		}

		tenantInfo, hasTenant := controlapi.TenantFromContext(r.Context())
		if !hasTenant {
			writeAPIError(w, http.StatusUnauthorized, "missing_tenant", "authentication required")
			return
		}

		captureID := req.SessionID
		if captureID == "" {
			captureID = "cap_" + uuid.New().String()
		}
		annotated, err := annotateCapture(normalized, captureID, tenantInfo.TenantID)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid traces payload")
			return
		}
		if err := sink.IngestTraces(r.Context(), annotated); err != nil {
			slog.Error("hosted traces: datalake ingest failed", "captureId", captureID, "tenantId", tenantInfo.TenantID, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "storage_error", "failed to ingest traces")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"captureId": captureID,
			"accepted":  true,
		})
	})
}

// HandleClose uses the default close semantics and does not perform capture
// assembly in hosted mode anymore.
func HandleClose(st store.Store) func(http.ResponseWriter, *http.Request, string) {
	return func(w http.ResponseWriter, r *http.Request, sessionID string) {
		session, ok := st.Get(sessionID)
		if !ok {
			writeAPIError(w, http.StatusNotFound, "unknown_session", "unknown session")
			return
		}

		resp := map[string]any{"sessionId": sessionID, "closed": true}
		_ = session

		if !st.Close(sessionID) {
			writeAPIError(w, http.StatusNotFound, "unknown_session", "unknown session")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// HandleLoadCase keeps replay semantics in the runtime store.
func HandleLoadCase(st store.Store) func(http.ResponseWriter, *http.Request, string) {
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

func normalizeOTLP(body []byte) ([]byte, error) {
	return proxybackend.NormalizeOTLPJSON(body)
}

func annotateCapture(payload []byte, captureID, tenantID string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, err
	}
	rs, _ := doc["resourceSpans"].([]any)
	for _, rsRaw := range rs {
		rsMap, _ := rsRaw.(map[string]any)
		ensureAttribute(rsMap, "resource", "sp.capture.id", captureID)
		ensureAttribute(rsMap, "resource", "sp.tenant.id", tenantID)
		scopeSpans, _ := rsMap["scopeSpans"].([]any)
		for _, ssRaw := range scopeSpans {
			ssMap, _ := ssRaw.(map[string]any)
			spans, _ := ssMap["spans"].([]any)
			for _, sRaw := range spans {
				spanMap, _ := sRaw.(map[string]any)
				appendSpanAttr(spanMap, "sp.capture.id", captureID)
				appendSpanAttr(spanMap, "sp.tenant.id", tenantID)
			}
		}
	}
	return json.Marshal(doc)
}

func ensureAttribute(rs map[string]any, field, key, val string) {
	resource, ok := rs[field].(map[string]any)
	if !ok {
		resource = map[string]any{}
		rs[field] = resource
	}
	attrs, _ := resource["attributes"].([]any)
	resource["attributes"] = append(attrs, map[string]any{
		"key": key,
		"value": map[string]any{
			"stringValue": val,
		},
	})
}

func appendSpanAttr(span map[string]any, key, val string) {
	attrs, _ := span["attributes"].([]any)
	span["attributes"] = append(attrs, map[string]any{
		"key": key,
		"value": map[string]any{
			"stringValue": val,
		},
	})
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}
