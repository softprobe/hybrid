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
// create session → load-case → inject hit (via rules) → get case → close
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
	rule := `{"version":1,"rules":[{"id":"r1","priority":100,"when":{"direction":"outbound","method":"GET","path":"/smoke"},"then":{"action":"mock","response":{"statusCode":200,"body":"smoke-ok"}}}]}`
	resp, body = client.do(http.MethodPost, "/v1/sessions/"+sessionID+"/rules", []byte(rule))
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("rules: status=%v body=%s", resp, body)
	}

	// 4. GET /v1/cases/{sessionID} — case must be stored
	resp, body = client.do(http.MethodGet, "/v1/cases/"+sessionID, nil)
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get case: status=%v body=%s", resp, body)
	}
	if !strings.Contains(string(body), sessionID) {
		t.Errorf("case body does not contain sessionID: %s", body)
	}

	// 5. Close
	resp, body = client.do(http.MethodPost, "/v1/sessions/"+sessionID+"/close", nil)
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("close: status=%v body=%s", resp, body)
	}
}
