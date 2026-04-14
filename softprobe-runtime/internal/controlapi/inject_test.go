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

func TestInjectRouteParsesSessionAndIdentity(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"replay"}`))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	payload := []byte(`{
		"resourceSpans": [{
			"resource": {
				"attributes": [
					{"key":"service.name","value":{"stringValue":"checkout"}}
				]
			},
			"scopeSpans": [{
				"spans": [{
					"name":"inject",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"inject"}},
						{"key":"sp.session.id","value":{"stringValue":"` + created.SessionID + `"}},
						{"key":"sp.service.name","value":{"stringValue":"checkout"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
						{"key":"url.path","value":{"stringValue":"/v1/payment_intents"}}
					]
				}]
			}]
		}]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/inject", bytes.NewReader(mustProtoPayload(t, payload)))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("not implemented")) {
		t.Fatalf("inject route still returned stub body: %s", rec.Body.Bytes())
	}
}

func TestInjectRouteReturnsHitAndMiss(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"replay"}`))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	caseJSON := fmt.Sprintf(`{
		"version":"1.0.0",
		"caseId":"checkout-happy-path",
		"traces":[{
			"resourceSpans":[{
				"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},
				"scopeSpans":[{
					"spans":[{
						"name":"HTTP POST",
						"attributes":[
							{"key":"sp.span.type","value":{"stringValue":"inject"}},
							{"key":"sp.session.id","value":{"stringValue":"%s"}},
							{"key":"sp.service.name","value":{"stringValue":"checkout"}},
							{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
							{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
							{"key":"url.path","value":{"stringValue":"/v1/payment_intents"}},
							{"key":"http.response.status_code","value":{"intValue":200}},
							{"key":"http.response.header.content-type","value":{"stringValue":"application/json"}},
							{"key":"http.response.body","value":{"stringValue":"{\"ok\":true}"}}
						]
					}]
				}]
			}]
		}]
	}`, created.SessionID)

	loadReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+"/load-case", bytes.NewBufferString(caseJSON))
	loadRec := httptest.NewRecorder()
	mux.ServeHTTP(loadRec, loadReq)
	if loadRec.Code != http.StatusOK {
		t.Fatalf("load-case status = %d, want 200", loadRec.Code)
	}

	hitPayload := []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"resource": {
				"attributes": [
					{"key":"service.name","value":{"stringValue":"checkout"}}
				]
			},
			"scopeSpans": [{
				"spans": [{
					"name":"inject",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"inject"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.service.name","value":{"stringValue":"checkout"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
						{"key":"url.path","value":{"stringValue":"/v1/payment_intents"}}
					]
				}]
			}]
		}]
	}`, created.SessionID))

	hitReq := httptest.NewRequest(http.MethodPost, "/v1/inject", bytes.NewReader(mustProtoPayload(t, hitPayload)))
	hitReq.Header.Set("Content-Type", "application/x-protobuf")
	hitRec := httptest.NewRecorder()
	mux.ServeHTTP(hitRec, hitReq)

	if hitRec.Code != http.StatusOK {
		t.Fatalf("hit status = %d, want 200", hitRec.Code)
	}

	var hitBody tracev1.TracesData
	if err := proto.Unmarshal(hitRec.Body.Bytes(), &hitBody); err != nil {
		t.Fatalf("unmarshal hit body: %v", err)
	}
	if len(hitBody.ResourceSpans) == 0 || len(hitBody.ResourceSpans[0].ScopeSpans) == 0 || len(hitBody.ResourceSpans[0].ScopeSpans[0].Spans) == 0 {
		t.Fatalf("hit body missing spans: %s", hitRec.Body.Bytes())
	}

	attrs := hitBody.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes
	if attrs[0].Key != "http.response.status_code" || attrs[0].Value == nil || attrs[0].Value.GetIntValue() != 200 {
		t.Fatalf("status attribute = %+v, want 200", attrs[0])
	}

	missPayload := []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"inject"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
						{"key":"url.path","value":{"stringValue":"/different"}}
					]
				}]
			}]
		}]
	}`, created.SessionID))
	missReq := httptest.NewRequest(http.MethodPost, "/v1/inject", bytes.NewReader(mustProtoPayload(t, missPayload)))
	missReq.Header.Set("Content-Type", "application/x-protobuf")
	missRec := httptest.NewRecorder()
	mux.ServeHTTP(missRec, missReq)

	if missRec.Code != http.StatusNotFound {
		t.Fatalf("miss status = %d, want 404", missRec.Code)
	}
}

func mustProtoPayload(t *testing.T, jsonPayload []byte) []byte {
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
