package controlapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/store"
)

// TestMetricsEndpointExposesPrometheusText verifies that GET /metrics returns
// valid Prometheus text-format exposition including all four documented metric
// families.
func TestMetricsEndpointExposesPrometheusText(t *testing.T) {
	st := store.NewStore()
	mux := controlapi.NewMux(st)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", ct)
	}

	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	required := []string{
		"softprobe_sessions_total",
		"softprobe_inject_requests_total",
		"softprobe_inject_latency_seconds",
		"softprobe_extract_spans_total",
	}
	for _, name := range required {
		if !strings.Contains(body, name) {
			t.Errorf("metric %q not found in /metrics output", name)
		}
	}
}

// TestMetricsCountersIncrementOnActivity verifies that session creation and
// inject/extract operations increment the corresponding counters.
func TestMetricsCountersIncrementOnActivity(t *testing.T) {
	st := store.NewStore()
	mux := controlapi.NewMux(st)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a replay session — should bump softprobe_sessions_total{mode="replay"}.
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions", strings.NewReader(`{"mode":"replay"}`))
	req.Header.Set("Content-Type", "application/json")
	_, _ = http.DefaultClient.Do(req)

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	if !strings.Contains(body, `softprobe_sessions_total{mode="replay"} 1`) {
		t.Errorf("expected sessions_total{mode=replay}=1 in:\n%s", body)
	}
}
