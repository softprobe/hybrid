package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestStrictPolicyBlocksUnmockedTraffic(t *testing.T) {
	runtimeURL := mustEnv("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := mustEnv("PROXY_URL", "http://127.0.0.1:8082")
	appURL := mustEnv("APP_URL", "http://127.0.0.1:8081")
	upstreamURL := mustEnv("UPSTREAM_URL", "http://127.0.0.1:8083")
	caseFile := "captured.case.json"

	ensureCapturedCase(t, runtimeURL, proxyURL, caseFile)
	resetAppCounter(t, appURL)
	resetUpstreamCounter(t, upstreamURL)

	sessionID := startSession(t, runtimeURL, "replay")
	loadCase(t, runtimeURL, sessionID, caseFile)
	setPolicy(t, runtimeURL, sessionID, []byte(`{"externalHttp":"strict"}`))

	req, err := http.NewRequest(http.MethodGet, proxyURL+"/unknown", nil)
	if err != nil {
		t.Fatalf("new proxy request: %v", err)
	}
	req.Header.Set("x-softprobe-session-id", sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("proxy status = %d, want 5xx: %s", resp.StatusCode, string(body))
	}

	if got := appRequestCount(t, appURL); got != 0 {
		t.Fatalf("app /hello hit count after strict miss = %d, want 0", got)
	}
	if got := upstreamFragmentCount(t, upstreamURL); got != 0 {
		t.Fatalf("upstream /fragment hit count after strict miss = %d, want 0", got)
	}
}

func setPolicy(t *testing.T, runtimeURL, sessionID string, policy []byte) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, runtimeURL+"/v1/sessions/"+sessionID+"/policy", bytes.NewReader(policy))
	if err != nil {
		t.Fatalf("new policy request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("set policy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("policy status = %d: %s", resp.StatusCode, string(body))
	}
}
