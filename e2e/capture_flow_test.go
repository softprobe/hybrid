package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureFlowProducesValidCaseFile(t *testing.T) {
	runtimeURL := mustEnv("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := mustEnv("PROXY_URL", "http://127.0.0.1:8082")
	caseFile := "captured.case.json"

	_ = os.Remove(caseFile)

	sessionID := startSession(t, runtimeURL, "capture")

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

	waitForAsyncCaptureUpload()
	closeSession(t, runtimeURL, sessionID)
	waitForFile(t, caseFile)
	validateCaseFile(t, caseFile)
	validateCaptureCaseTraceSemantics(t, caseFile)

	var doc struct {
		Traces []struct {
			ResourceSpans []struct {
				ScopeSpans []struct {
					Spans []struct {
						Attributes []struct {
							Key   string `json:"key"`
							Value struct {
								Int    otlpJSONInt64 `json:"intValue,omitempty"`
								String *string       `json:"stringValue,omitempty"`
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

func startSession(t *testing.T, runtimeURL, mode string) string {
	t.Helper()

	body := fmt.Sprintf(`{"mode":%q}`, mode)
	resp, err := http.Post(runtimeURL+"/v1/sessions", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("start session status = %d: %s", resp.StatusCode, string(data))
	}

	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode start session response: %v", err)
	}
	return created.SessionID
}

func closeSession(t *testing.T, runtimeURL, sessionID string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, runtimeURL+"/v1/sessions/"+sessionID+"/close", nil)
	if err != nil {
		t.Fatalf("close request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("close session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("close session status = %d: %s", resp.StatusCode, string(data))
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("file %s not created", path)
}

func validateCaseFile(t *testing.T, path string) {
	t.Helper()

	cmd := exec.Command(
		"npx",
		"-y",
		"ajv-cli@5",
		"validate",
		"-s",
		filepath.Join("..", "spec", "schemas", "case.schema.json"),
		"-r",
		filepath.Join("..", "spec", "schemas", "case-trace.schema.json"),
		"-d",
		path,
		"--spec=draft2020",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("case validation failed: %v\n%s", err, output)
	}
}

func mustEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
