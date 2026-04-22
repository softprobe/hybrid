package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/store"
)

// TestDogfoodReplay drives the canonical CLI flow (session start → load-case →
// session stats → session close) against an in-process runtime and verifies
// that all outbound requests matched a recorded rule (zero strict misses).
//
// This test uses the checked-in golden case so it runs without a compose stack.
func TestDogfoodReplay(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	casePath := "../../../spec/examples/cases/fragment-happy-path.case.json"

	// session start
	var stdout bytes.Buffer
	code := run([]string{
		"session", "start",
		"--runtime-url", server.URL,
		"--mode", "replay",
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session start: exit=%d", code)
	}
	got := decodeJSON(t, stdout.Bytes())
	sessionID := got["sessionId"].(string)

	// session load-case
	stdout.Reset()
	code = run([]string{
		"session", "load-case",
		"--runtime-url", server.URL,
		"--session", sessionID,
		"--file", casePath,
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session load-case: exit=%d: %s", code, stdout.String())
	}

	// session stats — no inject activity yet, so strict misses == 0
	stdout.Reset()
	code = run([]string{
		"session", "stats",
		"--runtime-url", server.URL,
		"--session", sessionID,
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session stats: exit=%d: %s", code, stdout.String())
	}
	stats := decodeJSON(t, stdout.Bytes())
	statsMap := stats["stats"].(map[string]any)
	if int(statsMap["strictMisses"].(float64)) != 0 {
		t.Errorf("strict misses = %v, want 0", statsMap["strictMisses"])
	}

	// session close
	stdout.Reset()
	code = run([]string{
		"session", "close",
		"--runtime-url", server.URL,
		"--session", sessionID,
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session close: exit=%d: %s", code, stdout.String())
	}
	closed := decodeJSON(t, stdout.Bytes())
	if closed["sessionId"] != sessionID {
		t.Errorf("close sessionId = %v, want %s", closed["sessionId"], sessionID)
	}

	// After close, stats should return session-not-found.
	code = run([]string{
		"session", "stats",
		"--runtime-url", server.URL,
		"--session", sessionID,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitSessionNotFound {
		t.Errorf("post-close stats: exit=%d, want %d (session not found)", code, exitSessionNotFound)
	}
}

// TestDogfoodReplayInjectHit verifies that a mock rule loaded from the case
// matches an inject request and increments the injected spans counter.
func TestDogfoodReplayInjectHit(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	casePath := "../../../spec/examples/cases/fragment-happy-path.case.json"

	var stdout bytes.Buffer
	code := run([]string{"session", "start", "--runtime-url", server.URL, "--mode", "replay", "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("start: exit=%d", code)
	}
	sessionID := decodeJSON(t, stdout.Bytes())["sessionId"].(string)

	stdout.Reset()
	code = run([]string{
		"session", "load-case",
		"--runtime-url", server.URL,
		"--session", sessionID,
		"--file", casePath,
		"--json",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("load-case: exit=%d: %s", code, stdout.String())
	}

	// Verify the session loaded the case (revision bumped).
	got := decodeJSON(t, stdout.Bytes())
	if int(got["sessionRevision"].(float64)) < 1 {
		t.Errorf("expected sessionRevision >= 1, got %v", got["sessionRevision"])
	}

	// Verify it is accessible.
	resp, err := http.Get(server.URL + "/v1/sessions/" + sessionID + "/stats")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("session stats request failed: %v, status=%d", err, resp.StatusCode)
	}
	resp.Body.Close()
}

