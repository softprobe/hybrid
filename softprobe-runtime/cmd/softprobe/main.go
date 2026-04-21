package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"softprobe-runtime/internal/controlapi"
)

const version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	stdout = safeWriter(stdout)
	stderr = safeWriter(stderr)

	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "--version", "-v", "version":
		_, _ = fmt.Fprintf(stdout, "softprobe %s\n", version)
		return 0
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "generate":
		return runGenerate(args[1:], stdout, stderr)
	case "session":
		return runSession(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return 2
	}
}

func runSession(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "start":
		return runSessionStart(args[1:], stdout, stderr)
	case "stats":
		return runSessionStats(args[1:], stdout, stderr)
	case "close":
		return runSessionClose(args[1:], stdout, stderr)
	case "load-case":
		return runSessionLoadCase(args[1:], stdout, stderr)
	case "rules":
		return runSessionRules(args[1:], stdout, stderr)
	case "policy":
		return runSessionPolicy(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return 2
	}
}

func runSessionRules(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "apply":
		return runSessionRulesApply(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return 2
	}
}

func runSessionPolicy(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "set":
		return runSessionPolicySet(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return 2
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "case":
		return runInspectCase(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return 2
	}
}

func runInspectCase(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect case", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "inspect case requires a case file path")
		return 2
	}

	casePath := fs.Arg(0)
	caseBytes, err := os.ReadFile(casePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "inspect case failed: %v\n", err)
		return 1
	}

	var doc caseDocument
	if err := json.Unmarshal(caseBytes, &doc); err != nil {
		_, _ = fmt.Fprintf(stderr, "inspect case failed: invalid case: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "case %s\n", doc.CaseID)
	if doc.Suite != "" {
		_, _ = fmt.Fprintf(stdout, "suite: %s\n", doc.Suite)
	}
	if doc.Mode != "" {
		_, _ = fmt.Fprintf(stdout, "mode: %s\n", doc.Mode)
	}
	_, _ = fmt.Fprintf(stdout, "traces: %d\n", len(doc.Traces))
	for i, trace := range doc.Traces {
		spans, hosts, directions := summarizeTrace(trace)
		_, _ = fmt.Fprintf(
			stdout,
			"trace %d: spans=%d hosts=%s directions=%s\n",
			i+1,
			spans,
			joinOrDash(hosts),
			joinOrDash(directions),
		)
	}
	return 0
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimRight(*runtimeURL, "/") + "/health")
	if err != nil {
		if *jsonOutput {
			writeJSONError(stdout, 1, "runtime_unreachable", fmt.Sprintf("runtime health check failed: %v", err), nil)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if *jsonOutput {
			writeJSONError(stdout, 1, "runtime_unhealthy", fmt.Sprintf("runtime health check failed: status %d", resp.StatusCode), nil)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: status %d\n", resp.StatusCode)
		return 1
	}

	var health struct {
		Status        string `json:"status"`
		SpecVersion   string `json:"specVersion"`
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		if *jsonOutput {
			writeJSONError(stdout, 1, "invalid_health_response", fmt.Sprintf("runtime health check failed: invalid body: %v", err), nil)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: invalid body: %v\n", err)
		return 1
	}

	metaResp, err := client.Get(strings.TrimRight(*runtimeURL, "/") + "/v1/meta")
	if err != nil {
		if *jsonOutput {
			writeJSONError(stdout, 1, "runtime_meta_unreachable", fmt.Sprintf("runtime metadata check failed: %v", err), nil)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "runtime metadata check failed: %v\n", err)
		return 1
	}
	defer metaResp.Body.Close()

	if metaResp.StatusCode != http.StatusOK {
		if *jsonOutput {
			writeJSONError(stdout, 1, "runtime_meta_unhealthy", fmt.Sprintf("runtime metadata check failed: status %d", metaResp.StatusCode), nil)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "runtime metadata check failed: status %d\n", metaResp.StatusCode)
		return 1
	}

	var meta struct {
		RuntimeVersion string `json:"runtimeVersion"`
		SpecVersion    string `json:"specVersion"`
		SchemaVersion  string `json:"schemaVersion"`
	}
	if err := json.NewDecoder(metaResp.Body).Decode(&meta); err != nil {
		if *jsonOutput {
			writeJSONError(stdout, 1, "invalid_meta_response", fmt.Sprintf("runtime metadata check failed: invalid body: %v", err), nil)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "runtime metadata check failed: invalid body: %v\n", err)
		return 1
	}

	if meta.SpecVersion != controlapi.SpecVersion || meta.SchemaVersion != controlapi.SchemaVersion {
		fields := map[string]any{
			"runtimeHealthy":        health.Status == "ok",
			"runtimeVersion":        meta.RuntimeVersion,
			"specVersion":           meta.SpecVersion,
			"schemaVersion":         meta.SchemaVersion,
			"expectedSpecVersion":   controlapi.SpecVersion,
			"expectedSchemaVersion": controlapi.SchemaVersion,
		}
		if *jsonOutput {
			writeJSONError(stdout, 1, "runtime_drift", "runtime spec/schema drift detected", fields)
			return 1
		}
		_, _ = fmt.Fprintf(
			stderr,
			"runtime drift detected: specVersion=%s schemaVersion=%s (want %s / %s)\n",
			meta.SpecVersion,
			meta.SchemaVersion,
			controlapi.SpecVersion,
			controlapi.SchemaVersion,
		)
		return 1
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", 0, map[string]any{
			"runtimeHealthy": health.Status == "ok",
			"runtimeVersion": meta.RuntimeVersion,
			"specVersion":    meta.SpecVersion,
			"schemaVersion":  meta.SchemaVersion,
		})
		return 0
	}

	_, _ = fmt.Fprintf(
		stdout,
		"softprobe %s\nruntime healthy: %s\nruntimeVersion: %s\nspecVersion: %s\nschemaVersion: %s\n",
		version,
		health.Status,
		meta.RuntimeVersion,
		meta.SpecVersion,
		meta.SchemaVersion,
	)
	return 0
}

func runSessionStart(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session start", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	mode := fs.String("mode", "replay", "session mode")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	shellOutput := fs.Bool("shell", false, "emit only a shell export line")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions",
		bytes.NewBufferString(fmt.Sprintf(`{"mode":%q}`, *mode)),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session start failed: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session start failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(stderr, "session start failed: status %d\n", resp.StatusCode)
		return 1
	}

	var created struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		_, _ = fmt.Fprintf(stderr, "session start failed: invalid body: %v\n", err)
		return 1
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", 0, map[string]any{
			"sessionId":       created.SessionID,
			"sessionRevision": created.SessionRevision,
			"schemaVersion":   controlapi.SchemaVersion,
			"specVersion":     controlapi.SpecVersion,
		})
		return 0
	}
	if *shellOutput {
		_, _ = fmt.Fprintf(stdout, "export SOFTPROBE_SESSION_ID=%s\n", created.SessionID)
		return 0
	}

	_, _ = fmt.Fprintf(
		stdout,
		"session created: %s\nspec/schema: %s / %s\n",
		created.SessionID,
		controlapi.SpecVersion,
		controlapi.SchemaVersion,
	)
	return 0
}

