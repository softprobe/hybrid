package controlapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"softprobe-runtime/internal/store"
)

func TestCreateSessionPersistsModeAndReturnsRevision(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"replay"}`))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatal("sessionId is empty")
	}
	if resp.SessionRevision != 0 {
		t.Fatalf("sessionRevision = %d, want 0", resp.SessionRevision)
	}

	session, ok := st.Get(resp.SessionID)
	if !ok {
		t.Fatalf("session %q not found in store", resp.SessionID)
	}
	if session.Mode != "replay" {
		t.Fatalf("mode = %q, want replay", session.Mode)
	}
}

func TestCloseSessionInvalidatesSubsequentControlCalls(t *testing.T) {
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

	closeReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+"/close", nil)
	closeRec := httptest.NewRecorder()
	mux.ServeHTTP(closeRec, closeReq)

	if closeRec.Code != http.StatusOK {
		t.Fatalf("close status = %d, want %d", closeRec.Code, http.StatusOK)
	}

	loadCaseReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+"/load-case", bytes.NewBufferString(`{}`))
	loadCaseRec := httptest.NewRecorder()
	mux.ServeHTTP(loadCaseRec, loadCaseReq)

	if loadCaseRec.Code != http.StatusNotFound {
		t.Fatalf("load-case status = %d, want %d after close", loadCaseRec.Code, http.StatusNotFound)
	}

	var errorBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(loadCaseRec.Body.Bytes(), &errorBody); err != nil {
		t.Fatalf("unmarshal error body: %v", err)
	}
	if errorBody.Error.Code != "unknown_session" {
		t.Fatalf("error code = %q, want unknown_session", errorBody.Error.Code)
	}
	if errorBody.Error.Message == "" {
		t.Fatal("error message is empty")
	}
}

func TestLoadCaseBumpsRevisionAndReplacesCase(t *testing.T) {
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

	firstCase := []byte(`{"version":"1.0.0","caseId":"example-minimal","traces":[]}`)
	loadCaseReq1 := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+"/load-case", bytes.NewReader(firstCase))
	loadCaseRec1 := httptest.NewRecorder()
	mux.ServeHTTP(loadCaseRec1, loadCaseReq1)

	if loadCaseRec1.Code != http.StatusOK {
		t.Fatalf("first load-case status = %d, want %d", loadCaseRec1.Code, http.StatusOK)
	}

	var firstResp struct {
		SessionRevision int `json:"sessionRevision"`
	}
	if err := json.Unmarshal(loadCaseRec1.Body.Bytes(), &firstResp); err != nil {
		t.Fatalf("unmarshal first load-case response: %v", err)
	}
	if firstResp.SessionRevision != 1 {
		t.Fatalf("first sessionRevision = %d, want 1", firstResp.SessionRevision)
	}

	session, ok := st.Get(created.SessionID)
	if !ok {
		t.Fatalf("session %q not found after first load-case", created.SessionID)
	}
	if session.Revision != 1 {
		t.Fatalf("stored revision = %d, want 1", session.Revision)
	}
	if !bytes.Contains(session.LoadedCase, []byte(`"caseId":"example-minimal"`)) {
		t.Fatalf("stored case missing example-minimal: %s", string(session.LoadedCase))
	}

	secondCase := []byte(`{"version":"1.0.0","caseId":"checkout-happy-path","traces":[{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"051581bf3cb55c13","attributes":[{"key":"sp.session.id","value":{"stringValue":"sess_checkout_001"}}]}]}]}]}]}`)
	loadCaseReq2 := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+"/load-case", bytes.NewReader(secondCase))
	loadCaseRec2 := httptest.NewRecorder()
	mux.ServeHTTP(loadCaseRec2, loadCaseReq2)

	if loadCaseRec2.Code != http.StatusOK {
		t.Fatalf("second load-case status = %d, want %d", loadCaseRec2.Code, http.StatusOK)
	}

	var secondResp struct {
		SessionRevision int `json:"sessionRevision"`
	}
	if err := json.Unmarshal(loadCaseRec2.Body.Bytes(), &secondResp); err != nil {
		t.Fatalf("unmarshal second load-case response: %v", err)
	}
	if secondResp.SessionRevision != 2 {
		t.Fatalf("second sessionRevision = %d, want 2", secondResp.SessionRevision)
	}

	session, ok = st.Get(created.SessionID)
	if !ok {
		t.Fatalf("session %q not found after second load-case", created.SessionID)
	}
	if session.Revision != 2 {
		t.Fatalf("stored revision after replace = %d, want 2", session.Revision)
	}
	if !bytes.Contains(session.LoadedCase, []byte(`"caseId":"checkout-happy-path"`)) {
		t.Fatalf("stored case missing checkout-happy-path: %s", string(session.LoadedCase))
	}
}

func TestPolicyRulesAndFixturesBumpRevision(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body1   string
		body2   string
		inspect func(store.Session) []byte
	}{
		{
			name:  "policy",
			path:  "/policy",
			body1: `{"externalHttp":"strict"}`,
			body2: `{"externalHttp":"allow"}`,
			inspect: func(session store.Session) []byte {
				return session.Policy
			},
		},
		{
			name:  "rules",
			path:  "/rules",
			body1: `{"rules":[{"name":"passthrough-out","when":{"direction":"outbound"},"then":{"action":"passthrough"}}]}`,
			body2: `{"rules":[{"name":"passthrough-in","when":{"direction":"inbound"},"then":{"action":"passthrough"}}]}`,
			inspect: func(session store.Session) []byte {
				return session.Rules
			},
		},
		{
			name:  "fixtures",
			path:  "/fixtures/auth",
			body1: `{"tokens":["t1"]}`,
			body2: `{"tokens":["t2"]}`,
			inspect: func(session store.Session) []byte {
				return session.FixturesAuth
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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

			update := func(body string) (int, store.Session) {
				req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+tc.path, bytes.NewBufferString(body))
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				session, _ := st.Get(created.SessionID)
				return rec.Code, session
			}

			code1, session1 := update(tc.body1)
			if code1 != http.StatusOK {
				t.Fatalf("first %s status = %d, want %d", tc.name, code1, http.StatusOK)
			}
			if session1.Revision != 1 {
				t.Fatalf("first %s revision = %d, want 1", tc.name, session1.Revision)
			}
			if !bytes.Equal(tc.inspect(session1), []byte(tc.body1)) {
				t.Fatalf("first %s payload not stored: %s", tc.name, string(tc.inspect(session1)))
			}

			code2, session2 := update(tc.body2)
			if code2 != http.StatusOK {
				t.Fatalf("second %s status = %d, want %d", tc.name, code2, http.StatusOK)
			}
			if session2.Revision != 2 {
				t.Fatalf("second %s revision = %d, want 2", tc.name, session2.Revision)
			}
			if !bytes.Equal(tc.inspect(session2), []byte(tc.body2)) {
				t.Fatalf("second %s payload not replaced: %s", tc.name, string(tc.inspect(session2)))
			}
		})
	}
}

func TestSessionStatsReturnsInjectHitCounters(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	sessionID := createSessionForStatsTest(t, mux, "replay")

	rulesDoc := `{
		"version": 1,
		"rules": [
			{
				"name": "fragment-mock",
				"when": {
					"direction": "outbound",
					"method": "GET",
					"path": "/fragment"
				},
				"then": {
					"action": "mock",
					"response": {
						"status": 200,
						"body": "{\"dep\":\"ok\"}"
					}
				}
			}
		]
	}`
	rulesReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sessionID+"/rules", bytes.NewBufferString(rulesDoc))
	rulesRec := httptest.NewRecorder()
	mux.ServeHTTP(rulesRec, rulesReq)
	if rulesRec.Code != http.StatusOK {
		t.Fatalf("apply rules status = %d, want 200", rulesRec.Code)
	}

	injectPayload := []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"inject"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"http.request.method","value":{"stringValue":"GET"}},
						{"key":"url.host","value":{"stringValue":"softprobe-proxy:8084"}},
						{"key":"url.path","value":{"stringValue":"/fragment"}}
					]
				}]
			}]
		}]
	}`, sessionID))
	injectReq := httptest.NewRequest(http.MethodPost, "/v1/inject", bytes.NewReader(mustProtoPayload(t, injectPayload)))
	injectReq.Header.Set("Content-Type", "application/x-protobuf")
	injectRec := httptest.NewRecorder()
	mux.ServeHTTP(injectRec, injectReq)
	if injectRec.Code != http.StatusOK {
		t.Fatalf("inject status = %d, want 200", injectRec.Code)
	}

	stats := getSessionStatsForTest(t, mux, sessionID)
	if stats.Stats.InjectedSpans != 1 {
		t.Fatalf("injectedSpans = %d, want 1", stats.Stats.InjectedSpans)
	}
	if stats.Stats.ExtractedSpans != 0 {
		t.Fatalf("extractedSpans = %d, want 0", stats.Stats.ExtractedSpans)
	}
	if stats.Stats.StrictMisses != 0 {
		t.Fatalf("strictMisses = %d, want 0", stats.Stats.StrictMisses)
	}
}

