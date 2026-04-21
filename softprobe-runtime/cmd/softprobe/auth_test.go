package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/store"
)

// authCaptureServer wraps an inner mux while recording every Authorization header
// that arrives from the CLI under test, so individual commands can be exercised
// against a single shared fake runtime.
type authCaptureServer struct {
	*httptest.Server
	mu     sync.Mutex
	tokens []string
	paths  []string
}

func newAuthCaptureServer(t *testing.T, inner http.Handler) *authCaptureServer {
	t.Helper()
	capture := &authCaptureServer{}
	capture.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.mu.Lock()
		capture.tokens = append(capture.tokens, r.Header.Get("Authorization"))
		capture.paths = append(capture.paths, r.Method+" "+r.URL.Path)
		capture.mu.Unlock()
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(capture.Close)
	return capture
}

func (c *authCaptureServer) snapshot() ([]string, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.tokens...), append([]string(nil), c.paths...)
}

// TestCLIAttachesBearerTokenFromEnv exercises every CLI subcommand that hits
// the runtime and asserts Authorization: Bearer <SOFTPROBE_API_TOKEN> is set
// on every outbound request. This is the runtime-side contract from
// internal/controlapi/errors.go#withOptionalBearerAuth.
func TestCLIAttachesBearerTokenFromEnv(t *testing.T) {
	const token = "sp_test_token_abc123"
	t.Setenv("SOFTPROBE_API_TOKEN", token)

	st := store.NewStore()
	capture := newAuthCaptureServer(t, controlapi.NewMux(st))

	casePath := filepath.Join(t.TempDir(), "demo.case.json")
	if err := os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"demo","traces":[]}`), 0o600); err != nil {
		t.Fatalf("write case file: %v", err)
	}
	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(rulesPath, []byte(`{"rules":[{"when":{"direction":"outbound"},"then":{"action":"passthrough"}}]}`), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	runOK := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		var stderr bytes.Buffer
		if code := run(args, &out, &stderr); code != 0 {
			t.Fatalf("args %v exit code = %d stderr = %q", args, code, stderr.String())
		}
		return out.String()
	}

	runOK("doctor", "--runtime-url", capture.URL)

	startOut := runOK("session", "start", "--runtime-url", capture.URL, "--json")
	var started struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(startOut), &started); err != nil {
		t.Fatalf("unmarshal session start: %v", err)
	}

	runOK("session", "load-case", "--runtime-url", capture.URL, "--session", started.SessionID, "--file", casePath)
	runOK("session", "rules", "apply", "--runtime-url", capture.URL, "--session", started.SessionID, "--file", rulesPath)
	runOK("session", "policy", "set", "--runtime-url", capture.URL, "--session", started.SessionID, "--strict")
	runOK("session", "stats", "--runtime-url", capture.URL, "--session", started.SessionID, "--json")
	runOK("session", "close", "--runtime-url", capture.URL, "--session", started.SessionID, "--json")

	tokens, paths := capture.snapshot()
	if len(tokens) == 0 {
		t.Fatal("no requests captured")
	}
	wantHeader := "Bearer " + token
	for i, got := range tokens {
		if got != wantHeader {
			t.Errorf("request %d (%s): Authorization = %q, want %q", i, paths[i], got, wantHeader)
		}
	}

	// Sanity: make sure we actually hit the full auth-protected surface.
	expectedPaths := []string{
		"GET /health",       // doctor skips auth for /health but still sends it
		"GET /v1/meta",      // doctor drift check
		"POST /v1/sessions", // session start
	}
	joined := strings.Join(paths, "\n")
	for _, want := range expectedPaths {
		if !strings.Contains(joined, want) {
			t.Errorf("paths %v do not include %q", paths, want)
		}
	}
}

// TestCLIOmitsBearerTokenWhenEnvUnset confirms the CLI stays quiet when no
// token is configured, matching the existing "auth disabled by default" story.
func TestCLIOmitsBearerTokenWhenEnvUnset(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "")

	capture := newAuthCaptureServer(t, controlapi.NewMux())

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", capture.URL}, &out, nil); code != 0 {
		t.Fatalf("doctor exit code = %d, want 0", code)
	}
	if code := run([]string{"session", "start", "--runtime-url", capture.URL, "--json"}, &bytes.Buffer{}, nil); code != 0 {
		t.Fatalf("session start exit code = %d, want 0", code)
	}

	tokens, paths := capture.snapshot()
	if len(tokens) == 0 {
		t.Fatal("no requests captured")
	}
	for i, got := range tokens {
		if got != "" {
			t.Errorf("request %d (%s): Authorization = %q, want empty", i, paths[i], got)
		}
	}
}

// TestCLIReturnsRuntimeErrorOn401WithoutToken documents the user-facing
// behavior when auth is enabled on the runtime but the CLI has no token:
// the runtime responds 401 and the CLI surfaces a non-zero exit code.
func TestCLIReturnsRuntimeErrorOn401WithoutToken(t *testing.T) {
	t.Setenv("SOFTPROBE_API_TOKEN", "secret-on-server")
	mux := controlapi.NewMux()
	t.Setenv("SOFTPROBE_API_TOKEN", "") // server keeps enforcing; CLI can't see it
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	var stderr bytes.Buffer
	code := run([]string{"session", "start", "--runtime-url", server.URL, "--json"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatal("session start without token should fail, got exit 0")
	}
	// We don't mandate a specific error string here; just that it surfaces
	// the runtime's non-2xx status in a way the user can act on.
	if !strings.Contains(stderr.String(), "401") && !strings.Contains(stderr.String(), "unauthorized") {
		t.Logf("note: stderr = %q (no explicit 401/unauthorized string, which is still acceptable)", stderr.String())
	}
}
