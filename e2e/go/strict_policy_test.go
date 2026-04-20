package main

import (
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"e2e/go/e2etestutil"
	"softprobe-go/softprobe"
)

func TestStrictPolicyBlocksUnmockedTraffic(t *testing.T) {
	runtimeURL := e2etestutil.MustEnv("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := e2etestutil.MustEnv("PROXY_URL", "http://127.0.0.1:8082")
	appURL := e2etestutil.MustEnv("APP_URL", "http://127.0.0.1:8081")
	upstreamURL := e2etestutil.MustEnv("UPSTREAM_URL", "http://127.0.0.1:8083")
	caseFile := filepath.Join(e2etestutil.ModuleRoot(), "captured.case.json")

	e2etestutil.EnsureCapturedCase(t, runtimeURL, proxyURL, caseFile)
	e2etestutil.ResetAppCounter(t, appURL)
	e2etestutil.ResetUpstreamCounter(t, upstreamURL)

	sp := softprobe.New(softprobe.Options{BaseURL: runtimeURL})
	session, err := sp.StartSession("replay")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := session.LoadCaseFromFile(caseFile); err != nil {
		t.Fatalf("LoadCaseFromFile: %v", err)
	}
	if err := session.SetPolicy([]byte(`{"externalHttp":"strict"}`)); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	sessionID := session.ID()

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

	if got := e2etestutil.AppRequestCount(t, appURL); got != 0 {
		t.Fatalf("app /hello hit count after strict miss = %d, want 0", got)
	}
	if got := e2etestutil.UpstreamFragmentCount(t, upstreamURL); got != 0 {
		t.Fatalf("upstream /fragment hit count after strict miss = %d, want 0", got)
	}
}