func runSessionStats(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session stats", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "session stats requires --session")
		return 2
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodGet,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/stats",
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session stats failed: %v\n", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session stats failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session stats failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}

	var stats struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
		Mode            string `json:"mode"`
		Stats           struct {
			InjectedSpans  int `json:"injectedSpans"`
			ExtractedSpans int `json:"extractedSpans"`
			StrictMisses   int `json:"strictMisses"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		_, _ = fmt.Fprintf(stderr, "session stats failed: invalid body: %v\n", err)
		return 1
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", 0, map[string]any{
			"sessionId":       stats.SessionID,
			"sessionRevision": stats.SessionRevision,
			"mode":            stats.Mode,
			"stats":           stats.Stats,
		})
		return 0
	}

	_, _ = fmt.Fprintf(
		stdout,
		"session %s stats\nrevision: %d\nmode: %s\ninjectedSpans: %d\nextractedSpans: %d\nstrictMisses: %d\n",
		stats.SessionID,
		stats.SessionRevision,
		stats.Mode,
		stats.Stats.InjectedSpans,
		stats.Stats.ExtractedSpans,
		stats.Stats.StrictMisses,
	)
	return 0
}

func runSessionClose(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session close", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "session close requires --session")
		return 2
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/close",
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session close failed: %v\n", err)
		return 1
	}

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session close failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session close failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}

	var closed struct {
		SessionID string `json:"sessionId"`
		Closed    bool   `json:"closed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&closed); err != nil {
		_, _ = fmt.Fprintf(stderr, "session close failed: invalid body: %v\n", err)
		return 1
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", 0, map[string]any{
			"sessionId": closed.SessionID,
			"closed":    closed.Closed,
		})
		return 0
	}

	_, _ = fmt.Fprintf(stdout, "session %s closed\n", closed.SessionID)
	return 0
}

