package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"softprobe-runtime/internal/runtimeapp"
)

func TestRunVersionPrintsBinaryVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"--version"}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "softprobe ") {
		t.Fatalf("output = %q, want version string", got)
	}
}

func TestRunDoctorChecksRuntimeHealth(t *testing.T) {
	server := httptest.NewServer(runtimeapp.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", server.URL}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "runtime healthy:") || !strings.Contains(got, "specVersion:") || !strings.Contains(got, "schemaVersion:") {
		t.Fatalf("output = %q, want health and explicit version fields", got)
	}
}

func TestRunDoctorReturnsNonZeroForUnhealthyRuntime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unhealthy", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	var stderr bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", server.URL}, nil, &stderr); code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
	if got := stderr.String(); !strings.Contains(got, "status 500") {
		t.Fatalf("stderr = %q, want status 500", got)
	}
}

func TestRunSessionStartEmitsJsonAndExportLine(t *testing.T) {
	server := httptest.NewServer(runtimeapp.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	var resp struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
		SchemaVersion   string `json:"schemaVersion"`
		SpecVersion     string `json:"specVersion"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal json output: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatal("sessionId is empty")
	}
	if resp.SessionRevision != 0 {
		t.Fatalf("sessionRevision = %d, want 0", resp.SessionRevision)
	}
	if resp.SchemaVersion == "" || resp.SpecVersion == "" {
		t.Fatalf("schema/spec versions are empty: %+v", resp)
	}
}

func TestRunSessionStartShellFriendlyOutput(t *testing.T) {
	server := httptest.NewServer(runtimeapp.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "export SOFTPROBE_SESSION_ID=") {
		t.Fatalf("output = %q, want export line", got)
	}
}

func TestRunSessionStartSupportsModeFlag(t *testing.T) {
	store := runtimeapp.NewStore()
	server := httptest.NewServer(runtimeapp.NewMux(store))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--mode", "capture", "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	var resp struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal json output: %v", err)
	}

	session, ok := store.Get(resp.SessionID)
	if !ok {
		t.Fatalf("session %q not found", resp.SessionID)
	}
	if session.Mode != "capture" {
		t.Fatalf("session mode = %q, want capture", session.Mode)
	}
}

func TestRunSessionLoadCasePersistsCaseAndReturnsZero(t *testing.T) {
	store := runtimeapp.NewStore()
	server := httptest.NewServer(runtimeapp.NewMux(store))
	t.Cleanup(server.Close)

	var startOut bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--json"}, &startOut, nil); code != 0 {
		t.Fatalf("session start exit code = %d, want 0", code)
	}

	var started struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal session start output: %v", err)
	}

	casePath := filepath.Join(t.TempDir(), "demo.case.json")
	if err := os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"demo","traces":[]}`), 0o600); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	var loadOut bytes.Buffer
	args := []string{"session", "load-case", "--runtime-url", server.URL, "--session", started.SessionID, "--file", casePath}
	if code := run(args, &loadOut, nil); code != 0 {
		t.Fatalf("session load-case exit code = %d, want 0", code)
	}

	session, ok := store.Get(started.SessionID)
	if !ok {
		t.Fatalf("session %q not found", started.SessionID)
	}
	if session.Revision != 1 {
		t.Fatalf("session revision = %d, want 1", session.Revision)
	}
	if !bytes.Contains(session.LoadedCase, []byte(`"caseId":"demo"`)) {
		t.Fatalf("loaded case missing demo case: %s", string(session.LoadedCase))
	}
}

func TestRunSessionLoadCaseLoadsGoldenCaseFixture(t *testing.T) {
	store := runtimeapp.NewStore()
	server := httptest.NewServer(runtimeapp.NewMux(store))
	t.Cleanup(server.Close)

	var startOut bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--json"}, &startOut, nil); code != 0 {
		t.Fatalf("session start exit code = %d, want 0", code)
	}

	var started struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal session start output: %v", err)
	}

	casePath := filepath.Join("..", "..", "..", "spec", "examples", "cases", "checkout-happy-path.case.json")
	var loadOut bytes.Buffer
	if code := run([]string{"session", "load-case", "--runtime-url", server.URL, "--session", started.SessionID, "--file", casePath}, &loadOut, nil); code != 0 {
		t.Fatalf("session load-case exit code = %d, want 0", code)
	}

	session, ok := store.Get(started.SessionID)
	if !ok {
		t.Fatalf("session %q not found", started.SessionID)
	}
	if !bytes.Contains(session.LoadedCase, []byte("checkout-happy-path")) {
		t.Fatalf("loaded case missing golden case id: %s", string(session.LoadedCase))
	}
}

