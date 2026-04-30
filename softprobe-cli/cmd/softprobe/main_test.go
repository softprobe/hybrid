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

	"softprobe-cli/internal/apiversion"
	"softprobe-cli/internal/controlapi"
	"softprobe-cli/internal/store"
	"softprobe-cli/internal/version"
)

func TestRunVersionPrintsBinaryVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"--version"}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	want := "softprobe " + version.CLIDetail(apiversion.SpecVersion) + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRunDoctorChecksRuntimeHealth(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", server.URL}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "runtime healthy:") || !strings.Contains(got, "runtimeVersion:") || !strings.Contains(got, "specVersion:") || !strings.Contains(got, "schemaVersion:") {
		t.Fatalf("output = %q, want health and explicit meta fields", got)
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

func TestRunDoctorJSONReportsHealthyRuntimeMeta(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", server.URL, "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	var resp struct {
		Status         string `json:"status"`
		ExitCode       int    `json:"exitCode"`
		RuntimeHealthy bool   `json:"runtimeHealthy"`
		RuntimeVersion string `json:"runtimeVersion"`
		SpecVersion    string `json:"specVersion"`
		SchemaVersion  string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal doctor json output: %v", err)
	}
	if resp.Status != "ok" || resp.ExitCode != 0 {
		t.Fatalf("doctor response = %+v, want ok/0", resp)
	}
	if !resp.RuntimeHealthy {
		t.Fatal("runtimeHealthy = false, want true")
	}
	if resp.RuntimeVersion == "" || resp.SpecVersion == "" || resp.SchemaVersion == "" {
		t.Fatalf("doctor meta fields are empty: %+v", resp)
	}
}

func TestRunDoctorJSONReportsSpecSchemaDrift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":        "ok",
				"specVersion":   "http-control-api@v1",
				"schemaVersion": "1",
			})
		case "/v1/meta":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"runtimeVersion": "0.0.0-dev",
				"specVersion":    "http-control-api@next",
				"schemaVersion":  "999",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", server.URL, "--json"}, &out, nil); code == 0 {
		t.Fatal("exit code = 0, want non-zero for drift")
	}

	var resp struct {
		Status                string `json:"status"`
		ExitCode              int    `json:"exitCode"`
		RuntimeVersion        string `json:"runtimeVersion"`
		SpecVersion           string `json:"specVersion"`
		SchemaVersion         string `json:"schemaVersion"`
		ExpectedSpecVersion   string `json:"expectedSpecVersion"`
		ExpectedSchemaVersion string `json:"expectedSchemaVersion"`
		Error                 struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal drift doctor json output: %v", err)
	}
	if resp.Status != "error" || resp.ExitCode == 0 {
		t.Fatalf("doctor response = %+v, want error/non-zero", resp)
	}
	if resp.Error.Code != "runtime_drift" {
		t.Fatalf("error code = %q, want runtime_drift", resp.Error.Code)
	}
	if resp.SpecVersion != "http-control-api@next" || resp.SchemaVersion != "999" {
		t.Fatalf("drifted values = spec %q schema %q, want http-control-api@next / 999", resp.SpecVersion, resp.SchemaVersion)
	}
	if resp.ExpectedSpecVersion != apiversion.SpecVersion || resp.ExpectedSchemaVersion != apiversion.SchemaVersion {
		t.Fatalf("expected values = spec %q schema %q, want %q / %q", resp.ExpectedSpecVersion, resp.ExpectedSchemaVersion, apiversion.SpecVersion, apiversion.SchemaVersion)
	}
}

func TestRunDoctorJSONReportsUnreachableRuntime(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	serverURL := server.URL
	server.Close()

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", serverURL, "--json"}, &out, nil); code == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}

	var resp struct {
		Status   string `json:"status"`
		ExitCode int    `json:"exitCode"`
		Error    struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal unreachable doctor json output: %v", err)
	}
	if resp.Status != "error" || resp.ExitCode == 0 {
		t.Fatalf("doctor response = %+v, want error/non-zero", resp)
	}
	if resp.Error.Code != "runtime_unreachable" {
		t.Fatalf("error code = %q, want runtime_unreachable", resp.Error.Code)
	}
	if resp.Error.Message == "" {
		t.Fatal("error message is empty")
	}
}

