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
)

// ---------------------------------------------------------------------------
// Fix 1 — SOFTPROBE_RUNTIME_URL env var
// ---------------------------------------------------------------------------

// TestRuntimeURLEnvVarHonoredByDoctor verifies that doctor uses SOFTPROBE_RUNTIME_URL
// when --runtime-url is not passed.
func TestRuntimeURLEnvVarHonoredByDoctor(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	t.Setenv("SOFTPROBE_RUNTIME_URL", server.URL)

	var stdout bytes.Buffer
	code := run([]string{"doctor"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("doctor exit=%d stdout=%s", code, stdout.String())
	}
}

// TestRuntimeURLEnvVarHonoredBySessionStart verifies session start uses SOFTPROBE_RUNTIME_URL.
func TestRuntimeURLEnvVarHonoredBySessionStart(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	t.Setenv("SOFTPROBE_RUNTIME_URL", server.URL)

	var stdout bytes.Buffer
	code := run([]string{"session", "start", "--mode", "replay", "--json"}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session start exit=%d stdout=%s", code, stdout.String())
	}
	var body map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["sessionId"]; !ok {
		t.Fatalf("missing sessionId: %v", body)
	}
}

// TestRuntimeURLFlagOverridesEnvVar verifies --runtime-url takes precedence over the env var.
func TestRuntimeURLFlagOverridesEnvVar(t *testing.T) {
	st := store.NewStore()
	server := httptest.NewServer(controlapi.NewMux(st))
	t.Cleanup(server.Close)

	// Point env var at a dead address; flag points at the real server.
	t.Setenv("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:1")

	var stdout bytes.Buffer
	code := run([]string{"doctor", "--runtime-url", server.URL}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("doctor exit=%d stdout=%s", code, stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Fix 3 — `softprobe cases get <captureID> [--out PATH]`
// ---------------------------------------------------------------------------

// TestCasesGetPrintsToStdout verifies that `cases get` fetches and prints the case.
func TestCasesGetPrintsToStdout(t *testing.T) {
	st := store.NewStore()
	// Inject a fake hosted GET /v1/captures/{id} handler alongside the normal mux.
	mux := http.NewServeMux()
	mux.Handle("/", controlapi.NewMux(st))
	mux.HandleFunc("/v1/captures/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"caseId":"test-case","traces":[]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	code := run([]string{
		"cases", "get",
		"--runtime-url", server.URL,
		"sess_abc123",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("cases get exit=%d stdout=%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "test-case") {
		t.Fatalf("expected case JSON in stdout, got: %s", stdout.String())
	}
}

// TestCasesGetWritesToFile verifies that --out writes the case to a file.
func TestCasesGetWritesToFile(t *testing.T) {
	st := store.NewStore()
	mux := http.NewServeMux()
	mux.Handle("/", controlapi.NewMux(st))
	mux.HandleFunc("/v1/captures/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"caseId":"file-case","traces":[]}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	outPath := filepath.Join(t.TempDir(), "out.case.json")

	var stdout bytes.Buffer
	code := run([]string{
		"cases", "get",
		"--runtime-url", server.URL,
		"--out", outPath,
		"sess_abc123",
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("cases get --out exit=%d stdout=%s", code, stdout.String())
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if !strings.Contains(string(data), "file-case") {
		t.Fatalf("unexpected file contents: %s", data)
	}
}

// TestCasesGetNotFoundReturnsSessionNotFound checks 404 maps to exitSessionNotFound.
func TestCasesGetNotFoundReturnsSessionNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/captures/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"session_not_found"}}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	code := run([]string{
		"cases", "get",
		"--runtime-url", server.URL,
		"sess_missing",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if code != exitSessionNotFound {
		t.Fatalf("want exitSessionNotFound(%d), got %d", exitSessionNotFound, code)
	}
}

// ---------------------------------------------------------------------------
// Fix 5 — session close --out downloads the case on hosted runtime
// ---------------------------------------------------------------------------

// TestSessionCloseOutDownloadsCaseFromHosted verifies that when --out is given
// and the runtime returns no capturePath (hosted pattern), the CLI fetches the
// case via GET /v1/captures/{id} and writes it to the --out file.
func TestSessionCloseOutDownloadsCaseFromHosted(t *testing.T) {
	const sessionID = "sess_hosted_abc"
	const caseJSON = `{"caseId":"hosted-case","traces":[]}`

	mux := http.NewServeMux()
	// close endpoint returns no capturePath (hosted runtime pattern)
	mux.HandleFunc("/v1/sessions/"+sessionID+"/close", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessionId":"` + sessionID + `","closed":true}`))
	})
	// captures endpoint returns the actual capture JSON
	mux.HandleFunc("/v1/captures/"+sessionID, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(caseJSON))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	outPath := filepath.Join(t.TempDir(), "captured.case.json")

	var stdout bytes.Buffer
	code := run([]string{
		"session", "close",
		"--runtime-url", server.URL,
		"--session", sessionID,
		"--out", outPath,
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session close exit=%d stdout=%s", code, stdout.String())
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("case file not written: %v", err)
	}
	if !strings.Contains(string(data), "hosted-case") {
		t.Fatalf("unexpected case file contents: %s", data)
	}
	if !strings.Contains(stdout.String(), "wrote") {
		t.Fatalf("expected 'wrote' in output, got: %s", stdout.String())
	}
}

// TestSessionCloseOutOSSStillWritesViaCapturePathResponse confirms that the
// existing OSS path (runtime writes the file, returns capturePath) still works.
func TestSessionCloseOutOSSStillWritesViaCapturePathResponse(t *testing.T) {
	const sessionID = "sess_oss_abc"

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sessions/"+sessionID+"/close", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessionId":"` + sessionID + `","closed":true,"capturePath":"/var/cases/` + sessionID + `.case.json"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	code := run([]string{
		"session", "close",
		"--runtime-url", server.URL,
		"--session", sessionID,
	}, &stdout, &bytes.Buffer{})
	if code != exitOK {
		t.Fatalf("session close exit=%d stdout=%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "capturePath") {
		t.Fatalf("expected capturePath in output, got: %s", stdout.String())
	}
}
