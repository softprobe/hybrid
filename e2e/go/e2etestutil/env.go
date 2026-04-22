package e2etestutil

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// MustEnv returns os.Getenv(key) or def if empty.
func MustEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// APIKey returns the value of SOFTPROBE_API_KEY, or "" if unset.
// Pass the result to softprobe.WithAPIToken when constructing the client so
// the suite works against both the OSS local runtime and the hosted service.
func APIKey() string {
	return strings.TrimSpace(os.Getenv("SOFTPROBE_API_KEY"))
}

// SkipIfRuntimeUnreachable calls t.Skip if the runtime /health endpoint
// doesn't respond with 200 within 2 s. Use this at the top of every test
// that targets a runtime that may not be running (e.g. when running a single
// suite in isolation without the compose stack).
func SkipIfRuntimeUnreachable(t *testing.T, runtimeURL string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(runtimeURL, "/") + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf("runtime unreachable at %s (%v) — skipping", runtimeURL, err)
	}
	resp.Body.Close()
}

// SkipIfURLUnreachable skips if the given URL (expected to return 200 on GET) is down.
func SkipIfURLUnreachable(t *testing.T, label, rawURL string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf("%s unreachable at %s (%v) — skipping", label, rawURL, fmt.Sprintf("%v", err))
	}
	resp.Body.Close()
}

// WaitForFile polls until path exists or times out.
func WaitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("file %s not created", path)
}
