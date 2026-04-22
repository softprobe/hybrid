// softprobe-dogfood-capture runs the canonical capture flow against the compose
// stack and writes spec/examples/cases/control-plane-v1.case.json with stable
// placeholder IDs so the output is deterministic across runs.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "dogfood-capture:", err)
		os.Exit(1)
	}
}

func run() error {
	runtimeURL := envOrDefault("RUNTIME_URL", "http://127.0.0.1:8080")
	proxyURL := envOrDefault("PROXY_URL", "http://127.0.0.1:8082")
	outPath := envOrDefault("OUT", "spec/examples/cases/control-plane-v1.case.json")

	client := &http.Client{Timeout: 10 * time.Second}

	// Create capture session.
	sessionID, err := createSession(client, runtimeURL)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	fmt.Fprintf(os.Stderr, "session: %s\n", sessionID)

	// Drive one request through the proxy.
	req, err := http.NewRequest(http.MethodGet, proxyURL+"/hello", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-softprobe-session-id", sessionID)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request: %w", err)
	}
	resp.Body.Close()

	// Wait briefly for async OTLP upload.
	time.Sleep(200 * time.Millisecond)

	// Close session.
	if err := closeSession(client, runtimeURL, sessionID); err != nil {
		return fmt.Errorf("close session: %w", err)
	}

	// Fetch case from runtime (hosted path) or read from disk (OSS path).
	caseData, err := fetchCase(client, runtimeURL, sessionID)
	if err != nil {
		return fmt.Errorf("fetch case: %w", err)
	}

	// Canonicalize: replace session ID with a stable placeholder.
	canonical := bytes.ReplaceAll(caseData, []byte(sessionID), []byte("dogfood-session-00000000"))

	if err := os.WriteFile(outPath, canonical, 0o644); err != nil {
		return fmt.Errorf("write case: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote: %s\n", outPath)
	return nil
}

func createSession(c *http.Client, runtimeURL string) (string, error) {
	body := strings.NewReader(`{"mode":"capture"}`)
	req, err := http.NewRequest(http.MethodPost, runtimeURL+"/v1/sessions", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	addBearer(req)
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.SessionID, nil
}

func closeSession(c *http.Client, runtimeURL, sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, runtimeURL+"/v1/sessions/"+sessionID+"/close", nil)
	if err != nil {
		return err
	}
	addBearer(req)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("close status %d", resp.StatusCode)
	}
	return nil
}

func fetchCase(c *http.Client, runtimeURL, sessionID string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, runtimeURL+"/v1/cases/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	addBearer(req)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cases status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func addBearer(req *http.Request) {
	if token := strings.TrimSpace(os.Getenv("SOFTPROBE_API_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
