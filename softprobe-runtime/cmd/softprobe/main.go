package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"softprobe-runtime/internal/controlapi"
	"softprobe-runtime/internal/version"
)

// Documented exit codes from docs-site/reference/cli.md#exit-codes. Changing
// these values is a breaking change to the CLI contract; CI pipelines and
// agents dispatch on them directly.
const (
	exitOK                 = 0
	exitGeneric            = 1
	exitInvalidArgs        = 2
	exitRuntimeUnreachable = 3
	exitSessionNotFound    = 4
	exitValidation         = 5
	exitDoctorFail         = 10
	exitSuiteFail          = 20
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	stdout = safeWriter(stdout)
	stderr = safeWriter(stderr)
	rawStdout := stdout

	// Pull global flags out before the subcommand dispatch so every command
	// shares identical --verbose / --quiet / --no-color semantics. This
	// mirrors docs-site/reference/cli.md#global-options, which documents
	// these as repo-wide contract regardless of which subcommand consumes
	// them.
	args, gs := extractGlobalFlags(args)
	globals = gs
	if gs.verbose {
		verboseStderr = stderr
	} else {
		verboseStderr = nil
	}
	if gs.help && len(args) == 0 {
		printUsage(rawStdout)
		return exitOK
	}

	if len(args) == 0 {
		printUsage(stderr)
		return exitInvalidArgs
	}

	// Route stdout to a throwaway sink under --quiet so subcommands can keep
	// calling fmt.Fprintf(stdout, …) naïvely. --help/--version bypass this.
	if gs.quiet {
		stdout = io.Discard
	}

	switch args[0] {
	case "--version", "version":
		_, _ = fmt.Fprintf(rawStdout, "softprobe %s\n", version.CLIDetail(controlapi.SpecVersion))
		return exitOK
	case "--help", "-h", "help":
		printUsage(rawStdout)
		return exitOK
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "generate":
		return runGenerate(args[1:], stdout, stderr)
	case "session":
		return runSession(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "replay":
		return runReplay(args[1:], stdout, stderr)
	case "suite":
		return runSuite(args[1:], stdout, stderr)
	case "capture":
		return runCapture(args[1:], stdout, stderr)
	case "scrub":
		return runScrub(args[1:], stdout, stderr)
	case "export":
		return runExport(args[1:], stdout, stderr)
	case "completion":
		return runCompletion(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return exitInvalidArgs
	}
}

func runSession(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitInvalidArgs
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
		return exitInvalidArgs
	}
}

func runSessionRules(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitInvalidArgs
	}

	switch args[0] {
	case "apply":
		return runSessionRulesApply(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return exitInvalidArgs
	}
}

func runSessionPolicy(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitInvalidArgs
	}

	switch args[0] {
	case "set":
		return runSessionPolicySet(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return exitInvalidArgs
	}
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitInvalidArgs
	}

	switch args[0] {
	case "case":
		return runInspectCase(args[1:], stdout, stderr)
	case "session":
		return runInspectSession(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return exitInvalidArgs
	}
}

func runInspectCase(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect case", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "inspect case requires a case file path")
		return exitInvalidArgs
	}

	casePath := fs.Arg(0)
	caseBytes, err := os.ReadFile(casePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "inspect case failed: %v\n", err)
		return exitGeneric
	}

	var doc caseDocument
	if err := json.Unmarshal(caseBytes, &doc); err != nil {
		_, _ = fmt.Fprintf(stderr, "inspect case failed: invalid case: %v\n", err)
		return exitValidation
	}

	if *jsonOutput {
		spanSummary := make([]map[string]any, 0)
		for _, trace := range doc.Traces {
			for _, r := range trace.ResourceSpans {
				for _, s := range r.ScopeSpans {
					for _, sp := range s.Spans {
						attrs := indexAttributes(sp.Attributes)
						spanSummary = append(spanSummary, map[string]any{
							"direction": attrs["sp.traffic.direction"],
							"method":    attrs["http.request.method"],
							"host":      attrs["url.host"],
							"path":      attrs["url.path"],
							"status":    attrs["http.response.status_code"],
						})
					}
				}
			}
		}
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"caseId":      doc.CaseID,
			"version":     doc.Version,
			"traceCount":  len(doc.Traces),
			"spanSummary": spanSummary,
		})
		return exitOK
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
	return exitOK
}

