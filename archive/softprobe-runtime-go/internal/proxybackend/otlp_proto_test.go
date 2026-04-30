package proxybackend

import (
	"strings"
	"testing"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestNormalizeOTLPJSONConvertsProtobufTraces(t *testing.T) {
	jsonPayload := []byte(`{
		"resourceSpans": [{
			"resource": {"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
			"scopeSpans": [{
				"spans": [{
					"name":"extract",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"sess_test"}},
						{"key":"url.path","value":{"stringValue":"/hello"}}
					]
				}]
			}]
		}]
	}`)

	var msg tracev1.TracesData
	if err := protojson.Unmarshal(jsonPayload, &msg); err != nil {
		t.Fatalf("unmarshal otlp json: %v", err)
	}
	payload, err := proto.Marshal(&msg)
	if err != nil {
		t.Fatalf("marshal otlp protobuf: %v", err)
	}

	normalized, err := normalizeOTLPJSON(payload)
	if err != nil {
		t.Fatalf("normalize protobuf payload: %v", err)
	}
	if !strings.Contains(string(normalized), `"sp.session.id"`) {
		t.Fatalf("normalized payload missing session id: %s", normalized)
	}
}

func TestNormalizeOTLPJSONAcceptsOTLPJSONPayload(t *testing.T) {
	jsonPayload := []byte(`{
		"resourceSpans": [{
			"resource": {"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
			"scopeSpans": [{
				"spans": [{
					"name":"extract",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"sess_json"}}
					]
				}]
			}]
		}]
	}`)

	normalized, err := normalizeOTLPJSON(jsonPayload)
	if err != nil {
		t.Fatalf("normalize json payload: %v", err)
	}
	var msg tracev1.TracesData
	if err := protojson.Unmarshal(normalized, &msg); err != nil {
		t.Fatalf("unmarshal normalized: %v", err)
	}
	req, err := parseExtractUploadRequest(normalized)
	if err != nil {
		t.Fatalf("parse extract: %v", err)
	}
	if req.SessionID != "sess_json" {
		t.Fatalf("session id = %q, want sess_json", req.SessionID)
	}
}

func TestParseExtractUploadRequestFromNormalizedProtobufTraces(t *testing.T) {
	jsonPayload := []byte(`{
		"resourceSpans": [{
			"resource": {"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
			"scopeSpans": [{
				"spans": [{
					"name":"extract",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"sess_test"}},
						{"key":"sp.service.name","value":{"stringValue":"checkout"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
						{"key":"url.path","value":{"stringValue":"/v1/payment_intents"}},
						{"key":"http.response.status_code","value":{"intValue":200}}
					]
				}]
			}]
		}]
	}`)

	var msg tracev1.TracesData
	if err := protojson.Unmarshal(jsonPayload, &msg); err != nil {
		t.Fatalf("unmarshal otlp json: %v", err)
	}
	payload, err := proto.Marshal(&msg)
	if err != nil {
		t.Fatalf("marshal otlp protobuf: %v", err)
	}

	normalized, err := normalizeOTLPJSON(payload)
	if err != nil {
		t.Fatalf("normalize protobuf payload: %v", err)
	}
	req, err := parseExtractUploadRequest(normalized)
	if err != nil {
		t.Fatalf("parse extract upload: %v", err)
	}
	if req.SessionID != "sess_test" {
		t.Fatalf("session id = %q, want sess_test", req.SessionID)
	}
}
