package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// doctorCheckResult is a single structured check emitted by `softprobe
// doctor`. The shape is the contract documented in cli.md: `name`, `status`
// ∈ {ok, warn, fail, skip}, and optional `details` map with free-form
// diagnostic information.
type doctorCheckResult struct {
	Name    string         `json:"name"`
	Status  string         `json:"status"`
	Details map[string]any `json:"details,omitempty"`
}

// Well-known paths searched when $WASM_PATH is unset. Kept small and
// documented in cli.md so the "did you forget to mount the sidecar" pain
// point is diagnosable without dropping into bash.
var wasmFallbackPaths = []string{
	"/etc/envoy/sp_istio_agent.wasm",
	"/usr/local/share/softprobe/sp_istio_agent.wasm",
}

// runWASMBinaryCheck looks for the proxy WASM module, preferring $WASM_PATH
// and falling back to well-known filesystem locations. A missing binary is
// reported as warn, never fail, because the CLI can drive a runtime without
// the sidecar installed locally (e.g. on a dev box talking to a staging
// proxy).
func runWASMBinaryCheck() doctorCheckResult {
	var candidates []string
	if p := strings.TrimSpace(os.Getenv("WASM_PATH")); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, wasmFallbackPaths...)

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return doctorCheckResult{
				Name:   "wasm-binary",
				Status: "ok",
				Details: map[string]any{
					"path":    p,
					"sizeKiB": info.Size() / 1024,
				},
			}
		}
	}
	return doctorCheckResult{
		Name:   "wasm-binary",
		Status: "warn",
		Details: map[string]any{
			"message":        "WASM binary not found on well-known paths",
			"pathsInspected": candidates,
		},
	}
}

// runHeaderEchoCheck smoke-tests the proxy by sending a request with a
// synthetic `x-softprobe-session-id` header and verifying the response
// echoes it. When $SOFTPROBE_PROXY_URL is unset we return `skip` instead of
// `warn` so users not running a local proxy don't see scary output.
func runHeaderEchoCheck() doctorCheckResult {
	proxyURL := strings.TrimSpace(os.Getenv("SOFTPROBE_PROXY_URL"))
	if proxyURL == "" {
		return doctorCheckResult{
			Name:   "header-echo",
			Status: "skip",
			Details: map[string]any{
				"message": "set SOFTPROBE_PROXY_URL to enable header-echo smoke test",
			},
		}
	}
	client := newHTTPClient(3 * time.Second)
	sessionID := "doctor-" + randHex(6)
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(proxyURL, "/")+"/", nil)
	if err != nil {
		return doctorCheckResult{
			Name:   "header-echo",
			Status: "warn",
			Details: map[string]any{
				"message": fmt.Sprintf("could not build request: %v", err),
			},
		}
	}
	req.Header.Set("x-softprobe-session-id", sessionID)

	resp, err := client.Do(req)
	if err != nil {
		return doctorCheckResult{
			Name:   "header-echo",
			Status: "warn",
			Details: map[string]any{
				"message": fmt.Sprintf("proxy unreachable: %v", err),
				"url":     proxyURL,
			},
		}
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	echoed := resp.Header.Get("x-softprobe-session-id")
	if echoed != sessionID {
		return doctorCheckResult{
			Name:   "header-echo",
			Status: "warn",
			Details: map[string]any{
				"message":    "x-softprobe-session-id header not echoed by proxy (may indicate misconfig)",
				"expected":   sessionID,
				"got":        echoed,
				"proxyURL":   proxyURL,
				"httpStatus": resp.StatusCode,
			},
		}
	}
	return doctorCheckResult{
		Name:   "header-echo",
		Status: "ok",
		Details: map[string]any{
			"proxyURL": proxyURL,
		},
	}
}

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "0000000000"
	}
	return hex.EncodeToString(buf)
}
