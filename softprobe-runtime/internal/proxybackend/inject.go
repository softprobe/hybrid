package proxybackend

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"softprobe-runtime/internal/store"
)

// InjectLookupRequest is the parsed OTLP inject lookup payload.
type InjectLookupRequest struct {
	SessionID        string
	ServiceName      string
	TrafficDirection string
	URLHost          string
	URLPath          string
	RequestMethod    string
	RequestHeaders   [][2]string
	RequestBody      string
}

type casePolicy struct {
	ExternalHTTP string `json:"externalHttp"`
}

// caseFileEnvelope holds only the fields the inject path still needs on the
// hot path. The runtime no longer walks `traces[]` to synthesize responses —
// case-time replay is materialized by the SDK via `findInCase` + `mockOutbound`
// (see `docs/design.md` §3.2.1 / §5.3). Case-embedded **rules** (§8.2) are
// still honored for frozen regression cases.
type caseFileEnvelope struct {
	Traces []json.RawMessage `json:"traces"`
	Rules  []injectRule      `json:"rules"`
}

// ParseInjectLookupRequest extracts the first inject span from OTLP JSON payloads.
func ParseInjectLookupRequest(payload []byte) (*InjectLookupRequest, error) {
	var data tracev1.TracesData
	if err := protojson.Unmarshal(payload, &data); err != nil {
		return nil, err
	}

	for _, resourceSpan := range data.ResourceSpans {
		serviceName := resourceAttrString(resourceSpan.Resource, "service.name")
		for _, scopeSpan := range resourceSpan.ScopeSpans {
			for _, span := range scopeSpan.Spans {
				if spanAttrString(span, "sp.span.type") != "inject" {
					continue
				}

				method := firstNonEmpty(
					spanAttrString(span, "http.request.method"),
					spanAttrString(span, "http.request.header.:method"),
				)
				urlPath := firstNonEmpty(
					spanAttrString(span, "url.path"),
					spanAttrString(span, "http.request.header.:path"),
				)

				return &InjectLookupRequest{
					SessionID:        spanAttrString(span, "sp.session.id"),
					ServiceName:      firstNonEmpty(spanAttrString(span, "sp.service.name"), serviceName),
					TrafficDirection: spanAttrString(span, "sp.traffic.direction"),
					URLHost:          spanAttrString(span, "url.host"),
					URLPath:          urlPath,
					RequestMethod:    method,
					RequestHeaders:   spanPrefixedStringAttrs(span, "http.request.header."),
					RequestBody:      spanAttrString(span, "http.request.body"),
				}, nil
			}
		}
	}

	return nil, errors.New("inject span not found")
}

// HandleInject resolves a proxy-side inject lookup by evaluating the session's
// mock rules (pushed via `PUT /v1/sessions/{id}/rules`) plus any inline rules
// embedded in the loaded case (frozen regression cases only). There is no
// case-trace walk on this hot path.
func HandleInject(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		payload, err := normalizeOTLPJSON(mustReadAll(r))
		if err != nil {
			http.Error(w, "invalid inject payload", http.StatusBadRequest)
			return
		}

		req, err := ParseInjectLookupRequest(payload)
		if err != nil {
			http.Error(w, "invalid inject payload", http.StatusBadRequest)
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

		sessionRules, err := parseInjectRulesDocument(session.Rules)
		if err != nil {
			http.Error(w, "invalid session rules", http.StatusInternalServerError)
			return
		}
		caseRules := caseEmbeddedRules(session.LoadedCase)

		match := selectInjectRule(req, isStrictExternalHTTPPolicy(session.Policy), caseRules, sessionRules)
		if match == nil {
			writeInjectMiss(w)
			return
		}

		switch match.Rule.Then.Action {
		case "mock":
			response := buildMockResponse(match.Rule)
			if response == nil {
				http.Error(w, "mock rule missing response", http.StatusInternalServerError)
				return
			}
			writeInjectHit(w, response)
		case "error":
			status, body := buildErrorResponse(match.Rule)
			writeInjectError(w, status, body)
		case "passthrough", "capture_only":
			// The proxy must perform the real outbound call; a miss tells it to
			// pass through. Capture semantics are handled by the extract path.
			writeInjectMiss(w)
		default:
			http.Error(w, "unsupported rule action", http.StatusInternalServerError)
		}
	}
}

func mustReadAll(r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	return body
}

// responseFromSpan materializes an OTLP extract span into a `MockResponse`.
// Retained because the capture path still reads this shape when persisting
// case files — the inject hot path no longer calls it.
func responseFromSpan(span *tracev1.Span) *MockResponse {
	var (
		statusCode  int
		headers     [][2]string
		body        string
		hasStatus   bool
		hasResponse bool
	)

	for _, attr := range span.Attributes {
		switch {
		case attr.Key == "http.response.status_code":
			code, ok := anyValueAsHTTPStatus(attr.Value)
			if ok {
				statusCode = code
				hasStatus = true
			}
		case len(attr.Key) > len("http.response.header.") && attr.Key[:len("http.response.header.")] == "http.response.header.":
			headers = append(headers, [2]string{attr.Key[len("http.response.header."):], anyValueString(attr.Value)})
			hasResponse = true
		case attr.Key == "http.response.body":
			body = anyValueString(attr.Value)
			hasResponse = true
		}
	}

	if !hasStatus && !hasResponse {
		return nil
	}
	if !hasStatus {
		statusCode = 200
	}

	return &MockResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}
}

// Silence unused warnings while responseFromSpan is kept only for the capture
// path (which lives elsewhere in the package). These are deliberate blanks to
// keep the function exported to the package for future reuse.
var (
	_ = responseFromSpan
	_ = (*resourcev1.Resource)(nil)
)

func writeInjectHit(w http.ResponseWriter, response *MockResponse) {
	body, err := encodeInjectResponseProto(response)
	if err != nil {
		http.Error(w, "encode inject response failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeInjectMiss(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "no inject match",
	})
}

func writeInjectError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func isStrictExternalHTTPPolicy(policy []byte) bool {
	if len(policy) == 0 {
		return false
	}

	var doc casePolicy
	if err := json.Unmarshal(policy, &doc); err != nil {
		return false
	}

	return doc.ExternalHTTP == "strict"
}
