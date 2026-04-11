package runtimeapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSessionPersistsModeAndReturnsRevision(t *testing.T) {
	store := NewStore()
	mux := NewMux(store)

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

	session, ok := store.Get(resp.SessionID)
	if !ok {
		t.Fatalf("session %q not found in store", resp.SessionID)
	}
	if session.Mode != "replay" {
		t.Fatalf("mode = %q, want replay", session.Mode)
	}
}

func TestCloseSessionInvalidatesSubsequentControlCalls(t *testing.T) {
	store := NewStore()
	mux := NewMux(store)

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
	store := NewStore()
	mux := NewMux(store)

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

	session, ok := store.Get(created.SessionID)
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

	session, ok = store.Get(created.SessionID)
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
		inspect func(Session) []byte
	}{
		{
			name:  "policy",
			path:  "/policy",
			body1: `{"externalHttp":"strict"}`,
			body2: `{"externalHttp":"allow"}`,
			inspect: func(session Session) []byte {
				return session.Policy
			},
		},
		{
			name:  "rules",
			path:  "/rules",
			body1: `{"rules":[{"when":{"direction":"outbound"},"then":{"action":"passthrough"}}]}`,
			body2: `{"rules":[{"when":{"direction":"inbound"},"then":{"action":"passthrough"}}]}`,
			inspect: func(session Session) []byte {
				return session.Rules
			},
		},
		{
			name:  "fixtures",
			path:  "/fixtures/auth",
			body1: `{"tokens":["t1"]}`,
			body2: `{"tokens":["t2"]}`,
			inspect: func(session Session) []byte {
				return session.FixturesAuth
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore()
			mux := NewMux(store)

			createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"replay"}`))
			createRec := httptest.NewRecorder()
			mux.ServeHTTP(createRec, createReq)

			var created struct {
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
				t.Fatalf("unmarshal create response: %v", err)
			}

			update := func(body string) (int, Session) {
				req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+tc.path, bytes.NewBufferString(body))
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				session, _ := store.Get(created.SessionID)
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
