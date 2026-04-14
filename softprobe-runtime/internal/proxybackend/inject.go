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
	RequestHeaders   [][2]string
	RequestBody      string
}

type casePolicy struct {
	ExternalHTTP string `json:"externalHttp"`
}

type caseFileEnvelope struct {
	Traces []json.RawMessage `json:"traces"`
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

				return &InjectLookupRequest{
					SessionID:        spanAttrString(span, "sp.session.id"),
					ServiceName:      firstNonEmpty(spanAttrString(span, "sp.service.name"), serviceName),
					TrafficDirection: spanAttrString(span, "sp.traffic.direction"),
					URLHost:          spanAttrString(span, "url.host"),
					URLPath:          spanAttrString(span, "url.path"),
					RequestHeaders:   spanPrefixedStringAttrs(span, "http.request.header."),
					RequestBody:      spanAttrString(span, "http.request.body"),
				}, nil
			}
		}
	}

	return nil, errors.New("inject span not found")
}

// HandleInject parses the OTLP request and returns a miss until hit resolution is implemented.
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
		if _, ok := st.Get(req.SessionID); !ok {
			http.Error(w, "unknown session", http.StatusNotFound)
			return
		}

		session, _ := st.Get(req.SessionID)
		if response, ok := replayResponseFromCase(session.LoadedCase, req); ok {
			writeInjectHit(w, response)
			return
		}

		if isStrictExternalHTTPPolicy(session.Policy) {
			writeInjectError(w, http.StatusInternalServerError, "strict policy requires a mock or replay match")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "no inject match",
		})
	}
}

func mustReadAll(r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	return body
}

func replayResponseFromCase(caseBytes []byte, req *InjectLookupRequest) (*MockResponse, bool) {
	var env caseFileEnvelope
	if err := json.Unmarshal(caseBytes, &env); err != nil {
		return nil, false
	}

	for _, rawTrace := range env.Traces {
		var td tracev1.TracesData
		if err := protojson.Unmarshal(rawTrace, &td); err != nil {
			continue
		}

		for _, resourceSpan := range td.ResourceSpans {
			for _, scopeSpan := range resourceSpan.ScopeSpans {
				for _, span := range scopeSpan.Spans {
					if !isMatchingInjectSpan(span, resourceSpan.Resource, req) {
						continue
					}

					response := responseFromSpan(span)
					if response == nil {
						return nil, false
					}
					return response, true
				}
			}
		}
	}

	return nil, false
}

func isMatchingInjectSpan(span *tracev1.Span, resource *resourcev1.Resource, req *InjectLookupRequest) bool {
	spanType := spanAttrString(span, "sp.span.type")
	if spanType != "inject" && spanType != "extract" {
		return false
	}
	if req.TrafficDirection != "" && spanAttrString(span, "sp.traffic.direction") != req.TrafficDirection {
		return false
	}
	if req.URLHost != "" && spanAttrString(span, "url.host") != req.URLHost {
		return false
	}
	if req.URLPath != "" && spanAttrString(span, "url.path") != req.URLPath {
		return false
	}
	if req.ServiceName != "" && firstNonEmpty(spanAttrString(span, "sp.service.name"), resourceAttrString(resource, "service.name")) != req.ServiceName {
		return false
	}
	return true
}

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
