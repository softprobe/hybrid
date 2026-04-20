package proxybackend

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"softprobe-runtime/internal/store"
)

func TestHandleInjectReturnsMockWhenSessionRuleMatches(t *testing.T) {
	st := store.NewStore()
	session := st.Create("replay")

	rulesDoc := `{
		"version": 1,
		"rules": [
			{
				"when": {"direction": "outbound", "method": "GET", "path": "/fragment"},
				"then": {
					"action": "mock",
					"response": {
						"status": 200,
						"headers": {"content-type": "application/json"},
						"body": "{\"dep\":\"ok\"}"
					}
				}
			}
		]
	}`
	if _, ok := st.ApplyRules(session.ID, []byte(rulesDoc)); !ok {
		t.Fatalf("apply rules failed")
	}

	payload := buildInjectLookupPayload(t, session.ID, "GET", "/fragment")
	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/inject", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	HandleInject(st)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/x-protobuf" {
		t.Fatalf("content-type = %q, want application/x-protobuf", ct)
	}
}

func TestHandleInjectReturnsMissWithoutRules(t *testing.T) {
	st := store.NewStore()
	session := st.Create("replay")

	payload := buildInjectLookupPayload(t, session.ID, "GET", "/fragment")
	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/inject", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	HandleInject(st)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (miss)", rr.Code)
	}
}

func TestHandleInjectAppliesStrictPolicyErrorRule(t *testing.T) {
	st := store.NewStore()
	session := st.Create("replay")

	if _, ok := st.ApplyPolicy(session.ID, []byte(`{"externalHttp":"strict"}`)); !ok {
		t.Fatalf("apply policy failed")
	}

	payload := buildInjectLookupPayload(t, session.ID, "GET", "/fragment")
	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/inject", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	HandleInject(st)(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s, want 500 from strict policy", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "strict policy requires") {
		t.Fatalf("body = %s, want strict policy error", rr.Body.String())
	}
}

func TestHandleInjectHonorsCaseEmbeddedRule(t *testing.T) {
	st := store.NewStore()
	session := st.Create("replay")

	caseDoc := `{
		"version": "1.0.0",
		"caseId": "case-with-rules",
		"traces": [],
		"rules": [
			{
				"when": {"direction": "outbound", "path": "/fragment"},
				"then": {
					"action": "mock",
					"response": {"status": 200, "body": "case-body"}
				}
			}
		]
	}`
	if _, ok := st.LoadCase(session.ID, []byte(caseDoc)); !ok {
		t.Fatalf("load case failed")
	}

	payload := buildInjectLookupPayload(t, session.ID, "GET", "/fragment")
	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/inject", strings.NewReader(payload))
	rr := httptest.NewRecorder()

	HandleInject(st)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200 from case rule", rr.Code, rr.Body.String())
	}
}

func TestStrictExternalHTTPPolicyIsDetected(t *testing.T) {
	if !isStrictExternalHTTPPolicy([]byte(`{"externalHttp":"strict"}`)) {
		t.Fatal("expected strict externalHttp policy to be detected")
	}
	if isStrictExternalHTTPPolicy([]byte(`{"externalHttp":"allow"}`)) {
		t.Fatal("allow policy must not be treated as strict")
	}
}

// buildInjectLookupPayload returns an OTLP JSON document containing a single
// inject span with the requested method / path attributes.
func buildInjectLookupPayload(t *testing.T, sessionID, method, urlPath string) string {
	t.Helper()
	doc := map[string]any{
		"resourceSpans": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						{"key": "service.name", "value": map[string]any{"stringValue": "checkout"}},
					},
				},
				"scopeSpans": []map[string]any{
					{
						"spans": []map[string]any{
							{
								"name": "inject lookup",
								"attributes": []map[string]any{
									{"key": "sp.span.type", "value": map[string]any{"stringValue": "inject"}},
									{"key": "sp.session.id", "value": map[string]any{"stringValue": sessionID}},
									{"key": "sp.traffic.direction", "value": map[string]any{"stringValue": "outbound"}},
									{"key": "sp.service.name", "value": map[string]any{"stringValue": "checkout"}},
									{"key": "http.request.method", "value": map[string]any{"stringValue": method}},
									{"key": "url.path", "value": map[string]any{"stringValue": urlPath}},
									{"key": "url.host", "value": map[string]any{"stringValue": "softprobe-proxy:8084"}},
								},
							},
						},
					},
				},
			},
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(raw)
}

// ensure io import stays used in case future tests need response-body reads
var _ = io.Discard
