package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"softprobe-runtime/internal/controlapi"
)

// ---------- PD1.1c: universal --json envelope --------------------------------

func TestSessionLoadCaseJSONEnvelope(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	sessionID := startReplaySession(t, server.URL)
	casePath := filepath.Join(t.TempDir(), "c.case.json")
	if err := os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"c","traces":[{"resourceSpans":[]}]}`), 0o600); err != nil {
		t.Fatalf("write case: %v", err)
	}

	var stdout bytes.Buffer
	code := run([]string{"session", "load-case", "--runtime-url", server.URL, "--session", sessionID, "--file", casePath, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d, want %d: %s", code, exitOK, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	assertJSONFields(t, got, "status", "exitCode", "sessionId", "sessionRevision", "caseId", "traceCount")
	if got["caseId"] != "c" {
		t.Fatalf("caseId = %v, want c", got["caseId"])
	}
}

func TestSessionPolicySetJSONEnvelope(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	sessionID := startReplaySession(t, server.URL)

	var stdout bytes.Buffer
	code := run([]string{"session", "policy", "set", "--runtime-url", server.URL, "--session", sessionID, "--strict", "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	assertJSONFields(t, got, "status", "exitCode", "sessionId", "sessionRevision")
}

func TestSessionPolicySetFileYAML(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	sessionID := startReplaySession(t, server.URL)

	policyPath := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(policyPath, []byte("externalHttp: strict\ndefaultOnMiss: error\n"), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	var stdout bytes.Buffer
	code := run([]string{"session", "policy", "set", "--runtime-url", server.URL, "--session", sessionID, "--file", policyPath, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
}

func TestSessionRulesApplyJSONEnvelope(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	sessionID := startReplaySession(t, server.URL)

	rulesPath := filepath.Join(t.TempDir(), "r.yaml")
	if err := os.WriteFile(rulesPath, []byte("rules:\n  - match:\n      method: GET\n      host: api\n    response:\n      status: 200\n"), 0o600); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	var stdout bytes.Buffer
	code := run([]string{"session", "rules", "apply", "--runtime-url", server.URL, "--session", sessionID, "--file", rulesPath, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	assertJSONFields(t, got, "status", "exitCode", "sessionId", "sessionRevision", "ruleCount")
	if int(got["ruleCount"].(float64)) != 1 {
		t.Fatalf("ruleCount = %v, want 1", got["ruleCount"])
	}
}

func TestInspectCaseJSONEnvelope(t *testing.T) {
	casePath := filepath.Join(t.TempDir(), "c.case.json")
	body := `{"version":"1.0.0","caseId":"demo","traces":[{"resourceSpans":[{"scopeSpans":[{"spans":[{"attributes":[{"key":"url.host","value":{"stringValue":"api"}},{"key":"http.request.method","value":{"stringValue":"GET"}}]}]}]}]}]}`
	if err := os.WriteFile(casePath, []byte(body), 0o600); err != nil {
		t.Fatalf("write case: %v", err)
	}
	var stdout bytes.Buffer
	code := run([]string{"inspect", "case", "--json", casePath}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	assertJSONFields(t, got, "status", "exitCode", "caseId", "traceCount", "spanSummary")
}

// ---------- PD1.3a: session start --policy --case atomic chain --------------

func TestSessionStartWithPolicyAndCaseChain(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	policyPath := filepath.Join(t.TempDir(), "p.yaml")
	_ = os.WriteFile(policyPath, []byte("externalHttp: strict\n"), 0o600)
	casePath := filepath.Join(t.TempDir(), "c.case.json")
	_ = os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"c","traces":[{"resourceSpans":[]}]}`), 0o600)

	var stdout bytes.Buffer
	code := run([]string{"session", "start", "--runtime-url", server.URL, "--mode", "replay", "--policy", policyPath, "--case", casePath, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	if got["sessionRevision"].(float64) < 2 {
		t.Fatalf("expected revision >=2 after policy+case chain, got %v", got["sessionRevision"])
	}
}

// ---------- PD1.3d: inspect session ------------------------------------------

func TestInspectSessionHappyPath(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	sessionID := startReplaySession(t, server.URL)

	policyPath := filepath.Join(t.TempDir(), "p.json")
	_ = os.WriteFile(policyPath, []byte(`{"externalHttp":"strict"}`), 0o600)
	run([]string{"session", "policy", "set", "--runtime-url", server.URL, "--session", sessionID, "--file", policyPath}, &bytes.Buffer{}, &bytes.Buffer{})

	var stdout bytes.Buffer
	code := run([]string{"inspect", "session", "--runtime-url", server.URL, "--session", sessionID, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	assertJSONFields(t, got, "status", "exitCode", "sessionId", "sessionRevision", "mode", "stats", "policy")
}

func TestInspectSessionUnknownReturnsSessionNotFound(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	code := run([]string{"inspect", "session", "--runtime-url", server.URL, "--session", "missing"}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitSessionNotFound {
		t.Fatalf("exit = %d, want %d", code, exitSessionNotFound)
	}
}

// ---------- PD1.4: validate case/rules/suite ---------------------------------

func TestValidateCaseOKAndInvalid(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "v.case.json")
	_ = os.WriteFile(valid, []byte(`{"version":"1.0.0","caseId":"v","traces":[{"resourceSpans":[{"scopeSpans":[]}]}]}`), 0o600)
	invalid := filepath.Join(dir, "i.case.json")
	_ = os.WriteFile(invalid, []byte(`{"version":"1.0.0"}`), 0o600)

	if code := run([]string{"validate", "case", valid}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitOK {
		t.Fatalf("valid case: exit = %d", code)
	}
	if code := run([]string{"validate", "case", invalid}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitValidation {
		t.Fatalf("invalid case: exit = %d, want %d", code, exitValidation)
	}
}

func TestValidateRulesOKAndInvalid(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "r.yaml")
	_ = os.WriteFile(good, []byte("rules:\n  - match: {method: GET}\n    response: {status: 200}\n"), 0o600)
	bad := filepath.Join(dir, "b.yaml")
	_ = os.WriteFile(bad, []byte("rules: []\n"), 0o600)

	if code := run([]string{"validate", "rules", good}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitOK {
		t.Fatalf("good rules: exit = %d", code)
	}
	if code := run([]string{"validate", "rules", bad}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitValidation {
		t.Fatalf("bad rules: exit = %d, want %d", code, exitValidation)
	}
}

func TestValidateSuiteOKAndInvalid(t *testing.T) {
	dir := t.TempDir()
	casePath := filepath.Join(dir, "c.case.json")
	_ = os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"c","traces":[]}`), 0o600)

	good := filepath.Join(dir, "g.suite.yaml")
	_ = os.WriteFile(good, []byte("name: demo\ncases:\n  - path: c.case.json\n"), 0o600)
	bad := filepath.Join(dir, "b.suite.yaml")
	_ = os.WriteFile(bad, []byte("cases: []\n"), 0o600)

	if code := run([]string{"validate", "suite", good}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitOK {
		t.Fatalf("good suite: exit = %d", code)
	}
	if code := run([]string{"validate", "suite", bad}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitValidation {
		t.Fatalf("bad suite: exit = %d, want %d", code, exitValidation)
	}
}

// ---------- PD1.6a: replay run (stats wrapper) ------------------------------

func TestReplayRunJSONExposesHitsAndMisses(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	sessionID := startReplaySession(t, server.URL)

	var stdout bytes.Buffer
	code := run([]string{"replay", "run", "--runtime-url", server.URL, "--session", sessionID, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	stats := got["stats"].(map[string]any)
	if _, ok := stats["hits"]; !ok {
		t.Fatalf("json output missing stats.hits: %v", got)
	}
	if _, ok := stats["misses"]; !ok {
		t.Fatalf("json output missing stats.misses: %v", got)
	}
}

// ---------- PD1.7b: suite run (sequential) ----------------------------------

func TestSuiteRunPassesAgainstValidCases(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	caseA := filepath.Join(dir, "a.case.json")
	caseB := filepath.Join(dir, "b.case.json")
	_ = os.WriteFile(caseA, []byte(`{"version":"1.0.0","caseId":"a","traces":[{"resourceSpans":[]}]}`), 0o600)
	_ = os.WriteFile(caseB, []byte(`{"version":"1.0.0","caseId":"b","traces":[{"resourceSpans":[]}]}`), 0o600)
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte("name: demo\ncases:\n  - path: a.case.json\n  - path: b.case.json\n"), 0o600)

	var stdout bytes.Buffer
	code := run([]string{"suite", "run", "--runtime-url", server.URL, "--json", suitePath}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("suite run exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	if got["passed"].(float64) != 2 || got["failed"].(float64) != 0 {
		t.Fatalf("unexpected suite result: %v", got)
	}
}

func TestSuiteRunParallel(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	var entries []string
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, filepath.Join(".", "c"+string(rune('a'+i))+".case.json"))
		_ = os.WriteFile(p, []byte(`{"version":"1.0.0","caseId":"x","traces":[{"resourceSpans":[]}]}`), 0o600)
		entries = append(entries, "  - path: "+filepath.Base(p))
	}
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte("name: demo\ncases:\n"+strings.Join(entries, "\n")+"\n"), 0o600)

	var stdout bytes.Buffer
	code := run([]string{"suite", "run", "--runtime-url", server.URL, "--parallel", "4", "--json", suitePath}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("suite run exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	if got["total"].(float64) != 4 {
		t.Fatalf("total = %v, want 4", got["total"])
	}
}

// ---------- PD1.7d: JUnit + HTML writers -------------------------------------

func TestSuiteRunJUnitXMLOutput(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	casePath := filepath.Join(dir, "a.case.json")
	_ = os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"a","traces":[{"resourceSpans":[]}]}`), 0o600)
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte("name: demo\ncases:\n  - path: a.case.json\n"), 0o600)

	junitOut := filepath.Join(dir, "junit.xml")
	reportOut := filepath.Join(dir, "report.html")
	code := run([]string{"suite", "run", "--runtime-url", server.URL, "--junit", junitOut, "--report", reportOut, suitePath}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("suite run exit = %d", code)
	}

	raw, err := os.ReadFile(junitOut)
	if err != nil {
		t.Fatalf("read junit: %v", err)
	}
	var ts struct {
		XMLName xml.Name `xml:"testsuite"`
		Name    string   `xml:"name,attr"`
		Tests   int      `xml:"tests,attr"`
	}
	if err := xml.Unmarshal(raw, &ts); err != nil {
		t.Fatalf("junit parse: %v", err)
	}
	if ts.Tests != 1 {
		t.Fatalf("junit tests = %d, want 1", ts.Tests)
	}
	if _, err := os.Stat(reportOut); err != nil {
		t.Fatalf("html report missing: %v", err)
	}
}

// ---------- PD1.7e: suite validate -------------------------------------------

func TestSuiteValidateFlagsMissingCase(t *testing.T) {
	dir := t.TempDir()
	suitePath := filepath.Join(dir, "s.suite.yaml")
	_ = os.WriteFile(suitePath, []byte("name: demo\ncases:\n  - path: nope.case.json\n"), 0o600)
	code := run([]string{"suite", "validate", suitePath}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d", code, exitValidation)
	}
}

// ---------- PD1.7f: suite diff -----------------------------------------------

func TestSuiteDiffReportsDrift(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.case.json")
	cur := filepath.Join(dir, "cur.case.json")
	_ = os.WriteFile(base, []byte(`{"version":"1.0.0","caseId":"a","traces":[{"resourceSpans":[{"scopeSpans":[{"spans":[{"attributes":[{"key":"http.request.method","value":{"stringValue":"GET"}},{"key":"url.host","value":{"stringValue":"api"}},{"key":"url.path","value":{"stringValue":"/a"}},{"key":"http.response.status_code","value":{"stringValue":"200"}}]}]}]}]}]}`), 0o600)
	_ = os.WriteFile(cur, []byte(`{"version":"1.0.0","caseId":"a","traces":[{"resourceSpans":[{"scopeSpans":[{"spans":[{"attributes":[{"key":"http.request.method","value":{"stringValue":"GET"}},{"key":"url.host","value":{"stringValue":"api"}},{"key":"url.path","value":{"stringValue":"/a"}},{"key":"http.response.status_code","value":{"stringValue":"500"}}]}]}]}]}]}`), 0o600)

	var stdout bytes.Buffer
	code := run([]string{"suite", "diff", "--baseline", base, "--current", cur, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("suite diff exit = %d", code)
	}
	got := decodeJSON(t, stdout.Bytes())
	if len(got["added"].([]any)) == 0 || len(got["removed"].([]any)) == 0 {
		t.Fatalf("expected drift in both directions, got %v", got)
	}
}

// ---------- PD1.5: capture run -----------------------------------------------

func TestCaptureRunWiresDriverAndWritesCase(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "captured.case.json")
	sentinel := filepath.Join(dir, "driver-ran")

	// Driver writes a file so we can assert it executed with the session id.
	var stdout bytes.Buffer
	code := run([]string{
		"capture", "run",
		"--runtime-url", server.URL,
		"--driver", "sh -c 'echo $SOFTPROBE_SESSION_ID > " + sentinel + "'",
		"--out", outPath,
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("capture run exit = %d: %s", code, stdout.String())
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("driver did not run: %v", err)
	}
	sentinelBody, _ := os.ReadFile(sentinel)
	if strings.TrimSpace(string(sentinelBody)) == "" {
		t.Fatalf("driver received empty SOFTPROBE_SESSION_ID")
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("capture output not written: %v", err)
	}
}

func TestCaptureRunTimeoutExitsNonZero(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	var stderr bytes.Buffer
	code := run([]string{
		"capture", "run",
		"--runtime-url", server.URL,
		"--driver", "sleep 5",
		"--timeout", "200ms",
	}, &bytes.Buffer{}, &stderr)
	if code == exitOK {
		t.Fatalf("expected non-zero exit on timeout, got 0")
	}
}

func TestCaptureRunRedactFileIsApplied(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	dir := t.TempDir()
	redact := filepath.Join(dir, "r.yaml")
	_ = os.WriteFile(redact, []byte("rules:\n  - match: {method: GET}\n    response: {status: 200}\n"), 0o600)

	code := run([]string{
		"capture", "run",
		"--runtime-url", server.URL,
		"--driver", "true",
		"--redact-file", redact,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("capture run with redact-file exit = %d", code)
	}
}

// ---------- PD1.8c: scrub ----------------------------------------------------

func TestScrubReplacesBearerTokens(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "t.txt")
	_ = os.WriteFile(target, []byte("Authorization: Bearer abc.def.ghi\nemail: foo@example.com\n"), 0o600)

	code := run([]string{"scrub", target}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("scrub exit = %d", code)
	}
	out, _ := os.ReadFile(target)
	if strings.Contains(string(out), "abc.def.ghi") {
		t.Fatalf("token not redacted: %s", out)
	}
	if strings.Contains(string(out), "foo@example.com") {
		t.Fatalf("email not redacted: %s", out)
	}
}

// ---------- PD1.8d: completion ----------------------------------------------

func TestCompletionEmitsShellSnippets(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			var stdout bytes.Buffer
			code := run([]string{"completion", shell}, &stdout, &bytes.Buffer{})
			if code != exitOK {
				t.Fatalf("exit = %d", code)
			}
			if len(stdout.Bytes()) < 50 {
				t.Fatalf("%s completion looks empty: %q", shell, stdout.String())
			}
		})
	}
}

