// End-to-end replay check mirroring e2e/jest-replay/, e2e/pytest-replay/,
// and e2e/junit-replay/. Drives the compose stack through the Go SDK's
// FindInCase + MockOutbound path. See docs/design.md §3.2.
package goreplay

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"e2e/go/e2etestutil"
	"github.com/softprobe/softprobe-go/softprobe"
)

// TestFragmentReplayThroughTheMesh drives `/hello` on the app with a
// replay session whose sole rule is a MockOutbound for `/fragment` built
// from the captured response. Assertion: the app returns the full
// composed body without the upstream being contacted.
func TestFragmentReplayThroughTheMesh(t *testing.T) {
	runtimeURL := e2etestutil.MustEnv("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:8080")
	appURL := e2etestutil.MustEnv("APP_URL", "http://127.0.0.1:8081")

	e2etestutil.SkipIfRuntimeUnreachable(t, runtimeURL)
	e2etestutil.SkipIfURLUnreachable(t, "app", appURL+"/health")

	casePath := filepath.Join(e2etestutil.RepoRoot(), "spec", "examples", "cases", "fragment-happy-path.case.json")

	sp := softprobe.New(softprobe.Options{BaseURL: runtimeURL, APIToken: e2etestutil.APIKey()})
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
