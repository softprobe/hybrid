// End-to-end replay check mirroring e2e/jest-replay/, e2e/pytest-replay/,
// and e2e/junit-replay/. Drives the compose stack through the Go SDK's
// FindInCase + MockOutbound path. See docs/design.md §3.2.
package goreplay

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"softprobe-go/softprobe"
)

func mustEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// TestFragmentReplayThroughTheMesh drives `/hello` on the app with a
// replay session whose sole rule is a MockOutbound for `/fragment` built
// from the captured response. Assertion: the app returns the full
// composed body without the upstream being contacted.
func TestFragmentReplayThroughTheMesh(t *testing.T) {
	runtimeURL := mustEnv("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:8080")
	appURL := mustEnv("APP_URL", "http://127.0.0.1:8081")

	// Resolve the golden case relative to the test binary so `go test` works
	// from either the repo root or the go-replay module directory.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	casePath := filepath.Join(wd, "..", "..", "spec", "examples", "cases", "fragment-happy-path.case.json")
	if _, err := os.Stat(casePath); err != nil {
		t.Fatalf("case file not found at %s: %v", casePath, err)
	}

	sp := softprobe.New(softprobe.Options{BaseURL: runtimeURL})
	session, err := sp.StartSession("replay")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Logf("close session: %v", err)
		}
	})

	if err := session.LoadCaseFromFile(casePath); err != nil {
		t.Fatalf("LoadCaseFromFile: %v", err)
	}

	hit, err := session.FindInCase(softprobe.CaseSpanPredicate{
		Direction: "outbound",
		Method:    "GET",
		Path:      "/fragment",
	})
	if err != nil {
		t.Fatalf("FindInCase: %v", err)
	}

	priority := 100
	if err := session.MockOutbound(softprobe.MockRuleSpec{
		ID:        "fragment-replay",
		Priority:  &priority,
		Direction: "outbound",
		Method:    "GET",
		Path:      "/fragment",
		Response:  hit.Response,
	}); err != nil {
		t.Fatalf("MockOutbound: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, appURL+"/hello", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("x-softprobe-session-id", session.ID())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /hello: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, string(body))
	}

	// The SUT composes its own response from the dependency body, so the
	// replay is proven when both fields come through with the expected
	// values — `message` from the app itself and `dep` from the mocked
	// outbound call registered via MockOutbound.
	var payload struct {
		Message string `json:"message"`
		Dep     string `json:"dep"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body: %v: %s", err, string(body))
	}
	if payload.Message != "hello" {
		t.Fatalf("message = %q, want %q (body=%s)", payload.Message, "hello", string(body))
	}
	if payload.Dep != "ok" {
		t.Fatalf("dep = %q, want %q (body=%s)", payload.Dep, "ok", string(body))
	}
}