// ---------- PD1.8b: export otlp ---------------------------------------------

func TestExportOTLPPostsEachTrace(t *testing.T) {
	var posts int
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		posts++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)

	casePath := filepath.Join(t.TempDir(), "c.case.json")
	_ = os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"c","traces":[{"resourceSpans":[]},{"resourceSpans":[]}]}`), 0o600)

	code := run([]string{"export", "otlp", "--case", casePath, "--endpoint", collector.URL + "/v1/traces"}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	if posts != 2 {
		t.Fatalf("posts = %d, want 2", posts)
	}
}

// ---------- helpers ---------------------------------------------------------

func startReplaySession(t *testing.T, runtimeURL string) string {
	t.Helper()
	var stdout bytes.Buffer
	code := run([]string{"session", "start", "--runtime-url", runtimeURL, "--mode", "replay", "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session start exit = %d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	id, ok := got["sessionId"].(string)
	if !ok {
		t.Fatalf("sessionId missing: %v", got)
	}
	return id
}

func decodeJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json decode: %v: %s", err, raw)
	}
	return out
}

func assertJSONFields(t *testing.T, got map[string]any, fields ...string) {
	t.Helper()
	for _, f := range fields {
		if _, ok := got[f]; !ok {
			t.Fatalf("missing field %q in JSON envelope: %v", f, got)
		}
	}
}