func indexAttributes(attrs []attribute) map[string]string {
	out := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.StringValue
	}
	return out
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	client := newHTTPClient(5 * time.Second)
	healthReq, err := newRuntimeRequest(http.MethodGet, strings.TrimRight(*runtimeURL, "/")+"/health", nil)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: %v\n", err)
		return exitDoctorFail
	}
	resp, err := client.Do(healthReq)
	if err != nil {
		if *jsonOutput {
			writeJSONError(stdout, exitDoctorFail, "runtime_unreachable", fmt.Sprintf("runtime health check failed: %v", err), nil)
			return exitDoctorFail
		}
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: %v\n", err)
		return exitDoctorFail
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if *jsonOutput {
			writeJSONError(stdout, exitDoctorFail, "runtime_unhealthy", fmt.Sprintf("runtime health check failed: status %d", resp.StatusCode), nil)
			return exitDoctorFail
		}
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: status %d\n", resp.StatusCode)
		return exitDoctorFail
	}

	var health struct {
		Status        string `json:"status"`
		SpecVersion   string `json:"specVersion"`
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		if *jsonOutput {
			writeJSONError(stdout, exitDoctorFail, "invalid_health_response", fmt.Sprintf("runtime health check failed: invalid body: %v", err), nil)
			return exitDoctorFail
		}
		_, _ = fmt.Fprintf(stderr, "runtime health check failed: invalid body: %v\n", err)
		return exitDoctorFail
	}

	metaReq, err := newRuntimeRequest(http.MethodGet, strings.TrimRight(*runtimeURL, "/")+"/v1/meta", nil)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "runtime metadata check failed: %v\n", err)
		return exitDoctorFail
	}
	metaResp, err := client.Do(metaReq)
	if err != nil {
		if *jsonOutput {
			writeJSONError(stdout, exitDoctorFail, "runtime_meta_unreachable", fmt.Sprintf("runtime metadata check failed: %v", err), nil)
			return exitDoctorFail
		}
		_, _ = fmt.Fprintf(stderr, "runtime metadata check failed: %v\n", err)
		return exitDoctorFail
	}
	defer metaResp.Body.Close()

	if metaResp.StatusCode != http.StatusOK {
		if *jsonOutput {
			writeJSONError(stdout, exitDoctorFail, "runtime_meta_unhealthy", fmt.Sprintf("runtime metadata check failed: status %d", metaResp.StatusCode), nil)
			return exitDoctorFail
		}
		_, _ = fmt.Fprintf(stderr, "runtime metadata check failed: status %d\n", metaResp.StatusCode)
		return exitDoctorFail
	}

	var meta struct {
		RuntimeVersion string `json:"runtimeVersion"`
		SpecVersion    string `json:"specVersion"`
		SchemaVersion  string `json:"schemaVersion"`
	}
	if err := json.NewDecoder(metaResp.Body).Decode(&meta); err != nil {
		if *jsonOutput {
			writeJSONError(stdout, exitDoctorFail, "invalid_meta_response", fmt.Sprintf("runtime metadata check failed: invalid body: %v", err), nil)
			return exitDoctorFail
		}
		_, _ = fmt.Fprintf(stderr, "runtime metadata check failed: invalid body: %v\n", err)
		return exitDoctorFail
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
			writeJSONError(stdout, exitDoctorFail, "runtime_drift", "runtime spec/schema drift detected", fields)
			return exitDoctorFail
		}
		_, _ = fmt.Fprintf(
			stderr,
			"runtime drift detected: specVersion=%s schemaVersion=%s (want %s / %s)\n",
			meta.SpecVersion,
			meta.SchemaVersion,
			controlapi.SpecVersion,
			controlapi.SchemaVersion,
		)
		return exitDoctorFail
	}

	// The fatal checks above have all passed. The remaining checks (WASM
	// binary, proxy header echo) are intentionally non-fatal: a missing
	// sidecar binary or a lack of local proxy doesn't prevent the CLI from
	// driving the runtime, but it is useful diagnostic context.
	wasmCheck := runWASMBinaryCheck()
	echoCheck := runHeaderEchoCheck()
	checks := []doctorCheckResult{
		{Name: "runtime-reachable", Status: "ok", Details: map[string]any{"url": *runtimeURL}},
		{Name: "version-drift", Status: "ok", Details: map[string]any{"cli": version.SemverTag(), "runtime": meta.RuntimeVersion}},
		{Name: "schema-version", Status: "ok", Details: map[string]any{"expected": controlapi.SchemaVersion, "got": meta.SchemaVersion}},
		wasmCheck,
		echoCheck,
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", 0, map[string]any{
			"cliVersion":     version.CLIDetail(controlapi.SpecVersion),
			"runtimeHealthy": health.Status == "ok",
			"runtimeVersion": meta.RuntimeVersion,
			"specVersion":    meta.SpecVersion,
			"schemaVersion":  meta.SchemaVersion,
			"checks":         checks,
		})
		return 0
	}

	_, _ = fmt.Fprintf(
		stdout,
		"softprobe %s\nruntime healthy: %s\nruntimeVersion: %s\nspecVersion: %s\nschemaVersion: %s\n",
		version.CLIDetail(controlapi.SpecVersion),
		health.Status,
		meta.RuntimeVersion,
		meta.SpecVersion,
		meta.SchemaVersion,
	)
	for _, c := range checks {
		marker := "✓"
		switch c.Status {
		case "warn":
			marker = "⚠"
		case "fail":
			marker = "✗"
		case "skip":
			marker = "·"
		}
		msg := c.Name
		if m, ok := c.Details["message"].(string); ok && m != "" {
			msg = c.Name + ": " + m
		}
		_, _ = fmt.Fprintf(stdout, "%s %s\n", marker, msg)
	}
	return 0
}