func TestSessionStatsReturnsCaptureExtractCounters(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	sessionID := createSessionForStatsTest(t, mux, "capture")

	tracePayload := []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"softprobe-proxy:8084"}},
						{"key":"url.path","value":{"stringValue":"/fragment"}},
						{"key":"http.response.status_code","value":{"intValue":200}}
					]
				}]
			}]
		}]
	}`, sessionID))
	traceReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(tracePayload))
	traceReq.Header.Set("Content-Type", "application/json")
	traceRec := httptest.NewRecorder()
	mux.ServeHTTP(traceRec, traceReq)
	if traceRec.Code != http.StatusNoContent {
		t.Fatalf("traces status = %d, want 204", traceRec.Code)
	}

	stats := getSessionStatsForTest(t, mux, sessionID)
	if stats.Stats.ExtractedSpans != 1 {
		t.Fatalf("extractedSpans = %d, want 1", stats.Stats.ExtractedSpans)
	}
	if stats.Stats.InjectedSpans != 0 {
		t.Fatalf("injectedSpans = %d, want 0", stats.Stats.InjectedSpans)
	}
	if stats.Stats.StrictMisses != 0 {
		t.Fatalf("strictMisses = %d, want 0", stats.Stats.StrictMisses)
	}
}

func TestSessionStatsReturnsStrictMissCounters(t *testing.T) {
	st := store.NewStore()
	mux := NewMux(st)

	sessionID := createSessionForStatsTest(t, mux, "replay")

	policyReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+sessionID+"/policy", bytes.NewBufferString(`{"externalHttp":"strict"}`))
	policyRec := httptest.NewRecorder()
	mux.ServeHTTP(policyRec, policyReq)
	if policyRec.Code != http.StatusOK {
		t.Fatalf("policy status = %d, want 200", policyRec.Code)
	}

	injectPayload := []byte(fmt.Sprintf(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"inject"}},
						{"key":"sp.session.id","value":{"stringValue":"%s"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"http.request.method","value":{"stringValue":"GET"}},
						{"key":"url.host","value":{"stringValue":"softprobe-proxy:8084"}},
						{"key":"url.path","value":{"stringValue":"/missing"}}
					]
				}]
			}]
		}]
	}`, sessionID))
	injectReq := httptest.NewRequest(http.MethodPost, "/v1/inject", bytes.NewReader(mustProtoPayload(t, injectPayload)))
	injectReq.Header.Set("Content-Type", "application/x-protobuf")
	injectRec := httptest.NewRecorder()
	mux.ServeHTTP(injectRec, injectReq)
	if injectRec.Code != http.StatusInternalServerError {
		t.Fatalf("inject status = %d, want 500", injectRec.Code)
	}

	stats := getSessionStatsForTest(t, mux, sessionID)
	if stats.Stats.StrictMisses != 1 {
		t.Fatalf("strictMisses = %d, want 1", stats.Stats.StrictMisses)
	}
	if stats.Stats.InjectedSpans != 0 {
		t.Fatalf("injectedSpans = %d, want 0", stats.Stats.InjectedSpans)
	}
	if stats.Stats.ExtractedSpans != 0 {
		t.Fatalf("extractedSpans = %d, want 0", stats.Stats.ExtractedSpans)
	}
}

