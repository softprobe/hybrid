// Package hosted contains smoke tests for the hosted softprobe-runtime.
// Run these tests against the deployed instance by setting SOFTPROBE_RUNTIME_URL
// and SOFTPROBE_API_KEY. The tests are skipped when either variable is unset.
//
// Usage:
//
//	SOFTPROBE_RUNTIME_URL=https://runtime.softprobe.dev \
//	SOFTPROBE_API_KEY=... \
//	go test ./e2e/hosted/ -v -count=1
package hosted_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func runtimeURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("SOFTPROBE_RUNTIME_URL")
	if u == "" {
		t.Skip("SOFTPROBE_RUNTIME_URL not set — skipping hosted smoke tests")
	}
	return strings.TrimRight(u, "/")
}

func apiKey(t *testing.T) string {
	t.Helper()
	k := os.Getenv("SOFTPROBE_API_KEY")
	if k == "" {
		t.Skip("SOFTPROBE_API_KEY not set — skipping hosted smoke tests")
	}
	return k
}

type smokeClient struct {
	base   string
	apiKey string
	http   *http.Client
}

func (c *smokeClient) do(method, path string, body []byte) (*http.Response, []byte) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, c.base+path, reqBody)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// TestHostedSmoke_AuthRejected verifies that requests without an API key receive 401.
func TestHostedSmoke_AuthRejected(t *testing.T) {
	base := runtimeURL(t)
	resp, _ := http.Get(base + "/v1/meta")
	if resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthenticated /v1/meta: status = %d, want 401", resp.StatusCode)
		}
	}
}

// TestHostedSmoke_HealthCheck verifies /health is reachable without auth.
func TestHostedSmoke_HealthCheck(t *testing.T) {
	base := runtimeURL(t)
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestHostedSmoke_FullReplayFlow runs the core replay workflow:
// create session → load-case → rules → stats → close
func TestHostedSmoke_FullReplayFlow(t *testing.T) {
	base := runtimeURL(t)
	key := apiKey(t)
	client := &smokeClient{base: base, apiKey: key, http: &http.Client{}}

	// 1. Create replay session
	resp, body := client.do(http.MethodPost, "/v1/sessions", []byte(`{"mode":"replay"}`))
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("create session: status=%v body=%s", resp, body)
	}
	var sessResp struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(body, &sessResp); err != nil || sessResp.SessionID == "" {
		t.Fatalf("create session response: %s", body)
	}
	sessionID := sessResp.SessionID
	t.Logf("session: %s", sessionID)

	// 2. Load a minimal case
	caseBody := fmt.Sprintf(`{"version":"1.0.0","caseId":"%s","traces":[]}`, sessionID)
	resp, body = client.do(http.MethodPost, "/v1/sessions/"+sessionID+"/load-case", []byte(caseBody))
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("load-case: status=%v body=%s", resp, body)
	}

	// 3. Apply a mock rule
	rule := `{"version":1,"rules":[{"name":"r1","priority":100,"when":{"direction":"outbound","method":"GET","path":"/smoke"},"then":{"action":"mock","response":{"statusCode":200,"body":"smoke-ok"}}}]}`
	resp, body = client.do(http.MethodPost, "/v1/sessions/"+sessionID+"/rules", []byte(rule))
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("rules: status=%v body=%s", resp, body)
	}

	// 4. Read stats to verify session is queryable.
	resp, body = client.do(http.MethodGet, "/v1/sessions/"+sessionID+"/stats", nil)
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stats: status=%v body=%s", resp, body)
	}

	// 5. Close
	resp, body = client.do(http.MethodPost, "/v1/sessions/"+sessionID+"/close", nil)
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("close: status=%v body=%s", resp, body)
	}
}

// TestHostedSmoke_CaptureRoundTrip verifies hosted capture goes through
// /v1/traces and becomes queryable via /v1/captures/{captureId}.
func TestHostedSmoke_CaptureRoundTrip(t *testing.T) {
	base := runtimeURL(t)
	key := apiKey(t)
	client := &smokeClient{base: base, apiKey: key, http: &http.Client{}}

	tracePayload := []byte(`{
  "resourceSpans": [{
    "scopeSpans": [{
      "spans": [{
        "traceId": "AAAAAAAAAAAAAAAAAAAAAA==",
        "spanId": "AAAAAAAAAAA=",
        "name": "sp.extract",
        "attributes": [
          {"key": "sp.span.type", "value": {"stringValue": "extract"}}
        ]
      }]
    }]
  }]
}`)

	resp, body := client.do(http.MethodPost, "/v1/traces", tracePayload)
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("post traces: status=%v body=%s", resp, body)
	}
	var traceResp struct {
		CaptureID string `json:"captureId"`
		Accepted  bool   `json:"accepted"`
	}
	if err := json.Unmarshal(body, &traceResp); err != nil {
		t.Fatalf("decode traces response: %v body=%s", err, body)
	}
	if traceResp.CaptureID == "" || !traceResp.Accepted {
		t.Fatalf("unexpected traces response: %s", body)
	}

	deadline := time.Now().Add(45 * time.Second)
	var lastStatus int
	var lastBody []byte
	for time.Now().Before(deadline) {
		resp, body = client.do(http.MethodGet, "/v1/captures/"+traceResp.CaptureID, nil)
		if resp != nil && resp.StatusCode == http.StatusOK {
			var capDoc map[string]any
			if err := json.Unmarshal(body, &capDoc); err != nil {
				t.Fatalf("decode capture document: %v body=%s", err, body)
			}
			if got, _ := capDoc["captureId"].(string); got != traceResp.CaptureID {
				t.Fatalf("captureId mismatch: got=%q want=%q body=%s", got, traceResp.CaptureID, body)
			}
			return
		}
		if resp != nil {
			lastStatus = resp.StatusCode
		}
		lastBody = body
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("capture not materialized: captureId=%s lastStatus=%d lastBody=%s", traceResp.CaptureID, lastStatus, lastBody)
}
