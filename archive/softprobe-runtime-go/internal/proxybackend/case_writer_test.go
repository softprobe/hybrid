package proxybackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCapturedCasePreservesParentSpanId(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.case.json")
	t.Setenv("SOFTPROBE_CAPTURE_CASE_PATH", out)

	// OTLP JSON uses base64 for traceId (16 bytes) / spanId (8 bytes) / parentSpanId (8 bytes).
	payload := []byte(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"traceId": "AAAAAAAAAAAAAAAAAAAAAA==",
					"spanId": "AAAAAAAAAAA=",
					"parentSpanId": "AQIDBAUGBwg=",
					"name": "/example",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"sess_test"}}
					]
				}]
			}]
		}]
	}`)

	if err := WriteCapturedCase("sess_test", [][]byte{payload}); err != nil {
		t.Fatalf("WriteCapturedCase: %v", err)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(body), `"parentSpanId"`) {
		t.Fatalf("expected parentSpanId in written case, got:\n%s", body)
	}
}
