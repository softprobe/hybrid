package controlapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"softprobe-runtime/internal/store"
)

func TestTracesRouteBuffersCapturePayloads(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"capture"}`))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	payload := []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"resource": {"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
			"scopeSpans": [{
				"spans": [{
					"name":"extract",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.service.name","value":{"stringValue":"checkout"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
						{"key":"url.path","value":{"stringValue":"/v1/payment_intents"}},
						{"key":"http.response.status_code","value":{"intValue":200}}
					]
				}]
			}]
		}]
	}`, created.SessionID))

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code/100 != 2 {
		t.Fatalf("status = %d, want 2xx", rec.Code)
	}

	session, ok := st.Get(created.SessionID)
	if !ok {
		t.Fatalf("session %q not found", created.SessionID)
	}
	if len(session.Extracts) != 1 {
		t.Fatalf("extract buffer length = %d, want 1", len(session.Extracts))
	}
	var buffered struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []struct {
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							String *string `json:"stringValue,omitempty"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal(session.Extracts[0], &buffered); err != nil {
		t.Fatalf("unmarshal buffered payload: %v", err)
	}
	if len(buffered.ResourceSpans) == 0 || len(buffered.ResourceSpans[0].ScopeSpans) == 0 || len(buffered.ResourceSpans[0].ScopeSpans[0].Spans) == 0 {
		t.Fatalf("buffered payload missing spans: %s", string(session.Extracts[0]))
	}
}

func TestTracesRouteAcceptsProtobufCapturePayloads(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"capture"}`))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	jsonPayload := fmt.Sprintf(`{
		"resourceSpans": [{
			"resource": {"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
			"scopeSpans": [{
				"spans": [{
					"name":"extract",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.service.name","value":{"stringValue":"checkout"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
						{"key":"url.path","value":{"stringValue":"/v1/payment_intents"}},
						{"key":"http.response.status_code","value":{"intValue":200}}
					]
				}]
			}]
		}]
	}`, created.SessionID)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(mustProtoTracePayload(t, []byte(jsonPayload))))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code/100 != 2 {
		t.Fatalf("status = %d, want 2xx: %s", rec.Code, rec.Body.String())
	}

	session, ok := st.Get(created.SessionID)
	if !ok {
		t.Fatalf("session %q not found", created.SessionID)
	}
	if len(session.Extracts) != 1 {
		t.Fatalf("extract buffer length = %d, want 1", len(session.Extracts))
	}

	var buffered tracev1.TracesData
	if err := protojson.Unmarshal(session.Extracts[0], &buffered); err != nil {
		t.Fatalf("unmarshal buffered payload: %v", err)
	}
	if len(buffered.ResourceSpans) == 0 || len(buffered.ResourceSpans[0].ScopeSpans) == 0 || len(buffered.ResourceSpans[0].ScopeSpans[0].Spans) == 0 {
		t.Fatalf("buffered payload missing spans: %s", string(session.Extracts[0]))
	}
}

func mustProtoTracePayload(t *testing.T, jsonPayload []byte) []byte {
	t.Helper()

	var msg tracev1.TracesData
	if err := protojson.Unmarshal(jsonPayload, &msg); err != nil {
		t.Fatalf("unmarshal otlp json: %v", err)
	}

	payload, err := proto.Marshal(&msg)
	if err != nil {
		t.Fatalf("marshal otlp protobuf: %v", err)
	}
	return payload
}
