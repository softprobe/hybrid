package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// findInCaseSpanMatch is a Go port of the SDK's `findInCase` lookup
// (see softprobe-js/src/core/case/find-span.ts). The CLI needs the same
// logic so `softprobe suite run` can resolve `source: case` mocks and
// `source: case.ingress` requests without shelling out to the SDK.
//
// The shape returned here intentionally mirrors the JSON produced by the
// TypeScript helpers so the same sidecar contract serializes identically
// from either runner.
type capturedSpan struct {
	TraceID      string              `json:"traceId,omitempty"`
	SpanID       string              `json:"spanId,omitempty"`
	Name         string              `json:"name,omitempty"`
	// OTLP encodes span kind as a number (SPAN_KIND_SERVER=2, SPAN_KIND_CLIENT=3,
	// …), though some emitters write strings. We don't read Kind ourselves —
	// we discriminate on `sp.span.type` — so keep it permissive.
	Kind         json.RawMessage     `json:"kind,omitempty"`
	ParentSpanID string              `json:"parentSpanId,omitempty"`
	Attributes   []otlpAttribute     `json:"attributes,omitempty"`
	_service     string              // derived from parent resource, not serialized
}

type otlpAttribute struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type capturedResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type capturedRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url,omitempty"`
	Path    string            `json:"path"`
	Host    string            `json:"host,omitempty"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type capturedHit struct {
	Span     capturedSpan     `json:"span"`
	Response capturedResponse `json:"response"`
	Request  capturedRequest  `json:"request"`
}

// caseSpanPredicate matches the CaseSpanPredicate interface from the
// TypeScript SDK. Every set field is AND'd together.
type caseSpanPredicate struct {
	Direction  string
	Service    string
	Host       string
	HostSuffix string
	Method     string
	Path       string
	PathPrefix string
}

func predicateFromSuiteMatch(m suiteMockMatch) caseSpanPredicate {
	return caseSpanPredicate{
		Direction:  m.Direction,
		Service:    m.Service,
		Host:       m.Host,
		HostSuffix: m.HostSuffix,
		Method:     m.Method,
		Path:       m.Path,
		PathPrefix: m.PathPrefix,
	}
}

// loadCaseSpans decodes a case document (as bytes) into a flat list of
// spans, carrying along the resource-level service name. We re-parse the
// JSON loosely because `main.go`'s `caseDocument` type flattens attribute
// values, losing the intValue variant we need for status codes.
func loadCaseSpans(caseBytes []byte) ([]capturedSpan, error) {
	var doc struct {
		Traces []struct {
			ResourceSpans []struct {
				Resource struct {
					Attributes []otlpAttribute `json:"attributes"`
				} `json:"resource"`
				ScopeSpans []struct {
					Spans []capturedSpan `json:"spans"`
				} `json:"scopeSpans"`
			} `json:"resourceSpans"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(caseBytes, &doc); err != nil {
		return nil, fmt.Errorf("parse case: %w", err)
	}
	var out []capturedSpan
	for _, trace := range doc.Traces {
		for _, rs := range trace.ResourceSpans {
			serviceName := readOTLPString(rs.Resource.Attributes, "service.name")
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					sp._service = serviceName
					out = append(out, sp)
				}
			}
		}
	}
	return out, nil
}

// findCaseSpans returns every inject/extract span that matches the
// predicate. Callers decide what to do with zero/one/many matches.
func findCaseSpans(spans []capturedSpan, pred caseSpanPredicate) []capturedSpan {
	var hits []capturedSpan
	for _, sp := range spans {
		if spanSatisfies(sp, pred) {
			hits = append(hits, sp)
		}
	}
	return hits
}

// findCaseSpanOne is the CLI equivalent of SDK `findInCase`: zero matches
// or multiple matches are errors so suite authors hit the ambiguity at
// authoring time, not in the middle of a production-triggered rerun.
func findCaseSpanOne(spans []capturedSpan, pred caseSpanPredicate) (capturedSpan, error) {
	hits := findCaseSpans(spans, pred)
	if len(hits) == 0 {
		return capturedSpan{}, fmt.Errorf("no span in case matches %s", formatPredicate(pred))
	}
	if len(hits) > 1 {
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			if h.SpanID != "" {
				ids = append(ids, h.SpanID)
			}
		}
		return capturedSpan{}, fmt.Errorf("%d spans match %s (span ids: %s) — disambiguate the predicate",
			len(hits), formatPredicate(pred), strings.Join(ids, ","))
	}
	return hits[0], nil
}

