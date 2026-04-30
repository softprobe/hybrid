package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/store"
)

// Exit codes from docs-site/reference/cli.md#exit-codes are part of the CLI
// stability contract. Every error path must map to the documented code so
// that CI pipelines and AI agents can dispatch on $? without grepping stderr.

func TestExitCodeInvalidArgsOnMissingFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"session load-case without args", []string{"session", "load-case"}},
		{"session stats without session", []string{"session", "stats", "--runtime-url", "http://127.0.0.1:1"}},
		{"unknown subcommand", []string{"nosuchcommand"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := run(tc.args, &bytes.Buffer{}, &bytes.Buffer{})
			if code != exitInvalidArgs {
				t.Fatalf("exit = %d, want %d (invalid args)", code, exitInvalidArgs)
			}
		})
	}
}

func TestExitCodeRuntimeUnreachableOnDeadServer(t *testing.T) {
	// Use a port that nothing is listening on.
	deadURL := "http://127.0.0.1:1"

	cases := []struct {
		name string
		args []string
	}{
		{"session start", []string{"session", "start", "--runtime-url", deadURL, "--json"}},
		{"session stats", []string{"session", "stats", "--runtime-url", deadURL, "--session", "s"}},
		{"session close", []string{"session", "close", "--runtime-url", deadURL, "--session", "s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := run(tc.args, &bytes.Buffer{}, &bytes.Buffer{})
			if code != exitRuntimeUnreachable {
				t.Fatalf("exit = %d, want %d (runtime unreachable)", code, exitRuntimeUnreachable)
			}
		})
	}
}

func TestExitCodeSessionNotFoundOnUnknownSessionEnvelope(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	casePath := filepath.Join(t.TempDir(), "demo.case.json")
	if err := os.WriteFile(casePath, []byte(`{"version":"1.0.0","caseId":"demo","traces":[]}`), 0o600); err != nil {
		t.Fatalf("write case file: %v", err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{"session stats", []string{"session", "stats", "--runtime-url", server.URL, "--session", "missing"}},
		{"session close", []string{"session", "close", "--runtime-url", server.URL, "--session", "missing"}},
		{"session load-case", []string{"session", "load-case", "--runtime-url", server.URL, "--session", "missing", "--file", casePath}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := run(tc.args, &bytes.Buffer{}, &bytes.Buffer{})
			if code != exitSessionNotFound {
				t.Fatalf("exit = %d, want %d (session not found)", code, exitSessionNotFound)
			}
		})
	}
}

func TestExitCodeDoctorFailOnUnhealthyRuntime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unhealthy", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	code := run([]string{"doctor", "--runtime-url", server.URL}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitDoctorFail {
		t.Fatalf("exit = %d, want %d (doctor fail)", code, exitDoctorFail)
	}
}

func TestExitCodeDoctorFailOnUnreachableRuntime(t *testing.T) {
	code := run([]string{"doctor", "--runtime-url", "http://127.0.0.1:1"}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitDoctorFail {
		t.Fatalf("exit = %d, want %d (doctor fail from unreachable)", code, exitDoctorFail)
	}
}

func TestExitCodeValidationOnMalformedCaseFile(t *testing.T) {
	casePath := filepath.Join(t.TempDir(), "bad.case.json")
	if err := os.WriteFile(casePath, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write bad case: %v", err)
	}

	code := run([]string{"inspect", "case", casePath}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d (validation)", code, exitValidation)
	}
}

func TestExitCodeOKOnHappyPath(t *testing.T) {
	_ = store.NewStore() // compile-check
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	code := run([]string{"doctor", "--runtime-url", server.URL}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (ok)", code, exitOK)
	}
}
