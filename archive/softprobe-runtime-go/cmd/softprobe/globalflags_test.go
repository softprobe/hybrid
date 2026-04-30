package main

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"softprobe-runtime/internal/controlapi"
)

func TestGlobalVerboseEmitsDiagnosticsOnStderr(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	var stderr bytes.Buffer
	code := run([]string{"--verbose", "doctor", "--runtime-url", server.URL}, &bytes.Buffer{}, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "GET ") {
		t.Fatalf("--verbose should log HTTP requests; stderr=%q", stderr.String())
	}
}

func TestGlobalQuietSuppressesNonErrorOutput(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	var stdout bytes.Buffer
	code := run([]string{"--quiet", "doctor", "--runtime-url", server.URL}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("--quiet should suppress stdout, got %q", stdout.String())
	}
}

func TestHelpFlagPrintsUsageAndExitsOK(t *testing.T) {
	var stdout bytes.Buffer
	code := run([]string{"--help"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d, want 0 for --help", code)
	}
	if !strings.Contains(stdout.String(), "softprobe") {
		t.Fatalf("--help should print usage, got %q", stdout.String())
	}
}

func TestNoColorEnvHonored(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// sanity: we don't currently emit ANSI, but the flag infra must accept
	// NO_COLOR without surfacing an error.
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)
	code := run([]string{"doctor", "--runtime-url", server.URL}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
}