func spanSatisfies(sp capturedSpan, pred caseSpanPredicate) bool {
	spanType := readOTLPString(sp.Attributes, "sp.span.type")
	if spanType != "inject" && spanType != "extract" {
		return false
	}
	if pred.Direction != "" && readOTLPString(sp.Attributes, "sp.traffic.direction") != pred.Direction {
		return false
	}
	method := readOTLPString(sp.Attributes, "http.request.method")
	if method == "" {
		method = readOTLPString(sp.Attributes, "http.request.header.:method")
	}
	if pred.Method != "" && method != pred.Method {
		return false
	}
	path := readOTLPString(sp.Attributes, "url.path")
	if path == "" {
		path = readOTLPString(sp.Attributes, "http.request.header.:path")
	}
	if pred.Path != "" && path != pred.Path {
		return false
	}
	if pred.PathPrefix != "" && !strings.HasPrefix(path, pred.PathPrefix) {
		return false
	}
	host := readOTLPString(sp.Attributes, "url.host")
	if pred.Host != "" && host != pred.Host {
		return false
	}
	if pred.HostSuffix != "" && !strings.HasSuffix(host, pred.HostSuffix) {
		return false
	}
	service := readOTLPString(sp.Attributes, "sp.service.name")
	if service == "" {
		service = sp._service
	}
	if pred.Service != "" && service != pred.Service {
		return false
	}
	return true
}

// responseFromSpan decodes a captured span into the runtime's mock-response
// shape. Mirrors softprobe-js's `responseFromSpan`.
func responseFromSpan(sp capturedSpan) (capturedResponse, error) {
	resp := capturedResponse{Headers: map[string]string{}}
	var status int
	var hasStatus bool
	for _, attr := range sp.Attributes {
		switch {
		case attr.Key == "http.response.status_code":
			n, ok := parseOTLPInt(attr.Value)
			if ok {
				status = n
				hasStatus = true
			}
		case attr.Key == "http.response.body":
			resp.Body = otlpValueToString(attr.Value)
		case strings.HasPrefix(attr.Key, "http.response.header."):
			name := strings.TrimPrefix(attr.Key, "http.response.header.")
			resp.Headers[name] = otlpValueToString(attr.Value)
		}
	}
	if !hasStatus {
		return capturedResponse{}, fmt.Errorf("captured span %s missing http.response.status_code", sp.SpanID)
	}
	resp.Status = status
	return resp, nil
}

// requestFromSpan decodes the captured request side of a span — used when
// `request.source: case.ingress` is set on the suite. Missing pieces
// degrade gracefully to empty strings.
func requestFromSpan(sp capturedSpan) capturedRequest {
	req := capturedRequest{Headers: map[string]string{}}
	for _, attr := range sp.Attributes {
		switch {
		case attr.Key == "http.request.method":
			req.Method = otlpValueToString(attr.Value)
		case attr.Key == "http.request.body":
			req.Body = otlpValueToString(attr.Value)
		case attr.Key == "url.path":
			req.Path = otlpValueToString(attr.Value)
		case attr.Key == "url.host":
			req.Host = otlpValueToString(attr.Value)
		case attr.Key == "url.full":
			req.URL = otlpValueToString(attr.Value)
		case strings.HasPrefix(attr.Key, "http.request.header."):
			name := strings.TrimPrefix(attr.Key, "http.request.header.")
			if strings.HasPrefix(name, ":") {
				// pseudo-headers like :method / :path are already lifted above
				continue
			}
			req.Headers[name] = otlpValueToString(attr.Value)
		}
	}
	return req
}

func readOTLPString(attrs []otlpAttribute, key string) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return otlpValueToString(attr.Value)
		}
	}
	return ""
}

func otlpValueToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper) > 0 {
		for _, key := range []string{"stringValue", "intValue", "doubleValue", "boolValue"} {
			if v, ok := wrapper[key]; ok {
				var s string
				if err := json.Unmarshal(v, &s); err == nil {
					return s
				}
				var n json.Number
				if err := json.Unmarshal(v, &n); err == nil {
					return n.String()
				}
				var b bool
				if err := json.Unmarshal(v, &b); err == nil {
					return strconv.FormatBool(b)
				}
			}
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

func parseOTLPInt(raw json.RawMessage) (int, bool) {
	s := otlpValueToString(raw)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func formatPredicate(p caseSpanPredicate) string {
	parts := []string{}
	if p.Direction != "" {
		parts = append(parts, fmt.Sprintf("direction=%q", p.Direction))
	}
	if p.Method != "" {
		parts = append(parts, fmt.Sprintf("method=%q", p.Method))
	}
	if p.Path != "" {
		parts = append(parts, fmt.Sprintf("path=%q", p.Path))
	}
	if p.PathPrefix != "" {
		parts = append(parts, fmt.Sprintf("pathPrefix=%q", p.PathPrefix))
	}
	if p.Host != "" {
		parts = append(parts, fmt.Sprintf("host=%q", p.Host))
	}
	if p.HostSuffix != "" {
		parts = append(parts, fmt.Sprintf("hostSuffix=%q", p.HostSuffix))
	}
	if p.Service != "" {
		parts = append(parts, fmt.Sprintf("service=%q", p.Service))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}