func TestSessionStatsReturnsUnknownSessionError(t *testing.T) {
	mux := NewMux(store.NewStore())

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/missing/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error body: %v", err)
	}
	if body.Error.Code != "unknown_session" {
		t.Fatalf("error code = %q, want unknown_session", body.Error.Code)
	}
}

type sessionStatsTestResponse struct {
	SessionID       string `json:"sessionId"`
	SessionRevision int    `json:"sessionRevision"`
	Mode            string `json:"mode"`
	Stats           struct {
		InjectedSpans  int `json:"injectedSpans"`
		ExtractedSpans int `json:"extractedSpans"`
		StrictMisses   int `json:"strictMisses"`
	} `json:"stats"`
}

func createSessionForStatsTest(t *testing.T, mux *http.ServeMux, mode string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(fmt.Sprintf(`{"mode":%q}`, mode)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create session status = %d, want 200", rec.Code)
	}

	var body struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	return body.SessionID
}

func TestListSessions(t *testing.T) {
	mux := NewMux()

	// Empty list
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty list: status = %d", rec.Code)
	}
	var resp struct {
		Sessions []any `json:"sessions"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Sessions) != 0 {
		t.Errorf("sessions = %d, want 0", len(resp.Sessions))
	}

	// Create two sessions, then list
	createSessionForStatsTest(t, mux, "replay")
	createSessionForStatsTest(t, mux, "capture")

	req2 := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("populated list: status = %d", rec2.Code)
	}
	var resp2 struct {
		Sessions []map[string]any `json:"sessions"`
	}
	_ = json.NewDecoder(rec2.Body).Decode(&resp2)
	if len(resp2.Sessions) != 2 {
		t.Errorf("sessions = %d, want 2", len(resp2.Sessions))
	}
}

func getSessionStatsForTest(t *testing.T, mux *http.ServeMux, sessionID string) sessionStatsTestResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+sessionID+"/stats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body sessionStatsTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal stats response: %v", err)
	}
	if body.SessionID != sessionID {
		t.Fatalf("sessionId = %q, want %q", body.SessionID, sessionID)
	}
	return body
}
