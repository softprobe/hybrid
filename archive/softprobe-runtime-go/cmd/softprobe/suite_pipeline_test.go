package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"softprobe-runtime/internal/controlapi"
)

// TestSuiteRunPipelineInlineMockAndAssert drives a complete suite through
// the CLI pipeline: the suite defines one inline mock and one set of
// assertions, the CLI registers the mock with the real runtime, calls a
// fake SUT, and verifies the response. No sidecar / hooks involved.
func TestSuiteRunPipelineInlineMockAndAssert(t *testing.T) {
	runtime := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(runtime.Close)

	app := fakeSUT(t, func(r *http.Request) (int, map[string]string, string) {
		if r.Header.Get("x-softprobe-session-id") == "" {
			t.Errorf("sut: missing session id header: %v", r.Header)
		}
		return 200, map[string]string{"content-type": "application/json"}, `{"message":"hello","dep":"live"}`
	})
	t.Cleanup(app.Close)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "happy.case.json")
	_ = os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"happy","traces":[{"resourceSpans":[]}]}`), 0o600)

	suite := `
name: demo
cases:
  - path: happy.case.json
request:
  method: GET
  path: /hello
mocks:
  - name: fragment
    match: { direction: outbound, method: GET, path: /fragment }
    response:
      status: 200
      headers: { content-type: application/json }
      body: '{"dep":"mocked"}'
assertions:
  status: 200
  headers:
    include:
      content-type: application/json
  body:
    mode: json-subset
