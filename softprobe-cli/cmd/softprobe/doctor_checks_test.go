package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"softprobe-cli/internal/apiversion"
	"softprobe-cli/internal/controlapi"
)

type doctorCheck struct {
	Name    string                 `json:"name"`
	Status  string                 `json:"status"`
	Details map[string]interface{} `json:"details"`
}

type doctorJSON struct {
	Status        string        `json:"status"`
	ExitCode      int           `json:"exitCode"`
	CLIVersion    string        `json:"cliVersion"`
	RuntimeVersion string       `json:"runtimeVersion"`
	Checks        []doctorCheck `json:"checks"`
}

func findCheck(t *testing.T, checks []doctorCheck, name string) doctorCheck {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("check %q not found in %+v", name, checks)
	return doctorCheck{}
}

func TestDoctorJSONIncludesStructuredChecks(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	// Point WASM_PATH somewhere that doesn't exist to exercise the warning.
	t.Setenv("WASM_PATH", filepath.Join(t.TempDir(), "nope.wasm"))
	// No proxy URL → header-echo check warns (skipped, not failed).
	t.Setenv("SOFTPROBE_PROXY_URL", "")

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", server.URL, "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit = %d, want 0 (warnings don't fail)", code)
	}

	var resp doctorJSON
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, out.String())
	}
	if resp.Status != "ok" || resp.ExitCode != 0 {
		t.Fatalf("status/exitCode = %q/%d, want ok/0", resp.Status, resp.ExitCode)
	}
	if resp.CLIVersion == "" {
		t.Fatalf("cliVersion empty; raw=%s", out.String())
	}

	wasm := findCheck(t, resp.Checks, "wasm-binary")
	if wasm.Status != "warn" {
		t.Fatalf("wasm-binary status = %q, want warn (file absent)", wasm.Status)
	}
	echo := findCheck(t, resp.Checks, "header-echo")
	if echo.Status != "warn" && echo.Status != "skip" {
		t.Fatalf("header-echo status = %q, want warn or skip when proxy URL missing", echo.Status)
	}
	hc := findCheck(t, resp.Checks, "runtime-reachable")
	if hc.Status != "ok" {
		t.Fatalf("runtime-reachable status = %q, want ok", hc.Status)
	}
}

func TestDoctorWasmBinaryDetectedWhenPresent(t *testing.T) {
	server := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(server.Close)

	wasmDir := t.TempDir()
	wasmPath := filepath.Join(wasmDir, "sp_istio_agent.wasm")
	if err := os.WriteFile(wasmPath, []byte("\x00asm"), 0o600); err != nil {
		t.Fatalf("write wasm: %v", err)
	}
	t.Setenv("WASM_PATH", wasmPath)

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", server.URL, "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var resp doctorJSON
	_ = json.Unmarshal(out.Bytes(), &resp)
	wasm := findCheck(t, resp.Checks, "wasm-binary")
	if wasm.Status != "ok" {
		t.Fatalf("wasm-binary status = %q, want ok", wasm.Status)
	}
}

func TestDoctorHeaderEchoSuccessful(t *testing.T) {
	rt := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(rt.Close)

	// A mock "proxy" that echoes x-softprobe-session-id.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-softprobe-session-id", r.Header.Get("x-softprobe-session-id"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("SOFTPROBE_PROXY_URL", proxy.URL)

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", rt.URL, "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var resp doctorJSON
	_ = json.Unmarshal(out.Bytes(), &resp)
	echo := findCheck(t, resp.Checks, "header-echo")
	if echo.Status != "ok" {
		t.Fatalf("header-echo status = %q, want ok; details=%+v", echo.Status, echo.Details)
	}
}

func TestDoctorHeaderEchoWarnsWhenProxyDrops(t *testing.T) {
	rt := httptest.NewServer(controlapi.NewMux())
	t.Cleanup(rt.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("SOFTPROBE_PROXY_URL", proxy.URL)

	var out bytes.Buffer
	if code := run([]string{"doctor", "--runtime-url", rt.URL, "--json"}, &out, nil); code != 0 {
		t.Fatalf("exit = %d, want 0 (warnings non-fatal)", code)
	}
	var resp doctorJSON
	_ = json.Unmarshal(out.Bytes(), &resp)
	echo := findCheck(t, resp.Checks, "header-echo")
	if echo.Status != "warn" {
		t.Fatalf("header-echo status = %q, want warn", echo.Status)
	}
}