func runSessionLoadCase(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session load-case", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	filePath := fs.String("file", "", "case file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *sessionID == "" || *filePath == "" {
		_, _ = fmt.Fprintln(stderr, "session load-case requires --session and --file")
		return 2
	}

	caseBytes, err := os.ReadFile(*filePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: %v\n", err)
		return 1
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/load-case",
		bytes.NewReader(caseBytes),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session load-case failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}

	var loaded struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loaded); err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: invalid body: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "session %s loaded case: revision %d\n", loaded.SessionID, loaded.SessionRevision)
	return 0
}

func runSessionRulesApply(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session rules apply", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	filePath := fs.String("file", "", "rules file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *sessionID == "" || *filePath == "" {
		_, _ = fmt.Fprintln(stderr, "session rules apply requires --session and --file")
		return 2
	}

	rulesBytes, err := os.ReadFile(*filePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: %v\n", err)
		return 1
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/rules",
		bytes.NewReader(rulesBytes),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "session %s rules applied\n", *sessionID)
	return 0
}

func runSessionPolicySet(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session policy set", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	strict := fs.Bool("strict", false, "enable strict policy")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "session policy set requires --session")
		return 2
	}

	policyBody := `{"externalHttp":"allow"}`
	if *strict {
		policyBody = `{"externalHttp":"strict"}`
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/policy",
		bytes.NewBufferString(policyBody),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session policy set failed: %v\n", err)
		return 1
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session policy set failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session policy set failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return 1
	}

	if *strict {
		_, _ = fmt.Fprintf(stdout, "session %s policy set to strict\n", *sessionID)
	} else {
		_, _ = fmt.Fprintf(stdout, "session %s policy set to allow\n", *sessionID)
	}
	return 0
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: softprobe [--version|version|doctor|inspect case|generate jest-session|session start|session load-case]")
}

func safeWriter(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func writeJSONError(w io.Writer, exitCode int, code, message string, fields map[string]any) {
	payload := copyJSONFields(fields)
	payload["error"] = map[string]any{
		"code":    code,
		"message": message,
	}
	writeJSONEnvelope(w, "error", exitCode, payload)
}

func writeJSONEnvelope(w io.Writer, status string, exitCode int, fields map[string]any) {
	payload := copyJSONFields(fields)
	payload["status"] = status
	payload["exitCode"] = exitCode
	_ = json.NewEncoder(w).Encode(payload)
}

func copyJSONFields(fields map[string]any) map[string]any {
	payload := make(map[string]any, len(fields)+2)
	for key, value := range fields {
		payload[key] = value
	}
	return payload
}

type caseDocument struct {
	CaseID string      `json:"caseId"`
	Suite  string      `json:"suite"`
	Mode   string      `json:"mode"`
	Traces []caseTrace `json:"traces"`
}

type caseTrace struct {
	ResourceSpans []resourceSpan `json:"resourceSpans"`
}

type resourceSpan struct {
	ScopeSpans []scopeSpan `json:"scopeSpans"`
}

type scopeSpan struct {
	Spans []span `json:"spans"`
}

type span struct {
	Attributes []attribute `json:"attributes"`
}

type attribute struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}

func summarizeTrace(trace caseTrace) (int, []string, []string) {
	hosts := make([]string, 0, 1)
	directions := make([]string, 0, 1)
	spanCount := 0

	for _, resource := range trace.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, sp := range scope.Spans {
				spanCount++
				for _, attr := range sp.Attributes {
					switch attr.Key {
					case "url.host":
						hosts = appendUnique(hosts, attr.Value.StringValue)
					case "sp.traffic.direction":
						directions = appendUnique(directions, attr.Value.StringValue)
					}
				}
			}
		}
	}

	return spanCount, hosts, directions
}

func appendUnique(values []string, candidate string) []string {
	if candidate == "" {
		return values
	}
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}
