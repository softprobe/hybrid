package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestReplayFlowUsesCapturedCase checks that a replay session can serve GET /hello through the
// ingress proxy without calling the live app or upstream. The runtime matches the captured
// ingress extract span (client-facing /hello) and returns the full stored http.response.body
// (the combined app response, including dep field). The live app and upstream are not contacted
// on that path. See TestReplayEgressInjectMocksUpstream for proof that the egress /fragment
// extract span is replayable as an inject hit (upstream mock).
func TestReplayFlowUsesCapturedCase(t *testing.T) {
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

	resp := doProxyHello(t, proxyURL, sessionID)
	assertHelloBody(t, resp)

	if got := appRequestCount(t, appURL); got != 0 {
		t.Fatalf("app /hello hit count after replay = %d, want 0 (replay must not reach live workload)", got)
	}
	if got := upstreamFragmentCount(t, upstreamURL); got != 0 {
		t.Fatalf("upstream /fragment hit count after replay = %d, want 0 (dependency must not be contacted live)", got)
	}

	resp = doProxyHello(t, proxyURL, sessionID)
	assertHelloBody(t, resp)

	if got := appRequestCount(t, appURL); got != 0 {
		t.Fatalf("app /hello hit count after second replay = %d, want 0", got)
	}
	if got := upstreamFragmentCount(t, upstreamURL); got != 0 {
		t.Fatalf("upstream /fragment hit count after second replay = %d, want 0", got)
	}
}

