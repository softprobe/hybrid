package proxybackend

import (
	"errors"
	"io"
	"net/http"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"softprobe-runtime/internal/store"
)

// HandleTraces accepts OTLP extract uploads and buffers them for capture sessions.
func HandleTraces(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid traces payload", http.StatusBadRequest)
			return
		}

		normalized, err := normalizeOTLPJSON(body)
		if err != nil {
			http.Error(w, "invalid traces payload", http.StatusBadRequest)
			return
		}

		req, err := parseExtractUploadRequest(normalized)
		if err != nil {
			http.Error(w, "invalid traces payload", http.StatusBadRequest)
			return
		}
		if req.SessionID == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}

		session, ok := st.Get(req.SessionID)
		if !ok {
			http.Error(w, "unknown session", http.StatusNotFound)
			return
		}

		if session.Mode == "capture" {
			_ = st.BufferExtract(req.SessionID, normalized)
			_, _ = st.RecordExtractedSpans(req.SessionID, req.SpanCount)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type extractUploadRequest struct {
	SessionID string
	SpanCount int
}

func parseExtractUploadRequest(payload []byte) (*extractUploadRequest, error) {
	var data tracev1.TracesData
	if err := protojson.Unmarshal(payload, &data); err != nil {
		return nil, err
	}

	req := &extractUploadRequest{}
	for _, resourceSpan := range data.ResourceSpans {
		for _, scopeSpan := range resourceSpan.ScopeSpans {
			for _, span := range scopeSpan.Spans {
				if spanAttrString(span, "sp.span.type") != "extract" {
					continue
				}

				req.SpanCount++
				if req.SessionID == "" {
					req.SessionID = spanAttrString(span, "sp.session.id")
				}
			}
		}
	}

	if req.SpanCount == 0 {
		return nil, errors.New("extract span not found")
	}

	return req, nil
}