func runSessionStart(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session start", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	mode := fs.String("mode", "replay", "session mode")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	shellOutput := fs.Bool("shell", false, "emit only a shell export line")
	policyFile := fs.String("policy", "", "policy YAML/JSON file to apply atomically after start")
	caseFile := fs.String("case", "", "case file to load atomically after start")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	client := newHTTPClient(5 * time.Second)
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions",
		bytes.NewBufferString(fmt.Sprintf(`{"mode":%q}`, *mode)),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session start failed: %v\n", err)
		return exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session start failed: %v\n", err)
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session start failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(resp.StatusCode, body)
	}

	var created struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		_, _ = fmt.Fprintf(stderr, "session start failed: invalid body: %v\n", err)
		return exitGeneric
	}

	// Optional chain: --policy FILE and --case FILE are equivalent to running
	// session start → session policy set --file → session load-case in one
	// go, which is what cli.md promises for `session start --policy --case`.
	finalRevision := created.SessionRevision
	if code, rev := maybeApplyPolicyFromFile(client, *runtimeURL, created.SessionID, *policyFile, stderr); code != exitOK {
		return code
	} else {
		finalRevision = rev
	}
	if code, rev := maybeLoadCaseFromFile(client, *runtimeURL, created.SessionID, *caseFile, stderr); code != exitOK {
		return code
	} else if rev != 0 {
		finalRevision = rev
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"sessionId":       created.SessionID,
			"sessionRevision": finalRevision,
			"mode":            *mode,
			"schemaVersion":   controlapi.SchemaVersion,
			"specVersion":     controlapi.SpecVersion,
		})
		return exitOK
	}
	if *shellOutput {
		_, _ = fmt.Fprintf(stdout, "export SOFTPROBE_SESSION_ID=%s\n", created.SessionID)
		return exitOK
	}

	_, _ = fmt.Fprintf(
		stdout,
		"session created: %s\nspec/schema: %s / %s\n",
		created.SessionID,
		controlapi.SpecVersion,
		controlapi.SchemaVersion,
	)
	return exitOK
}

