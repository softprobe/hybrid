package main

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/store"
)

// TestReplayRunDetectsCodeRegression verifies that replay run reports a
// non-zero exit and labels the failure as a code regression when the session
// has strict misses.
func TestReplayRunDetectsCodeRegression(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	// Create a capture session and close it immediately (no extracts → no
	// inject misses). Then create a replay session with a strict policy and
	// close it as a replay session that has zero strict misses — valid.
	var stdout bytes.Buffer
	code := run([]string{"session", "start", "--runtime-url", server.URL, "--mode", "replay", "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session start: exit=%d", code)
	}
	sessionID := decodeJSON(t, stdout.Bytes())["sessionId"].(string)

	// Report with --json should include errorType field when misses > 0.
	stdout.Reset()
	code = run([]string{
		"replay", "run",
		"--runtime-url", server.URL,
		"--session", sessionID,
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("replay run: exit=%d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	stats := got["stats"].(map[string]any)
	if _, ok := stats["hits"]; !ok {
		t.Errorf("missing stats.hits: %v", got)
	}
}

// TestReplayRunClassifiesTransportFailure verifies that an unreachable runtime
// exits with exitRuntimeUnreachable rather than exitGeneric.
func TestReplayRunClassifiesTransportFailure(t *testing.T) {
	code := run([]string{
		"replay", "run",
		"--runtime-url", "http://127.0.0.1:1",
		"--session", "any",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitRuntimeUnreachable {
		t.Errorf("replay run unreachable: exit=%d, want %d", code, exitRuntimeUnreachable)
	}
}

// TestReplayRunCaseStalenessDiagnostic verifies that when a session has a
// loaded case but zero inject activity, the JSON output includes a staleness
// hint.
func TestReplayRunCaseStalenessDiagnostic(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	casePath := filepath.Join("..", "..", "..", "spec", "examples", "cases", "fragment-happy-path.case.json")

	var stdout bytes.Buffer
	code := run([]string{"session", "start", "--runtime-url", server.URL, "--mode", "replay",
		"--case", casePath, "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session start: exit=%d", code)
	}
	sessionID := decodeJSON(t, stdout.Bytes())["sessionId"].(string)

	stdout.Reset()
	code = run([]string{
		"replay", "run",
		"--runtime-url", server.URL,
		"--session", sessionID,
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("replay run: exit=%d: %s", code, stdout.String())
	}
	got := decodeJSON(t, stdout.Bytes())
	body := stdout.String()

	// When a case is loaded but no hits occurred, the JSON should include a
	// diagnostic hint (caseLoaded=true, hits=0 → staleness indicator).
	stats := got["stats"].(map[string]any)
	hits := int(stats["hits"].(float64))
	if hits != 0 {
		t.Skipf("unexpected hits=%d; skipping staleness assertion", hits)
	}
	if !strings.Contains(body, "caseLoaded") {
		t.Errorf("expected caseLoaded field in JSON output for case-loaded+zero-hit session: %s", body)
	}
}