func TestRunSessionLoadCaseReturnsErrorForUnknownSession(t *testing.T) {
	server := httptest.NewServer(runtimeapp.NewMux())
	t.Cleanup(server.Close)

	casePath := filepath.Join(t.TempDir(), "demo.case.json")
	if err := os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"demo","traces":[]}`), 0o600); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	var stderr bytes.Buffer
	code := run(
		[]string{"session", "load-case", "--runtime-url", server.URL, "--session", "missing", "--file", casePath},
		nil,
		&stderr,
	)
	if code == 0 {
		t.Fatal("session load-case should fail for unknown session")
	}
	if got := stderr.String(); !strings.Contains(got, "unknown session") {
		t.Fatalf("stderr = %q, want unknown session", got)
	}
}

func TestRunInspectCaseSummarizesGoldenFixture(t *testing.T) {
	var out bytes.Buffer
	casePath := filepath.Join("..", "..", "..", "spec", "examples", "cases", "checkout-happy-path.case.json")
	if code := run([]string{"inspect", "case", casePath}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	got := out.String()
	for _, want := range []string{
		"case checkout-happy-path",
		"traces: 1",
		"api.stripe.com",
		"outbound",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestRunSessionRulesApplyPersistsRules(t *testing.T) {
	store := runtimeapp.NewStore()
	server := httptest.NewServer(runtimeapp.NewMux(store))
	t.Cleanup(server.Close)

	var startOut bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--json"}, &startOut, nil); code != 0 {
		t.Fatalf("session start exit code = %d, want 0", code)
	}

	var started struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal session start output: %v", err)
	}

	rulesPath := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(rulesPath, []byte(`{"rules":[{"when":{"direction":"outbound"},"then":{"action":"passthrough"}}]}`), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	var out bytes.Buffer
	args := []string{"session", "rules", "apply", "--runtime-url", server.URL, "--session", started.SessionID, "--file", rulesPath}
	if code := run(args, &out, nil); code != 0 {
		t.Fatalf("session rules apply exit code = %d, want 0", code)
	}

	session, ok := store.Get(started.SessionID)
	if !ok {
		t.Fatalf("session %q not found", started.SessionID)
	}
	if session.Revision != 1 {
		t.Fatalf("session revision = %d, want 1", session.Revision)
	}
	if !bytes.Contains(session.Rules, []byte(`"action":"passthrough"`)) {
		t.Fatalf("stored rules missing passthrough rule: %s", string(session.Rules))
	}
}

func TestRunSessionPolicySetPersistsStrictPolicy(t *testing.T) {
	store := runtimeapp.NewStore()
	server := httptest.NewServer(runtimeapp.NewMux(store))
	t.Cleanup(server.Close)

	var startOut bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--json"}, &startOut, nil); code != 0 {
		t.Fatalf("session start exit code = %d, want 0", code)
	}

	var started struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("unmarshal session start output: %v", err)
	}

	var out bytes.Buffer
	args := []string{"session", "policy", "set", "--runtime-url", server.URL, "--session", started.SessionID, "--strict"}
	if code := run(args, &out, nil); code != 0 {
		t.Fatalf("session policy set exit code = %d, want 0", code)
	}

	session, ok := store.Get(started.SessionID)
	if !ok {
		t.Fatalf("session %q not found", started.SessionID)
	}
	if session.Revision != 1 {
		t.Fatalf("session revision = %d, want 1", session.Revision)
	}
	if !bytes.Contains(session.Policy, []byte(`"externalHttp":"strict"`)) {
		t.Fatalf("stored policy missing strict setting: %s", string(session.Policy))
	}
}