// maybeApplyPolicyFromFile posts `policyFile` (if non-empty) to /v1/sessions/{id}/policy,
// returning (exitCode, newRevision). A no-op returns (exitOK, 0).
func maybeApplyPolicyFromFile(client *http.Client, runtimeURL, sessionID, policyFile string, stderr io.Writer) (int, int) {
	if policyFile == "" {
		return exitOK, 0
	}
	raw, err := os.ReadFile(policyFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session start --policy failed: %v\n", err)
		return exitGeneric, 0
	}
	normalized, err := normalizePolicyPayload(policyFile, raw)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session start --policy failed: %v\n", err)
		return exitValidation, 0
	}
	code, rev := postForRevision(client, runtimeURL, sessionID, "policy", normalized, stderr, "session start --policy")
	return code, rev
}

// maybeLoadCaseFromFile mirrors the policy helper for /v1/sessions/{id}/load-case.
func maybeLoadCaseFromFile(client *http.Client, runtimeURL, sessionID, caseFile string, stderr io.Writer) (int, int) {
	if caseFile == "" {
		return exitOK, 0
	}
	raw, err := os.ReadFile(caseFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session start --case failed: %v\n", err)
		return exitGeneric, 0
	}
	code, rev := postForRevision(client, runtimeURL, sessionID, "load-case", raw, stderr, "session start --case")
	return code, rev
}

func postForRevision(client *http.Client, runtimeURL, sessionID, suffix string, body []byte, stderr io.Writer, label string) (int, int) {
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(runtimeURL, "/")+"/v1/sessions/"+sessionID+"/"+suffix,
		bytes.NewReader(body),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s failed: %v\n", label, err)
		return exitGeneric, 0
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s failed: %v\n", label, err)
		return classifyTransportError(err), 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "%s failed: status %d: %s\n", label, resp.StatusCode, strings.TrimSpace(string(respBody)))
		return classifyHTTPError(resp.StatusCode, respBody), 0
	}
	var decoded struct {
		SessionRevision int `json:"sessionRevision"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return exitOK, decoded.SessionRevision
}

func runSessionStats(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session stats", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "session stats requires --session")
		return exitInvalidArgs
	}

	client := newHTTPClient(5 * time.Second)
	req, err := newRuntimeRequest(
		http.MethodGet,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/stats",
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session stats failed: %v\n", err)
		return exitGeneric
	}

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session stats failed: %v\n", err)
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session stats failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(resp.StatusCode, body)
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
		return exitGeneric
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"sessionId":       stats.SessionID,
			"sessionRevision": stats.SessionRevision,
			"mode":            stats.Mode,
			"stats":           stats.Stats,
		})
		return exitOK
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
	outPath := fs.String("out", "", "override capture output path (capture sessions only)")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "session close requires --session")
		return exitInvalidArgs
	}

	client := newHTTPClient(5 * time.Second)
	closeURL := strings.TrimRight(*runtimeURL, "/") + "/v1/sessions/" + *sessionID + "/close"
	if trimmedOut := strings.TrimSpace(*outPath); trimmedOut != "" {
		q := url.Values{}
		q.Set("out", trimmedOut)
		closeURL += "?" + q.Encode()
	}
	req, err := newRuntimeRequest(
		http.MethodPost,
		closeURL,
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session close failed: %v\n", err)
		return exitGeneric
	}

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session close failed: %v\n", err)
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session close failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(resp.StatusCode, body)
	}

	var closed struct {
		SessionID   string `json:"sessionId"`
		Closed      bool   `json:"closed"`
		CapturePath string `json:"capturePath,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&closed); err != nil {
		_, _ = fmt.Fprintf(stderr, "session close failed: invalid body: %v\n", err)
		return exitGeneric
	}

	if *jsonOutput {
		payload := map[string]any{
			"sessionId": closed.SessionID,
			"closed":    closed.Closed,
		}
		if closed.CapturePath != "" {
			payload["capturePath"] = closed.CapturePath
		}
		writeJSONEnvelope(stdout, "ok", exitOK, payload)
		return exitOK
	}

	if closed.CapturePath != "" {
		_, _ = fmt.Fprintf(stdout, "session %s closed\ncapturePath: %s\n", closed.SessionID, closed.CapturePath)
	} else {
		_, _ = fmt.Fprintf(stdout, "session %s closed\n", closed.SessionID)
	}
	return exitOK
}

