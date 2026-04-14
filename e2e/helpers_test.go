package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// otlpJSONInt64 unmarshals OTLP JSON intValue, which may be a number or a decimal string (protojson).
type otlpJSONInt64 struct {
	V *int64
}

func (o *otlpJSONInt64) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		o.V = nil
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		o.V = &n
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("intValue: %w", err)
	}
	parsed, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("intValue %q: %w", s, err)
	}
	o.V = &parsed
	return nil
}

// spanDoc is a minimal OTLP JSON span shape for e2e validation (official JSON field names).
type spanDoc struct {
	TraceID      string `json:"traceId"`
	SpanID       string `json:"spanId"`
	ParentSpanID string `json:"parentSpanId"`
	Attributes   []struct {
		Key   string `json:"key"`
		Value struct {
			Int    otlpJSONInt64 `json:"intValue,omitempty"`
			String *string       `json:"stringValue,omitempty"`
		} `json:"value"`
	} `json:"attributes"`
}

type caseFileDoc struct {
	Traces []struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []spanDoc `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	} `json:"traces"`
}

func attrString(sp *spanDoc, key string) string {
	for _, a := range sp.Attributes {
		if a.Key == key && a.Value.String != nil {
			return *a.Value.String
		}
	}
	return ""
}

func captureCaseTraceSemanticsError(caseFile string) error {
	data, err := os.ReadFile(caseFile)
	if err != nil {
		return err
	}
	if bytes.Contains(bytes.ToLower(data), append([]byte("x-sp-"), []byte("traceparent")...)) {
		return errors.New("case file contains legacy tracestate traceparent key; regenerate capture with updated proxy")
	}
	var doc caseFileDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}

	var helloSpan, fragmentSpan *spanDoc
	for i := range doc.Traces {
		for j := range doc.Traces[i].ResourceSpans {
			for k := range doc.Traces[i].ResourceSpans[j].ScopeSpans {
				for s := range doc.Traces[i].ResourceSpans[j].ScopeSpans[k].Spans {
					sp := &doc.Traces[i].ResourceSpans[j].ScopeSpans[k].Spans[s]
					if attrString(sp, "sp.span.type") != "extract" {
						continue
					}
					switch attrString(sp, "url.path") {
					case "/hello":
						helloSpan = sp
					case "/fragment":
						fragmentSpan = sp
					}
				}
			}
		}
	}
	if helloSpan == nil {
		return errors.New("no extract span for /hello")
	}
	if fragmentSpan == nil {
		return errors.New("no extract span for /fragment")
	}

	// Ingress /hello: test client often has no W3C context — require trace + span ids only.
	if _, err := checkSpanIDBytes("ingress /hello traceId", helloSpan.TraceID, 16); err != nil {
		return err
	}
	helloSelf, err := checkSpanIDBytes("ingress /hello spanId", helloSpan.SpanID, 8)
	if err != nil {
		return err
	}
	if helloSpan.ParentSpanID != "" {
		parent, err := checkSpanIDBytes("ingress /hello parentSpanId", helloSpan.ParentSpanID, 8)
		if err != nil {
			return err
		}
		if string(parent) == string(helloSelf) {
			return errors.New("ingress /hello: parentSpanId equals spanId")
		}
	}
	if attrString(helloSpan, "sp.traffic.direction") != "outbound" {
		return fmt.Errorf("ingress /hello: sp.traffic.direction = %q, want outbound", attrString(helloSpan, "sp.traffic.direction"))
	}

	// Egress /fragment: app uses OTel; proxy must record W3C parent on the outbound hop.
	if _, err := checkSpanIDBytes("egress /fragment traceId", fragmentSpan.TraceID, 16); err != nil {
		return err
	}
	self, err := checkSpanIDBytes("egress /fragment spanId", fragmentSpan.SpanID, 8)
	if err != nil {
		return err
	}
	if fragmentSpan.ParentSpanID == "" {
		return errors.New("egress /fragment: missing parentSpanId (expected W3C parent from app outbound context)")
	}
	parent, err := checkSpanIDBytes("egress /fragment parentSpanId", fragmentSpan.ParentSpanID, 8)
	if err != nil {
		return err
	}
	if string(parent) == string(self) {
		return errors.New("egress /fragment: parentSpanId equals spanId")
	}
	if attrString(fragmentSpan, "sp.traffic.direction") != "outbound" {
		return fmt.Errorf("egress /fragment: sp.traffic.direction = %q, want outbound", attrString(fragmentSpan, "sp.traffic.direction"))
	}

	return nil
}

func checkSpanIDBytes(field, enc string, wantLen int) ([]byte, error) {
	if enc == "" {
		return nil, fmt.Errorf("%s: missing or empty", field)
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("%s: base64: %w", field, err)
	}
	if len(raw) != wantLen {
		return nil, fmt.Errorf("%s: decoded length %d, want %d", field, len(raw), wantLen)
	}
	return raw, nil
}

// validateCaptureCaseTraceSemantics checks OTLP identity fields for ingress (/hello) and egress (/fragment)
// extract spans: valid trace id (16 bytes), span id (8 bytes), parent span id (8 bytes) when W3C context exists.
func validateCaptureCaseTraceSemantics(t *testing.T, caseFile string) {
	t.Helper()
	if err := captureCaseTraceSemanticsError(caseFile); err != nil {
		t.Fatalf("trace semantics: %v", err)
	}
}

func egressURLFromCapturedCase(caseFile string) (string, bool) {
	data, err := os.ReadFile(caseFile)
	if err != nil {
		return "", false
	}
	var doc caseFileDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", false
	}
	for _, tr := range doc.Traces {
		for _, rs := range tr.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for i := range ss.Spans {
					sp := &ss.Spans[i]
					if attrString(sp, "url.path") != "/fragment" {
						continue
					}
					host := attrString(sp, "url.host")
					if host == "" {
						return "", false
					}
					return "http://" + host, true
				}
			}
		}
	}
	return "", false
}

func fragmentResponseBodyFromCase(caseFile string) (string, bool) {
	data, err := os.ReadFile(caseFile)
	if err != nil {
		return "", false
	}
	var doc caseFileDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", false
	}
	for _, tr := range doc.Traces {
		for _, rs := range tr.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for i := range ss.Spans {
					sp := &ss.Spans[i]
					if attrString(sp, "url.path") != "/fragment" {
						continue
					}
					body := attrString(sp, "http.response.body")
					if body == "" {
						return "", false
					}
					return body, true
				}
			}
		}
	}
	return "", false
}

func egressProxyURLForEnv(defaultHostPort string) string {
	if v := os.Getenv("EGRESS_PROXY_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return defaultHostPort
}

// egressHTTPBaseForTest returns the base URL for the egress Envoy listener.
// Captured cases record url.host as seen from inside docker-compose (e.g. softprobe-proxy:8084);
// `go test` on the host uses published ports on 127.0.0.1 unless EGRESS_PROXY_URL is set.
func egressHTTPBaseForTest(caseFile string) string {
	if v := strings.TrimSpace(os.Getenv("EGRESS_PROXY_URL")); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	fromCase, ok := egressURLFromCapturedCase(caseFile)
	if !ok {
		return "http://127.0.0.1:8084"
	}
	u, err := url.Parse(fromCase)
	if err != nil {
		return "http://127.0.0.1:8084"
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "8084"
	}
	if strings.EqualFold(host, "softprobe-proxy") {
		return fmt.Sprintf("http://127.0.0.1:%s", port)
	}
	return strings.TrimSuffix(fromCase, "/")
}
