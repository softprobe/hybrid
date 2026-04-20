// Live capture integration test: drives the compose ingress proxy in capture
// mode and validates the OTLP-shaped case artifact written by the runtime.
package capture

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"e2e/go/e2etestutil"
	"softprobe-go/softprobe"
)

func TestCaptureFlowProducesValidCaseFile(t *testing.T) {
	runtimeURL := e2etestutil.MustEnv("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := e2etestutil.MustEnv("PROXY_URL", "http://127.0.0.1:8082")
	caseFile := filepath.Join(e2etestutil.ModuleRoot(), "captured.case.json")

	_ = os.Remove(caseFile)

	sp := softprobe.New(softprobe.Options{BaseURL: runtimeURL})
	session, err := sp.StartSession("capture")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	sessionID := session.ID()

	req, err := http.NewRequest(http.MethodGet, proxyURL+"/hello", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("x-softprobe-session-id", sessionID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("proxy status = %d: %s", resp.StatusCode, string(body))
	}

	var responseBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		t.Fatalf("decode proxy body: %v", err)
	}
	if responseBody["message"] != "hello" || responseBody["dep"] != "ok" {
		t.Fatalf("proxy response = %+v, want message=hello and dep=ok", responseBody)
	}

	e2etestutil.WaitForAsyncCaptureUpload()
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	e2etestutil.WaitForFile(t, caseFile)
	e2etestutil.ValidateCaseFile(t, caseFile)
	e2etestutil.ValidateCaptureCaseTraceSemantics(t, caseFile)

	var doc struct {
		Traces []struct {
			ResourceSpans []struct {
				ScopeSpans []struct {
					Spans []struct {
						Attributes []struct {
							Key   string `json:"key"`
							Value struct {
								Int    e2etestutil.OtlpJSONInt64 `json:"intValue,omitempty"`
								String *string                   `json:"stringValue,omitempty"`
							} `json:"value"`
						} `json:"attributes"`
					} `json:"spans"`
				} `json:"scopeSpans"`
			} `json:"resourceSpans"`
		} `json:"traces"`
	}
	data, err := os.ReadFile(caseFile)
	if err != nil {
		t.Fatalf("read case file: %v", err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal case file: %v", err)
	}
	var seenHello, seenFragment bool
	var helloFullURL string
	var helloStatus int64
	for _, tr := range doc.Traces {
		for _, rs := range tr.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					var path, fullURL string
					var statusCode int64
					for _, attr := range sp.Attributes {
						switch attr.Key {
						case "url.path":
							if attr.Value.String != nil {
								path = *attr.Value.String
							}
						case "url.full":
							if attr.Value.String != nil {
								fullURL = *attr.Value.String
							}
						case "http.response.status_code":
							if attr.Value.Int.V != nil {
								statusCode = *attr.Value.Int.V
							}
						}
					}
					switch path {
					case "/hello":
						seenHello = true
						helloFullURL = fullURL
						helloStatus = statusCode
					case "/fragment":
						seenFragment = true
						if statusCode != 200 {
							t.Fatalf("fragment span status_code = %d, want 200", statusCode)
						}
					}
				}
			}
		}
	}
	if !seenHello {
		t.Fatal("case file: no extract span with url.path /hello (ingress app I/O)")
	}
	if !seenFragment {
		t.Fatal("case file: no extract span with url.path /fragment (egress dependency I/O)")
	}
	if !strings.Contains(helloFullURL, "/hello") {
		t.Fatalf("hello span url.full = %q, want /hello", helloFullURL)
	}
	if helloStatus != 200 {
		t.Fatalf("hello span status_code = %d, want 200", helloStatus)
	}
}
