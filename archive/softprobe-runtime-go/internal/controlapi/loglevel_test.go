package controlapi_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/store"
)

// TestLogLevelDebugIncludesDebugLines verifies that when the logger is set at
// debug level, debug messages appear in the output.
func TestLogLevelDebugIncludesDebugLines(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	st := store.NewStore()
	mux := controlapi.NewMuxWithLogger(st, logger)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a session — should log at debug level.
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions", strings.NewReader(`{"mode":"replay"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/sessions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !strings.Contains(buf.String(), "session") {
		t.Errorf("expected debug log about session, got:\n%s", buf.String())
	}
}

// TestLogLevelWarnSuppressesDebug verifies that at warn level, debug/info lines
// are suppressed.
func TestLogLevelWarnSuppressesDebug(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	st := store.NewStore()
	mux := controlapi.NewMuxWithLogger(st, logger)
	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions", strings.NewReader(`{"mode":"replay"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/sessions: %v", err)
	}
	defer resp.Body.Close()

	if buf.Len() > 0 {
		t.Errorf("expected no log output at warn level for normal session create, got:\n%s", buf.String())
	}
}

// TestParseLevelFromEnv verifies that ParseLogLevel maps the documented
// string values to slog.Level.
func TestParseLevelFromEnv(t *testing.T) {
	cases := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tc := range cases {
		got := controlapi.ParseLogLevel(tc.input)
		if got != tc.want {
			t.Errorf("ParseLogLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