func runSessionLoadCase(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session load-case", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	filePath := fs.String("file", "", "case file path")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	if *sessionID == "" || *filePath == "" {
		_, _ = fmt.Fprintln(stderr, "session load-case requires --session and --file")
		return exitInvalidArgs
	}

	caseBytes, err := os.ReadFile(*filePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: %v\n", err)
		return exitGeneric
	}

	client := newHTTPClient(5 * time.Second)
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/load-case",
		bytes.NewReader(caseBytes),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: %v\n", err)
		return exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: %v\n", err)
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session load-case failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(resp.StatusCode, body)
	}

	var loaded struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loaded); err != nil {
		_, _ = fmt.Fprintf(stderr, "session load-case failed: invalid body: %v\n", err)
		return exitGeneric
	}

	caseID, traceCount := caseSummary(caseBytes)
	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"sessionId":       loaded.SessionID,
			"sessionRevision": loaded.SessionRevision,
			"caseId":          caseID,
			"traceCount":      traceCount,
		})
		return exitOK
	}

	_, _ = fmt.Fprintf(stdout, "session %s loaded case: revision %d\n", loaded.SessionID, loaded.SessionRevision)
	return exitOK
}

func runSessionRulesApply(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session rules apply", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	filePath := fs.String("file", "", "rules file path")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	if *sessionID == "" || *filePath == "" {
		_, _ = fmt.Fprintln(stderr, "session rules apply requires --session and --file")
		return exitInvalidArgs
	}

	rulesBytes, err := os.ReadFile(*filePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: %v\n", err)
		return exitGeneric
	}

	// The runtime accepts JSON; accept YAML at the CLI boundary so docs can
	// say "rules/stripe.yaml" without lying.
	rulesBytes, err = normalizeRulesPayload(*filePath, rulesBytes)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: %v\n", err)
		return exitValidation
	}

	client := newHTTPClient(5 * time.Second)
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/rules",
		bytes.NewReader(rulesBytes),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: %v\n", err)
		return exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: %v\n", err)
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session rules apply failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(resp.StatusCode, body)
	}

	var applied struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&applied)
	if applied.SessionID == "" {
		applied.SessionID = *sessionID
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"sessionId":       applied.SessionID,
			"sessionRevision": applied.SessionRevision,
			"ruleCount":       countRules(rulesBytes),
		})
		return exitOK
	}

	_, _ = fmt.Fprintf(stdout, "session %s rules applied\n", applied.SessionID)
	return exitOK
}