`
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte(suite), 0o600)

	var stdout bytes.Buffer
	code := run([]string{
		"suite", "run",
		"--runtime-url", runtime.URL,
		"--app-url", app.URL,
		"--json",
		suitePath,
	}, &stdout, io.Discard)
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	if got["passed"].(float64) != 1 || got["failed"].(float64) != 0 {
		t.Fatalf("unexpected result: %v", got)
	}
}

// TestSuiteRunPipelineFailsOnStatusMismatch covers the assertion failure
// path: the fake SUT returns 500, the suite expects 200, the case is
// reported as failed with a useful error message.
func TestSuiteRunPipelineFailsOnStatusMismatch(t *testing.T) {
	runtime := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(runtime.Close)

	app := fakeSUT(t, func(_ *http.Request) (int, map[string]string, string) {
		return 500, nil, `{"error":"boom"}`
	})
	t.Cleanup(app.Close)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "fail.case.json")
	_ = os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"fail","traces":[{"resourceSpans":[]}]}`), 0o600)

	suite := `
name: demo
cases:
  - path: fail.case.json
request:
  method: GET
  path: /hello
assertions:
  status: 200
`
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte(suite), 0o600)

	var stdout bytes.Buffer
	code := run([]string{
		"suite", "run",
		"--runtime-url", runtime.URL,
		"--app-url", app.URL,
		"--json",
		suitePath,
	}, &stdout, io.Discard)
	if code != exitSuiteFail {
		t.Fatalf("exit = %d, want %d: %s", code, exitSuiteFail, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	if got["failed"].(float64) != 1 {
		t.Fatalf("expected 1 failure, got %v", got)
	}
	cases := got["cases"].([]any)
	first := cases[0].(map[string]any)
	if !strings.Contains(first["error"].(string), "expected status 200") {
		t.Fatalf("unexpected error message: %v", first["error"])
	}
}

// TestSuiteRunPipelineWithHookSidecar proves the hook wire protocol end
// to end: a user MockResponseHook rewrites the captured response body
// before it's registered as a mock rule. Skipped when `node` isn't on
// PATH — the sidecar is a hard requirement for the hook path.
func TestSuiteRunPipelineWithHookSidecar(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH; skipping sidecar test")
	}

	// Wrap the real runtime with a capturing middleware so the test
	// can inspect `/rules` POSTs even after the suite closes the
	// session (the store purges closed session state).
	realRuntime := controlapi.NewMux()
	var capturedRulesBody []byte
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rules") {
			body, _ := io.ReadAll(r.Body)
			capturedRulesBody = append([]byte(nil), body...)
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		realRuntime.ServeHTTP(w, r)
	}))
	t.Cleanup(runtime.Close)

	// The SUT echoes back whatever mock rule the runtime would have
	// served for /fragment. We assert the mutated body landed on /rules
	// before any /hello call hit the SUT.
	var seenSession string
	app := fakeSUT(t, func(r *http.Request) (int, map[string]string, string) {
		seenSession = r.Header.Get("x-softprobe-session-id")
		return 200, map[string]string{"content-type": "application/json"}, `{"dep":"mutated"}`
	})
	t.Cleanup(app.Close)

	dir := t.TempDir()
	// Case with an outbound /fragment span that findInCase can resolve.
	casePath := filepath.Join(dir, "hooks.case.json")
	_ = os.WriteFile(casePath, []byte(sampleOutboundFragmentCase), 0o600)

	hookPath := filepath.Join(dir, "hooks.mjs")
	_ = os.WriteFile(hookPath, []byte(`
export function rewriteDep({ capturedResponse }) {
  return { ...capturedResponse, body: JSON.stringify({ dep: "mutated" }) };
}
`), 0o600)

	suite := `
name: hooks-demo
cases:
  - path: hooks.case.json
request:
  method: GET
  path: /hello
mocks:
  - name: fragment
    match: { direction: outbound, method: GET, path: /fragment }
    hook: rewriteDep
assertions:
  status: 200
  body:
    mode: json-subset
`
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte(suite), 0o600)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"suite", "run",
		"--runtime-url", runtime.URL,
		"--app-url", app.URL,
		"--hooks", hookPath,
		"--json",
		suitePath,
	}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout.String(), stderr.String())
	}
	if seenSession == "" {
		t.Fatalf("SUT never saw a session id header: stderr=%s", stderr.String())
	}

	// Verify the mock rule that landed on the runtime carries the
	// hook-mutated body, proving the sidecar ran before /rules POST.
	if len(capturedRulesBody) == 0 {
		t.Fatalf("no /rules POST observed")
	}
	var posted struct {
		Rules []struct {
			Then struct {
				Response struct {
					Body string `json:"body"`
				} `json:"response"`
			} `json:"then"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(capturedRulesBody, &posted); err != nil {
		t.Fatalf("decode rules payload: %v: %s", err, capturedRulesBody)
	}
	if len(posted.Rules) == 0 {
		t.Fatalf("no rules posted")
	}
	found := false
	for _, r := range posted.Rules {
		if strings.Contains(r.Then.Response.Body, "mutated") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("hook did not mutate the posted rule body: %s", capturedRulesBody)
	}
}

// TestSuiteRunPipelineHookMissingReportsError checks the "hook name in
// YAML doesn't exist in --hooks file" failure path. The CLI must fail
// the case with a clear message rather than silently skipping the hook.
func TestSuiteRunPipelineHookMissingReportsError(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH; skipping sidecar test")
	}

	runtime := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(runtime.Close)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "c.case.json")
	_ = os.WriteFile(casePath, []byte(sampleOutboundFragmentCase), 0o600)

	hookPath := filepath.Join(dir, "hooks.mjs")
	_ = os.WriteFile(hookPath, []byte(`export function someOtherHook() { return null; }`), 0o600)

	suite := `
name: broken
cases:
  - path: c.case.json
mocks:
  - name: fragment
    match: { direction: outbound, method: GET, path: /fragment }
    hook: rewriteDep
`
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte(suite), 0o600)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"suite", "run",
		"--runtime-url", runtime.URL,
		"--hooks", hookPath,
		"--json",
		suitePath,
	}, &stdout, &stderr)
	if code != exitSuiteFail {
		t.Fatalf("exit = %d, want %d\nstdout=%s", code, exitSuiteFail, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	cases := got["cases"].([]any)
	first := cases[0].(map[string]any)
	if !strings.Contains(first["error"].(string), "rewriteDep") {
		t.Fatalf("expected error mentioning the missing hook name, got: %v", first["error"])
	}
}

// TestSuiteRunEnvExpansion verifies that `${VAR}` / `${VAR:-default}`
// tokens in the suite file are expanded at load time.
func TestSuiteRunEnvExpansion(t *testing.T) {
	runtime := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(runtime.Close)

	app := fakeSUT(t, func(r *http.Request) (int, map[string]string, string) {
		if got := r.Header.Get("x-env-token"); got != "from-env" {
			t.Errorf("x-env-token = %q, want from-env", got)
		}
		if got := r.Header.Get("x-default-token"); got != "fallback" {
			t.Errorf("x-default-token = %q, want fallback", got)
		}
		return 200, nil, `{}`
	})
	t.Cleanup(app.Close)

	t.Setenv("SUITE_ENV_TOKEN", "from-env")
	dir := t.TempDir()
	casePath := filepath.Join(dir, "c.case.json")
	_ = os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"c","traces":[{"resourceSpans":[]}]}`), 0o600)

	suite := `
name: env-demo
cases:
  - path: c.case.json
request:
  method: GET
  path: /hello
  headers:
    x-env-token: "${SUITE_ENV_TOKEN}"
    x-default-token: "${UNSET_VAR:-fallback}"
assertions:
  status: 200
`
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte(suite), 0o600)

	var stdout bytes.Buffer
	code := run([]string{
		"suite", "run",
		"--runtime-url", runtime.URL,
		"--app-url", app.URL,
		"--json",
		suitePath,
	}, &stdout, io.Discard)
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
}

// fakeSUT spins up an httptest server that invokes `h` on every request
// and writes back the handler's response. Headers with nil map skip the
// Write entirely; body is written as-is.
func fakeSUT(t *testing.T, h func(*http.Request) (int, map[string]string, string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, headers, body := h(r)
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

// sampleOutboundFragmentCase is the minimum OTLP shape we need to make
// findInCase resolve `{direction: outbound, method: GET, path: /fragment}`
// — one outbound extract span with status 200 and body "ok".
const sampleOutboundFragmentCase = `{
  "version": "1.0.0",
  "caseId": "fragment-happy-path",
  "traces": [
    {
      "resourceSpans": [
        {
          "resource": { "attributes": [{"key":"service.name","value":{"stringValue":"app"}}] },
          "scopeSpans": [
            {
              "spans": [
                {
                  "spanId": "abc",
                  "attributes": [
                    {"key":"sp.span.type","value":{"stringValue":"extract"}},
                    {"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
                    {"key":"http.request.method","value":{"stringValue":"GET"}},
                    {"key":"url.path","value":{"stringValue":"/fragment"}},
                    {"key":"url.host","value":{"stringValue":"upstream"}},
                    {"key":"http.response.status_code","value":{"intValue":"200"}},
                    {"key":"http.response.body","value":{"stringValue":"{\"dep\":\"ok\"}"}},
                    {"key":"http.response.header.content-type","value":{"stringValue":"application/json"}}
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}`
