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

	"softprobe-runtime/internal/runtimeapp"
)

const version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
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
	if err := fs.Parse(args); err != nil {
		return 2
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimRight(*runtimeURL, "/") + "/health")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: status %d\n", resp.StatusCode)
		return 1
	}

	var health struct {
		Status        string `json:"status"`
		SpecVersion   string `json:"specVersion"`
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: invalid body: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(
		stdout,
		"softprobe %s\nruntime healthy: %s\nspecVersion: %s\nschemaVersion: %s\n",
		version,
		health.Status,
		health.SpecVersion,
		health.SchemaVersion,
	)
	return 0
}

func runSessionStart(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session start", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	mode := fs.String("mode", "replay", "session mode")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
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
		_ = json.NewEncoder(stdout).Encode(map[string]any{
			"sessionId":       created.SessionID,
			"sessionRevision": created.SessionRevision,
			"schemaVersion":   runtimeapp.SchemaVersion,
			"specVersion":     runtimeapp.SpecVersion,
		})
		return 0
	}

	_, _ = fmt.Fprintf(
		stdout,
		"session created: %s\nexport SOFTPROBE_SESSION_ID=%s\nspec/schema: %s / %s\n",
		created.SessionID,
		created.SessionID,
		runtimeapp.SpecVersion,
		runtimeapp.SchemaVersion,
	)
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
	_, _ = fmt.Fprintln(w, "usage: softprobe [--version|version|doctor|inspect case|session start|session load-case]")
}

type caseDocument struct {
	CaseID string       `json:"caseId"`
	Suite  string       `json:"suite"`
	Mode   string       `json:"mode"`
	Traces []caseTrace  `json:"traces"`
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
