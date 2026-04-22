package main

import (
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"e2e/go/e2etestutil"
	"github.com/softprobe/softprobe-go/softprobe"
)

// TestReplayEgressInjectMocksUpstream issues GET /fragment on the egress listener with a replay
// session so the runtime matches the mock rule and returns the stored dependency response without
// calling the live upstream.
//
// This test exercises the same SDK authoring flow as e2e/go/go-replay/ and the other SDK harnesses.
func TestReplayEgressInjectMocksUpstream(t *testing.T) {
	runtimeURL := e2etestutil.MustEnv("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := e2etestutil.MustEnv("PROXY_URL", "http://127.0.0.1:8082")
	upstreamURL := e2etestutil.MustEnv("UPSTREAM_URL", "http://127.0.0.1:8083")
	caseFile := filepath.Join(e2etestutil.ModuleRoot(), "captured.case.json")

	e2etestutil.SkipIfRuntimeUnreachable(t, runtimeURL)

	e2etestutil.EnsureCapturedCase(t, runtimeURL, proxyURL, caseFile)
	e2etestutil.ValidateCaptureCaseTraceSemantics(t, caseFile)

	egressBase := e2etestutil.EgressHTTPBaseForTest(caseFile)
	fromCase, hostOK := e2etestutil.EgressURLFromCapturedCase(caseFile)

	e2etestutil.ResetUpstreamCounter(t, upstreamURL)

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

	if err := session.LoadCaseFromFile(caseFile); err != nil {
		t.Fatalf("LoadCaseFromFile: %v", err)
	}

	hit, err := session.FindInCase(softprobe.CaseSpanPredicate{
		Direction: "outbound",
		Method:    http.MethodGet,
		Path:      "/fragment",
	})
	if err != nil {
		t.Fatalf("FindInCase: %v", err)
	}

	priority := 100
	if err := session.MockOutbound(softprobe.MockRuleSpec{
		ID:        "fragment-egress-replay",
		Priority:  &priority,
		Direction: "outbound",
		Method:    http.MethodGet,
		Path:      "/fragment",
		Response:  hit.Response,
	}); err != nil {
		t.Fatalf("MockOutbound: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, egressBase+"/fragment", nil)
	if err != nil {
		t.Fatalf("new egress request: %v", err)
	}
	if hostOK {
		if u, err := url.Parse(fromCase); err == nil && u.Host != "" {
			req.Host = u.Host
		}
	}
	req.Header.Set("x-softprobe-session-id", session.ID())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("egress proxy request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("egress status = %d: %s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read egress body: %v", err)
	}
	if strings.TrimSpace(string(body)) != strings.TrimSpace(hit.Response.Body) {
		t.Fatalf("egress body = %q, want captured fragment body %q", strings.TrimSpace(string(body)), strings.TrimSpace(hit.Response.Body))
	}

	if got := e2etestutil.UpstreamFragmentCount(t, upstreamURL); got != 0 {
		t.Fatalf("upstream /fragment hits after egress replay inject = %d, want 0 (must not call live dependency)", got)
	}
}