// TestReplayEgressInjectMocksUpstream issues GET /fragment on the egress listener with a replay
// session so the runtime must match the captured egress extract span and return the stored
// dependency response without calling the live upstream.
func TestReplayEgressInjectMocksUpstream(t *testing.T) {
	runtimeURL := mustEnv("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := mustEnv("PROXY_URL", "http://127.0.0.1:8082")
	upstreamURL := mustEnv("UPSTREAM_URL", "http://127.0.0.1:8083")
	caseFile := "captured.case.json"

	ensureCapturedCase(t, runtimeURL, proxyURL, caseFile)
	validateCaptureCaseTraceSemantics(t, caseFile)

	egressBase := egressHTTPBaseForTest(caseFile)
	fromCase, hostOK := egressURLFromCapturedCase(caseFile)
	wantBody, ok := fragmentResponseBodyFromCase(caseFile)
	if !ok {
		t.Fatal("case file: no http.response.body on /fragment extract span")
	}

	resetUpstreamCounter(t, upstreamURL)

	sessionID := startSession(t, runtimeURL, "replay")
	loadCase(t, runtimeURL, sessionID, caseFile)

	req, err := http.NewRequest(http.MethodGet, egressBase+"/fragment", nil)
	if err != nil {
		t.Fatalf("new egress request: %v", err)
	}
	if hostOK {
		if u, err := url.Parse(fromCase); err == nil && u.Host != "" {
			// Case records docker internal host:port; dial 127.0.0.1 but send the captured Host
			// so inject lookup matches url.host on the span.
			req.Host = u.Host
		}
	}
	req.Header.Set("x-softprobe-session-id", sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("egress proxy request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("egress status = %d: %s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read egress body: %v", err)
	}
	if strings.TrimSpace(string(body)) != strings.TrimSpace(wantBody) {
		t.Fatalf("egress body = %q, want captured fragment body %q", strings.TrimSpace(string(body)), strings.TrimSpace(wantBody))
	}

	if got := upstreamFragmentCount(t, upstreamURL); got != 0 {
		t.Fatalf("upstream /fragment hits after egress replay inject = %d, want 0 (must not call live dependency)", got)
	}
}

func ensureCapturedCase(t *testing.T, runtimeURL, proxyURL, caseFile string) {
	t.Helper()

	if caseFileHasIngressEgressCapture(caseFile) && captureCaseTraceSemanticsError(caseFile) == nil {
		return
	}
	_ = os.Remove(caseFile)

	sessionID := startSession(t, runtimeURL, "capture")
	resp := doProxyHello(t, proxyURL, sessionID)
	assertHelloBody(t, resp)
	waitForAsyncCaptureUpload()
	closeSession(t, runtimeURL, sessionID)
	waitForFile(t, caseFile)
	validateCaseFile(t, caseFile)
	validateCaptureCaseTraceSemantics(t, caseFile)
	if !caseFileHasIngressEgressCapture(caseFile) {
		t.Fatalf("captured case file %s must contain ingress /hello and egress /fragment extract data", caseFile)
	}
}

func loadCase(t *testing.T, runtimeURL, sessionID, caseFile string) {
	t.Helper()

	data, err := os.ReadFile(caseFile)
	if err != nil {
		t.Fatalf("read case file: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, runtimeURL+"/v1/sessions/"+sessionID+"/load-case", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new load-case request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("load-case request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("load-case status = %d: %s", resp.StatusCode, string(body))
	}
}

func doProxyHello(t *testing.T, proxyURL, sessionID string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, proxyURL+"/hello", nil)
	if err != nil {
		t.Fatalf("new proxy request: %v", err)
	}
	req.Header.Set("x-softprobe-session-id", sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	return resp
}

func assertHelloBody(t *testing.T, resp *http.Response) {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("proxy status = %d: %s", resp.StatusCode, string(body))
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode proxy body: %v", err)
	}
	if body["message"] != "hello" || body["dep"] != "ok" {
		t.Fatalf("proxy body = %+v, want message=hello and dep=ok", body)
	}
}

func resetAppCounter(t *testing.T, appURL string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, appURL+"/reset", nil)
	if err != nil {
		t.Fatalf("new reset request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reset app counter: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("reset status = %d: %s", resp.StatusCode, string(body))
	}
}

func resetUpstreamCounter(t *testing.T, upstreamURL string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, upstreamURL+"/reset", nil)
	if err != nil {
		t.Fatalf("new upstream reset request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reset upstream counter: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upstream reset status = %d: %s", resp.StatusCode, string(body))
	}
}

func upstreamFragmentCount(t *testing.T, upstreamURL string) int64 {
	t.Helper()

	resp, err := http.Get(upstreamURL + "/count")
	if err != nil {
		t.Fatalf("get upstream /count: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upstream count status = %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Count int64 `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode upstream count: %v", err)
	}
	return body.Count
}

func appRequestCount(t *testing.T, appURL string) int64 {
	t.Helper()

	resp, err := http.Get(appURL + "/count")
	if err != nil {
		t.Fatalf("get app /count: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("count status = %d: %s", resp.StatusCode, string(body))
	}

	var body struct {
		Count int64 `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode count response: %v", err)
	}
	return body.Count
}

func waitForAsyncCaptureUpload() {
	// Allow both ingress and egress extract uploads to finish.
	time.Sleep(4 * time.Second)
}

// caseFileHasIngressEgressCapture reports whether the case file is suitable for replay tests:
// it must include captured traffic for both the client→app leg (/hello) and the app→dependency
// leg (/fragment). Stale or single-hop captures are rejected so replay always regenerates.
func caseFileHasIngressEgressCapture(caseFile string) bool {
	data, err := os.ReadFile(caseFile)
	if err != nil {
		return false
	}

	var doc struct {
		Traces []struct {
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
		} `json:"traces"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}

	var seenHello, seenFragment bool
	var helloBody string
	for _, tr := range doc.Traces {
		for _, rs := range tr.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					var path, body string
					for _, attr := range sp.Attributes {
						switch attr.Key {
						case "url.path":
							if attr.Value.String != nil {
								path = *attr.Value.String
							}
						case "http.response.body":
							if attr.Value.String != nil {
								body = *attr.Value.String
							}
						}
					}
					switch path {
					case "/hello":
						seenHello = true
						helloBody = body
					case "/fragment":
						seenFragment = true
					}
				}
			}
		}
	}
	if !seenHello || !seenFragment {
		return false
	}
	// Ingress response must be the full handler output (app + dependency), not an older single-field body.
	return strings.Contains(helloBody, `"message":"hello"`) && strings.Contains(helloBody, `"dep":"ok"`)
}