func TestRunSessionStartEmitsJsonAndExportLine(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	var resp struct {
		Status          string `json:"status"`
		ExitCode        int    `json:"exitCode"`
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
	if resp.Status != "ok" || resp.ExitCode != 0 {
		t.Fatalf("status/exitCode = %q/%d, want ok/0", resp.Status, resp.ExitCode)
	}
	if resp.SessionRevision != 0 {
		t.Fatalf("sessionRevision = %d, want 0", resp.SessionRevision)
	}
	if resp.SchemaVersion == "" || resp.SpecVersion == "" {
		t.Fatalf("schema/spec versions are empty: %+v", resp)
	}
}

func TestRunSessionStartHumanReadableOutput(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	got := out.String()
	if !strings.Contains(got, "session created:") {
		t.Fatalf("output = %q, want human-readable session summary", got)
	}
	if strings.Contains(got, "export SOFTPROBE_SESSION_ID=") {
		t.Fatalf("output = %q, want shell export only behind --shell", got)
	}
}

func TestRunSessionStartShellOutput(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if code := run([]string{"session", "start", "--runtime-url", server.URL, "--shell"}, &out, nil); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	got := strings.TrimSpace(out.String())
	if !strings.HasPrefix(got, "export SOFTPROBE_SESSION_ID=") {
		t.Fatalf("output = %q, want shell export line", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("output = %q, want only one shell export line", got)
	}
}

func TestRunSessionStartSupportsModeFlag(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
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

	session, ok := st.Get(resp.SessionID)
	if !ok {
		t.Fatalf("session %q not found", resp.SessionID)
	}
	if session.Mode != "capture" {
		t.Fatalf("session mode = %q, want capture", session.Mode)
	}
}

func TestRunSessionLoadCasePersistsCaseAndReturnsZero(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
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

	session, ok := st.Get(started.SessionID)
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
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
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

	session, ok := st.Get(started.SessionID)
	if !ok {
		t.Fatalf("session %q not found", started.SessionID)
	}
	if !bytes.Contains(session.LoadedCase, []byte("checkout-happy-path")) {
		t.Fatalf("loaded case missing golden case id: %s", string(session.LoadedCase))
	}
}

func TestRunSessionLoadCaseReturnsErrorForUnknownSession(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
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
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
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
	if err := os.WriteFile(rulesPath, []byte(`{"rules":[{"name":"allow-outbound","when":{"direction":"outbound"},"then":{"action":"passthrough"}}]}`), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	var out bytes.Buffer
	args := []string{"session", "rules", "apply", "--runtime-url", server.URL, "--session", started.SessionID, "--file", rulesPath}
	if code := run(args, &out, nil); code != 0 {
		t.Fatalf("session rules apply exit code = %d, want 0", code)
	}

	session, ok := st.Get(started.SessionID)
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

func TestRunSessionStatsEmitsJSONAndHumanOutput(t *testing.T) {
	st := store.NewStore()
	session := st.Create("replay")
	if _, ok := st.RecordInjectedSpans(session.ID, 2); !ok {
		t.Fatal("record injected spans failed")
	}
	if _, ok := st.RecordExtractedSpans(session.ID, 3); !ok {
		t.Fatal("record extracted spans failed")
	}
	if _, ok := st.RecordStrictMiss(session.ID, 1); !ok {
		t.Fatal("record strict misses failed")
	}

	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	var jsonOut bytes.Buffer
	if code := run([]string{"session", "stats", "--runtime-url", server.URL, "--session", session.ID, "--json"}, &jsonOut, nil); code != 0 {
		t.Fatalf("session stats exit code = %d, want 0", code)
	}

	var jsonResp struct {
		Status          string `json:"status"`
		ExitCode        int    `json:"exitCode"`
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
		Mode            string `json:"mode"`
		Stats           struct {
			InjectedSpans  int `json:"injectedSpans"`
			ExtractedSpans int `json:"extractedSpans"`
			StrictMisses   int `json:"strictMisses"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &jsonResp); err != nil {
		t.Fatalf("unmarshal session stats output: %v", err)
	}
	if jsonResp.SessionID != session.ID {
		t.Fatalf("sessionId = %q, want %q", jsonResp.SessionID, session.ID)
	}
	if jsonResp.Status != "ok" || jsonResp.ExitCode != 0 {
		t.Fatalf("status/exitCode = %q/%d, want ok/0", jsonResp.Status, jsonResp.ExitCode)
	}
	if jsonResp.Mode != "replay" {
		t.Fatalf("mode = %q, want replay", jsonResp.Mode)
	}
	if jsonResp.Stats.InjectedSpans != 2 || jsonResp.Stats.ExtractedSpans != 3 || jsonResp.Stats.StrictMisses != 1 {
		t.Fatalf("stats = %+v, want injected=2 extracted=3 strict=1", jsonResp.Stats)
	}

	var humanOut bytes.Buffer
	if code := run([]string{"session", "stats", "--runtime-url", server.URL, "--session", session.ID}, &humanOut, nil); code != 0 {
		t.Fatalf("session stats human exit code = %d, want 0", code)
	}
	for _, want := range []string{
		"session ",
		"injectedSpans: 2",
		"extractedSpans: 3",
		"strictMisses: 1",
	} {
		if !strings.Contains(humanOut.String(), want) {
			t.Fatalf("human output = %q, want %q", humanOut.String(), want)
		}
	}
}

func TestRunSessionCloseEmitsJSONAndClosesSession(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
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

	var closeOut bytes.Buffer
	if code := run([]string{"session", "close", "--runtime-url", server.URL, "--session", started.SessionID, "--json"}, &closeOut, nil); code != 0 {
		t.Fatalf("session close exit code = %d, want 0", code)
	}

	var closed struct {
		Status    string `json:"status"`
		ExitCode  int    `json:"exitCode"`
		SessionID string `json:"sessionId"`
		Closed    bool   `json:"closed"`
	}
	if err := json.Unmarshal(closeOut.Bytes(), &closed); err != nil {
		t.Fatalf("unmarshal session close output: %v", err)
	}
	if closed.Status != "ok" || closed.ExitCode != 0 {
		t.Fatalf("status/exitCode = %q/%d, want ok/0", closed.Status, closed.ExitCode)
	}
	if closed.SessionID != started.SessionID {
		t.Fatalf("sessionId = %q, want %q", closed.SessionID, started.SessionID)
	}
	if !closed.Closed {
		t.Fatal("closed = false, want true")
	}
	if _, ok := st.Get(started.SessionID); ok {
		t.Fatalf("session %q still present after close", started.SessionID)
	}
}

func TestRunSessionPolicySetPersistsStrictPolicy(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
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

	session, ok := st.Get(started.SessionID)
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
