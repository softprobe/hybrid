package controlapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"softprobe-runtime/internal/store"
)

func TestCloseCaptureSessionWritesValidCaseFile(t *testing.T) {
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if err := os.MkdirAll("e2e", 0o755); err != nil {
		t.Fatalf("mkdir e2e: %v", err)
	}

	st := store.NewStore()
	mux := NewMux(st)

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"mode":"capture"}`))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)

	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	tracePayload := []byte(`{
		"resourceSpans": [{
			"resource": {"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}}]},
			"scopeSpans": [{
				"spans": [{
					"traceId":"5b8efff798038103d269b633813fc60c",
					"spanId":"051581bf3cb55c13",
					"attributes": [
						{"key":"sp.span.type","value":{"stringValue":"extract"}},
						{"key":"sp.session.id","value":{"stringValue":"` + created.SessionID + `"}},
						{"key":"sp.traffic.direction","value":{"stringValue":"outbound"}},
						{"key":"url.host","value":{"stringValue":"api.stripe.com"}},
						{"key":"url.path","value":{"stringValue":"/v1/payment_intents"}},
						{"key":"http.response.status_code","value":{"intValue":200}}
					]
				}]
			}]
		}]
	}`)

	traceReq := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(tracePayload))
	traceReq.Header.Set("Content-Type", "application/json")
	traceRec := httptest.NewRecorder()
	mux.ServeHTTP(traceRec, traceReq)
	if traceRec.Code/100 != 2 {
		t.Fatalf("trace status = %d, want 2xx", traceRec.Code)
	}

	closeReq := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+created.SessionID+"/close", nil)
	closeRec := httptest.NewRecorder()
	mux.ServeHTTP(closeRec, closeReq)
	if closeRec.Code != http.StatusOK {
		t.Fatalf("close status = %d, want 200", closeRec.Code)
	}

	generated := filepath.Join("e2e", "captured.case.json")
	if _, err := os.Stat(generated); err != nil {
		t.Fatalf("captured case file missing: %v", err)
	}

	cmd := exec.Command(
		"npx",
		"-y",
		"ajv-cli@5",
		"validate",
		"-s",
		filepath.Join(oldWD, "..", "..", "..", "spec", "schemas", "case.schema.json"),
		"-r",
		filepath.Join(oldWD, "..", "..", "..", "spec", "schemas", "case-trace.schema.json"),
		"-d",
		generated,
		"--spec=draft2020",
	)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("case validation failed: %v\n%s", err, output)
	}
}
