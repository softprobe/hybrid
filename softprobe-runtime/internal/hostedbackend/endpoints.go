package hostedbackend

import (
	"encoding/base64"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/datalake"
	"softprobe-runtime/internal/store"
)

// NewHostedEndpoints returns an http.ServeMux with the hosted-only endpoints.
//
//	GET /v1/captures/{captureId}   — export a capture JSON from datalake
func NewHostedEndpoints(st store.Store, datalakeClient interface {
	QuerySQL(context.Context, string) (datalake.SQLQueryResponse, error)
}) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/v1/captures/", handleGetCapture(st, datalakeClient))
	return mux
}

type sqlQuerier interface {
	QuerySQL(context.Context, string) (datalake.SQLQueryResponse, error)
}

func handleGetCapture(st store.Store, datalake sqlQuerier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = st
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		tenantInfo, ok := controlapi.TenantFromContext(r.Context())
		if !ok {
			writeAPIError(w, http.StatusUnauthorized, "missing_tenant", "authentication required")
			return
		}

		captureID := strings.TrimPrefix(r.URL.Path, "/v1/captures/")
		captureID = strings.TrimSuffix(captureID, "/")
		if captureID == "" {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "missing capture id")
			return
		}

		sql := captureQuerySQL(tenantInfo.TenantID, captureID)
		resp, err := datalake.QuerySQL(r.Context(), sql)
		if err != nil {
			slog.Error("hosted get-capture: query failed", "captureId", captureID, "tenantId", tenantInfo.TenantID, "error", err)
			writeAPIError(w, http.StatusInternalServerError, "storage_error", "failed to query capture")
			return
		}
		if resp.RowCount == 0 {
			writeAPIError(w, http.StatusNotFound, "not_found", "capture not found")
			return
		}

		out, err := buildCaptureJSON(captureID, resp.Columns, resp.Rows)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to build capture")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	})
}

func captureQuerySQL(tenantID, captureID string) string {
	tenantEscaped := strings.ReplaceAll(tenantID, "'", "''")
	captureEscaped := strings.ReplaceAll(captureID, "'", "''")
	return fmt.Sprintf(
		"SELECT timestamp, trace_id, span_id, parent_span_id, app_id, tenant_id, message_type, span_kind, http_request_method, http_request_path, http_request_headers, http_request_body, http_response_status_code, http_response_headers, http_response_body, attributes FROM union_spans WHERE attributes['sp.capture.id'] = '%s' AND attributes['sp.tenant.id'] = '%s' ORDER BY timestamp ASC",
		captureEscaped,
		tenantEscaped,
	)
}

func buildCaptureJSON(captureID string, columns []string, rows [][]json.RawMessage) ([]byte, error) {
	traces := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		entry := map[string]any{}
		for idx, col := range columns {
			if idx >= len(row) {
				continue
			}
			var val any
			if len(row[idx]) == 0 || string(row[idx]) == "null" {
				continue
			}
			if err := json.Unmarshal(row[idx], &val); err != nil {
				return nil, err
			}
			entry[col] = val
		}

		traceID, _ := asString(entry["trace_id"])
		spanID, _ := asString(entry["span_id"])
		parentSpanID, _ := asString(entry["parent_span_id"])
		method, _ := asString(entry["http_request_method"])
		path, _ := asString(entry["http_request_path"])
		fullURL := path
		body, _ := asString(entry["http_response_body"])
		msgType, _ := asString(entry["message_type"])
		direction := "outbound"

		statusCode := int64(0)
		if v, ok := entry["http_response_status_code"]; ok {
			switch n := v.(type) {
			case float64:
				statusCode = int64(n)
			case int64:
				statusCode = n
			}
		}
		if attrsMap, ok := entry["attributes"].(map[string]any); ok {
			method = firstNonEmpty(
				method,
				mapLookup(attrsMap, "http.request.method"),
				mapLookup(attrsMap, "http.request.header.:method"),
			)
			path = firstNonEmpty(
				path,
				mapLookup(attrsMap, "url.path"),
				mapLookup(attrsMap, "http.request.path"),
				mapLookup(attrsMap, "http.request.header.:path"),
			)
			fullURL = firstNonEmpty(fullURL, mapLookup(attrsMap, "url.full"))
			body = firstNonEmpty(body, mapLookup(attrsMap, "http.response.body"))
			direction = firstNonEmpty(direction, mapLookup(attrsMap, "sp.traffic.direction"))
			if statusCode == 0 {
				if s := mapLookup(attrsMap, "http.response.status_code"); s != "" {
					var parsed int64
					_, _ = fmt.Sscan(s, &parsed)
					if parsed > 0 {
						statusCode = parsed
					}
				}
			}
		}
		if path == "" && strings.HasPrefix(msgType, "/") {
			path = msgType
		}

		attrs := []map[string]any{
			kvString("sp.span.type", "extract"),
			kvString("sp.traffic.direction", direction),
			kvString("http.request.method", method),
			kvString("url.path", path),
			kvString("url.full", fullURL),
			kvString("http.response.body", body),
			kvInt("http.response.status_code", statusCode),
		}
		if msgType != "" {
			attrs = append(attrs, kvString("message.type", msgType))
		}

		trace := map[string]any{
			"resourceSpans": []map[string]any{
				{
					"scopeSpans": []map[string]any{
						{
							"spans": []map[string]any{
								{
									"traceId":      hexToBase64(traceID),
									"spanId":       hexToBase64(spanID),
									"parentSpanId": hexToBase64(parentSpanID),
									"name":         "sp.extract",
									"attributes":   attrs,
								},
							},
						},
					},
				},
			},
		}
		traces = append(traces, trace)
	}

	doc := map[string]any{
		"version":   "1.0.0",
		"caseId":    captureID,
		"captureId": captureID,
		"mode":      "capture",
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"traces":    traces,
	}
	return json.MarshalIndent(doc, "", "  ")
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func hexToBase64(s string) string {
	if s == "" {
		return ""
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return s
	}
	return base64.StdEncoding.EncodeToString(b)
}

func kvString(key, val string) map[string]any {
	return map[string]any{
		"key": key,
		"value": map[string]any{
			"stringValue": val,
		},
	}
}

func kvInt(key string, val int64) map[string]any {
	return map[string]any{
		"key": key,
		"value": map[string]any{
			"intValue": val,
		},
	}
}

func mapLookup(attrs map[string]any, key string) string {
	for k, v := range attrs {
		if strings.Contains(k, key) {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