func runSessionPolicySet(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("session policy set", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	strict := fs.Bool("strict", false, "enable strict policy")
	filePath := fs.String("file", "", "policy file (YAML or JSON)")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}

	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "session policy set requires --session")
		return exitInvalidArgs
	}

	if *strict && *filePath != "" {
		_, _ = fmt.Fprintln(stderr, "session policy set: --strict and --file are mutually exclusive")
		return exitInvalidArgs
	}

	var policyBody []byte
	switch {
	case *strict:
		policyBody = []byte(`{"externalHttp":"strict","defaultOnMiss":"error"}`)
	case *filePath != "":
		raw, err := os.ReadFile(*filePath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "session policy set failed: %v\n", err)
			return exitGeneric
		}
		normalized, err := normalizePolicyPayload(*filePath, raw)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "session policy set failed: %v\n", err)
			return exitValidation
		}
		policyBody = normalized
	default:
		policyBody = []byte(`{"externalHttp":"allow"}`)
	}

	client := newHTTPClient(5 * time.Second)
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/policy",
		bytes.NewReader(policyBody),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session policy set failed: %v\n", err)
		return exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "session policy set failed: %v\n", err)
		return classifyTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "session policy set failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(resp.StatusCode, body)
	}

	var applied struct {
		SessionID       string `json:"sessionId"`
		SessionRevision int    `json:"sessionRevision"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&applied)
	if applied.SessionID == "" {
		applied.SessionID = *sessionID
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"sessionId":       applied.SessionID,
			"sessionRevision": applied.SessionRevision,
		})
		return exitOK
	}

	switch {
	case *strict:
		_, _ = fmt.Fprintf(stdout, "session %s policy set to strict\n", applied.SessionID)
	case *filePath != "":
		_, _ = fmt.Fprintf(stdout, "session %s policy set from %s\n", applied.SessionID, *filePath)
	default:
		_, _ = fmt.Fprintf(stdout, "session %s policy set to allow\n", applied.SessionID)
	}
	return exitOK
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: softprobe [--version|version|doctor|inspect case|generate jest-session|session start|session load-case|session policy set --file PATH|session close --out PATH|validate case|validate rules|validate suite|replay run|suite run|suite validate]")
}

// normalizeRulesPayload accepts JSON straight through, and translates
// `.yaml`/`.yml` rules files into canonical JSON for the runtime. Parse
// errors surface as schema/validation failures at the CLI.
func normalizeRulesPayload(filePath string, raw []byte) ([]byte, error) {
	if hasYAMLExtension(filePath) {
		return yamlToJSON(raw)
	}
	if !looksLikeJSON(raw) {
		// Be lenient: allow YAML content even if the extension lies.
		return yamlToJSON(raw)
	}
	return raw, nil
}

// normalizePolicyPayload mirrors normalizeRulesPayload for `session policy set --file`.
func normalizePolicyPayload(filePath string, raw []byte) ([]byte, error) {
	return normalizeRulesPayload(filePath, raw)
}

func hasYAMLExtension(filePath string) bool {
	lower := strings.ToLower(filePath)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func looksLikeJSON(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

// countRules tries both the wrapped {"rules":[…]} and bare-array shape.
func countRules(jsonBody []byte) int {
	var wrapped struct {
		Rules []json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(jsonBody, &wrapped); err == nil && wrapped.Rules != nil {
		return len(wrapped.Rules)
	}
	var bare []json.RawMessage
	if err := json.Unmarshal(jsonBody, &bare); err == nil {
		return len(bare)
	}
	return 0
}

func caseSummary(caseBytes []byte) (string, int) {
	var doc caseDocument
	if err := json.Unmarshal(caseBytes, &doc); err != nil {
		return "", 0
	}
	return doc.CaseID, len(doc.Traces)
}

func safeWriter(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

// classifyTransportError maps a failed http.Client.Do result to the
// documented CLI exit-code surface. Any error reaching the runtime is treated
// as "runtime unreachable" (exit 3) — there is no case where the Go HTTP
// client returns an error mid-response for our synchronous requests.
func classifyTransportError(err error) int {
	if err == nil {
		return exitOK
	}
	return exitRuntimeUnreachable
}

// classifyHTTPError maps a non-2xx control-API response body to the
// documented CLI exit-code surface. It recognises the standard runtime error
// envelope `{"error":{"code":"…"}}` from softprobe-runtime/internal/controlapi.
func classifyHTTPError(status int, body []byte) int {
	switch status {
	case http.StatusNotFound:
		if matchesErrorCode(body, "unknown_session") {
			return exitSessionNotFound
		}
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return exitValidation
	}
	return exitGeneric
}

func matchesErrorCode(body []byte, wantCode string) bool {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	return envelope.Error.Code == wantCode
}

// newRuntimeRequest builds an HTTP request for the control runtime and
// attaches the bearer token from SOFTPROBE_API_TOKEN when it is set, matching
// the contract enforced by softprobe-runtime/internal/controlapi.withOptionalBearerAuth.
func newRuntimeRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if token := strings.TrimSpace(os.Getenv("SOFTPROBE_API_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
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
	Version string      `json:"version"`
	CaseID  string      `json:"caseId"`
	Suite   string      `json:"suite"`
	Mode    string      `json:"mode"`
	Traces  []caseTrace `json:"traces"`
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
