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

	"softprobe-go/softprobe"
)

// Note: the former TestReplayFlowUsesCapturedCase covered runtime-side auto-
// ingress replay from case-embedded rules. That pathway was removed in
// P4.5e ("Runtime: delete replay action"), which made the SDK responsible
// for picking captured responses (FindInCase) and registering mock rules
// (MockOutbound). Ingress replay is therefore no longer a runtime feature,
// and the equivalent SDK-authoring flow is exercised by
// TestReplayEgressInjectMocksUpstream below and by the four SDK harnesses
// under e2e/{go,jest,pytest,junit}-replay/.

// TestReplayEgressInjectMocksUpstream issues GET /fragment on the egress listener with a replay
// session so the runtime must match the captured egress extract span and return the stored
// dependency response without calling the live upstream.
//
// This test exercises the same SDK authoring flow as e2e/go-replay/,
// e2e/jest-replay/, e2e/pytest-replay/, and e2e/junit-replay/: it drives
// softprobe-go's Softprobe / SoftprobeSession facade end-to-end
// (StartSession → LoadCaseFromFile → FindInCase → MockOutbound) against
// the live runtime, then hits the egress proxy listener to prove the
// mock rule is honored. The expected response body comes from the
// captured case via FindInCase, so the assertion remains data-driven.
func TestReplayEgressInjectMocksUpstream(t *testing.T) {
	runtimeURL := mustEnv("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := mustEnv("PROXY_URL", "http://127.0.0.1:8082")
	upstreamURL := mustEnv("UPSTREAM_URL", "http://127.0.0.1:8083")
	caseFile := "captured.case.json"

	ensureCapturedCase(t, runtimeURL, proxyURL, caseFile)
	validateCaptureCaseTraceSemantics(t, caseFile)

	egressBase := egressHTTPBaseForTest(caseFile)
	fromCase, hostOK := egressURLFromCapturedCase(caseFile)

	resetUpstreamCounter(t, upstreamURL)

	sp := softprobe.New(softprobe.Options{BaseURL: runtimeURL})
	session, err := sp.StartSession("replay")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Logf("close session: %v", err)
		}
	})

	if err := session.LoadCaseFromFile(caseFile); err != nil {
		t.Fatalf("LoadCaseFromFile: %v", err)
	}

	hit, err := session.FindInCase(softprobe.CaseSpanPredicate{
		Direction: "outbound",
		Method:    http.MethodGet,
		Path:      "/fragment",
	})
	if err != nil {
		t.Fatalf("FindInCase: %v", err)
	}

	priority := 100
	if err := session.MockOutbound(softprobe.MockRuleSpec{
		ID:        "fragment-egress-replay",
		Priority:  &priority,
		Direction: "outbound",
		Method:    http.MethodGet,
		Path:      "/fragment",
		Response:  hit.Response,
	}); err != nil {
		t.Fatalf("MockOutbound: %v", err)
	}

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
	req.Header.Set("x-softprobe-session-id", session.ID())
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
	if strings.TrimSpace(string(body)) != strings.TrimSpace(hit.Response.Body) {
		t.Fatalf("egress body = %q, want captured fragment body %q", strings.TrimSpace(string(body)), strings.TrimSpace(hit.Response.Body))
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
