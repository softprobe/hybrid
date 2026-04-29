package e2etestutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/softprobe/softprobe-go/softprobe"
)

// EnsureCapturedCase guarantees a fresh captured.case.json suitable for downstream replay tests.
func EnsureCapturedCase(t *testing.T, runtimeURL, proxyURL, caseFile string) {
	t.Helper()

	if CaseFileHasIngressEgressCapture(caseFile) && CaptureCaseTraceSemanticsError(caseFile) == nil {
		return
	}
	_ = os.Remove(caseFile)

	sp := softprobe.New(softprobe.Options{BaseURL: runtimeURL, APIToken: APIKey()})
	session, err := sp.StartSession("capture")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	sessionID := session.ID()
	resp := DoProxyHello(t, proxyURL, sessionID)
	AssertHelloBody(t, resp)
	WaitForAsyncCaptureUpload()
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	MaterializeCapturedCase(t, runtimeURL, sessionID, caseFile)
	ValidateCaseFile(t, caseFile)
	ValidateCaptureCaseTraceSemantics(t, caseFile)
	if !CaseFileHasIngressEgressCapture(caseFile) {
		t.Fatalf("captured case file %s must contain ingress /hello and egress /fragment extract data", caseFile)
	}
}

// DoProxyHello issues GET /hello on the ingress proxy with the session header.
func DoProxyHello(t *testing.T, proxyURL, sessionID string) *http.Response {
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

// AssertHelloBody decodes the proxy /hello JSON and checks message + dep.
func AssertHelloBody(t *testing.T, resp *http.Response) {
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

// ResetAppCounter resets the SUT request counter.
func ResetAppCounter(t *testing.T, appURL string) {
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

// ResetUpstreamCounter resets the upstream /fragment counter.
func ResetUpstreamCounter(t *testing.T, upstreamURL string) {
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

// UpstreamFragmentCount returns the upstream /fragment hit count.
func UpstreamFragmentCount(t *testing.T, upstreamURL string) int64 {
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

// AppRequestCount returns the app /hello hit count.
func AppRequestCount(t *testing.T, appURL string) int64 {
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

// WaitForAsyncCaptureUpload sleeps briefly so ingress and egress extract uploads finish.
func WaitForAsyncCaptureUpload() {
	time.Sleep(4 * time.Second)
}

// MaterializeCapturedCase ensures caseFile exists after capture close.
// Hosted runtimes expose it via GET /v1/captures/{captureId}; legacy local
// runtimes may still expose GET /v1/cases/{sessionId}.
func MaterializeCapturedCase(t *testing.T, runtimeURL, sessionID, caseFile string) {
	t.Helper()

	if fileExists(caseFile) {
		return
	}

	apiKey := APIKey()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(120 * time.Second)
	captureURL := strings.TrimRight(runtimeURL, "/") + "/v1/captures/" + sessionID
	legacyCaseURL := strings.TrimRight(runtimeURL, "/") + "/v1/cases/" + sessionID
	for time.Now().Before(deadline) {
		for _, u := range []string{captureURL, legacyCaseURL} {
			req, err := http.NewRequest(http.MethodGet, u, nil)
			if err == nil {
				if apiKey != "" {
					req.Header.Set("Authorization", "Bearer "+apiKey)
				}
				resp, err := client.Do(req)
				if err == nil {
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						if err := os.WriteFile(caseFile, body, 0o644); err != nil {
							t.Fatalf("write case file %s: %v", caseFile, err)
						}
						return
					}
					// Capture materialization may take a moment; continue polling while
					// datalake flush/query views converge.
					if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusInternalServerError {
						t.Fatalf("GET %s: status=%d body=%s", u, resp.StatusCode, bytes.TrimSpace(body))
					}
				}
			}
		}
		if fileExists(caseFile) {
			return
		}
		time.Sleep(1 * time.Second)
	}

	WaitForFile(t, caseFile)
	if !fileExists(caseFile) {
		t.Fatalf("captured case not materialized at %s (runtime=%s session=%s)", caseFile, runtimeURL, sessionID)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CaseFileHasIngressEgressCapture reports whether the case file is suitable for replay tests.
func CaseFileHasIngressEgressCapture(caseFile string) bool {
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
	var helloMethod, fragmentMethod string
	for _, tr := range doc.Traces {
		for _, rs := range tr.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					var path, body, method string
					for _, attr := range sp.Attributes {
						switch attr.Key {
						case "url.path":
							if attr.Value.String != nil {
								path = *attr.Value.String
							}
						case "http.request.method":
							if attr.Value.String != nil {
								method = *attr.Value.String
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
						helloMethod = method
					case "/fragment":
						seenFragment = true
						fragmentMethod = method
					}
				}
			}
		}
	}
	if !seenHello || !seenFragment {
		return false
	}
	if helloMethod != "GET" || fragmentMethod != "GET" {
		return false
	}
	return strings.Contains(helloBody, `"message":"hello"`) && strings.Contains(helloBody, `"dep":"ok"`)
}
