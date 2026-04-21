package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// runSuite dispatches `softprobe suite {run,validate,diff}`. The runner
// implements the surface documented in docs-site/reference/cli.md#suite.
// It is intentionally lean — each case is driven through the public
// control-API so the runner has zero privileged access beyond what any
// external CI driver could do on its own.
func runSuite(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: softprobe suite {run|validate|diff} ...")
		return exitInvalidArgs
	}
	switch args[0] {
	case "run":
		return runSuiteRun(args[1:], stdout, stderr)
	case "validate":
		return runSuiteValidate(args[1:], stdout, stderr)
	case "diff":
		return runSuiteDiff(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "suite: unknown subcommand %q\n", args[0])
		return exitInvalidArgs
	}
}

type suiteCaseResult struct {
	CaseID     string `json:"caseId"`
	Path       string `json:"path"`
	Status     string `json:"status"` // "passed", "failed", "skipped"
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

func runSuiteRun(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("suite run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	parallel := fs.Int("parallel", defaultSuiteParallelism(), "concurrent cases")
	hooks := fs.String("hooks", "", "hook files (currently accepted but ignored pending PD3.1c)")
	junitPath := fs.String("junit", "", "write JUnit XML to PATH")
	reportPath := fs.String("report", "", "write HTML report to PATH")
	filter := fs.String("filter", "", "run only cases whose path contains substring")
	failFast := fs.Bool("fail-fast", false, "stop on first failure")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	_ = hooks // accept but ignore until hook runtime lands

	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "suite run: expected one suite file path")
		return exitInvalidArgs
	}
	if *parallel < 1 {
		*parallel = 1
	}
	suitePath := fs.Arg(0)

	doc, errs := loadSuite(suitePath)
	if len(errs) > 0 {
		for _, e := range errs {
			_, _ = fmt.Fprintf(stderr, "suite run: %s\n", e)
		}
		return exitValidation
	}

	cases := expandSuiteCases(suitePath, doc, *filter)
	if len(cases) == 0 {
		_, _ = fmt.Fprintln(stderr, "suite run: no cases selected")
		if *jsonOutput {
			writeSuiteJSON(stdout, doc.Name, []suiteCaseResult{}, 0, 0)
		}
		return exitOK
	}

	results := runSuiteCases(*runtimeURL, cases, *parallel, *failFast, stderr)

	passed, failed := tally(results)
	if *junitPath != "" {
		if err := writeJUnit(*junitPath, doc.Name, results); err != nil {
			_, _ = fmt.Fprintf(stderr, "suite run: write junit: %v\n", err)
		}
	}
	if *reportPath != "" {
		if err := writeSuiteHTMLReport(*reportPath, doc.Name, results); err != nil {
			_, _ = fmt.Fprintf(stderr, "suite run: write report: %v\n", err)
		}
	}

	if *jsonOutput {
		writeSuiteJSON(stdout, doc.Name, results, passed, failed)
	} else {
		writeSuiteHuman(stdout, doc.Name, results, passed, failed)
	}

	if failed > 0 {
		return exitSuiteFail
	}
	return exitOK
}

func runSuiteValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("suite validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintln(stderr, "suite validate: expected one suite file path")
		return exitInvalidArgs
	}
	suitePath := fs.Arg(0)

	raw, err := os.ReadFile(suitePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "suite validate failed: %v\n", err)
		return exitGeneric
	}
	errs := validateSuitePayload(suitePath, raw)

	// Also verify every referenced case file exists so CI doesn't discover
	// typos only at run time. Hook references are accepted but not resolved
	// until PD3.1c lands.
	if len(errs) == 0 {
		doc, parseErrs := parseSuiteDocument(suitePath, raw)
		if len(parseErrs) > 0 {
			errs = append(errs, parseErrs...)
		} else {
			for i, entry := range doc.Cases {
				resolved := resolveSuiteCasePath(suitePath, entry.Path)
				matches, err := filepath.Glob(resolved)
				if err != nil {
					errs = append(errs, fmt.Sprintf("cases[%d]: invalid glob %q: %v", i, entry.Path, err))
					continue
				}
				if len(matches) == 0 && !strings.ContainsAny(entry.Path, "*?[") {
					// Only flag non-glob references as missing.
					if _, err := os.Stat(resolved); err != nil {
						errs = append(errs, fmt.Sprintf("cases[%d]: case file not found: %s", i, entry.Path))
					}
				}
			}
		}
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, statusFor(errs), exitCodeFor(errs), map[string]any{
			"suite":  suitePath,
			"errors": errOrEmpty(errs),
		})
	} else if len(errs) == 0 {
		_, _ = fmt.Fprintf(stdout, "suite: %s valid\n", suitePath)
	} else {
		_, _ = fmt.Fprintf(stderr, "suite: %s invalid\n", suitePath)
		for _, e := range errs {
			_, _ = fmt.Fprintf(stderr, "  - %s\n", e)
		}
	}

	if len(errs) > 0 {
		return exitValidation
	}
	return exitOK
}

// runSuiteDiff compares baseline vs current sets of case files by diffing
// their extracted outbound span signatures (method+host+path+status).
func runSuiteDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("suite diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	baseline := fs.String("baseline", "", "baseline case glob")
	current := fs.String("current", "", "current case glob")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	if *baseline == "" || *current == "" {
		_, _ = fmt.Fprintln(stderr, "suite diff requires --baseline and --current")
		return exitInvalidArgs
	}

	baseSigs, err := caseSignaturesForGlob(*baseline)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "suite diff: baseline: %v\n", err)
		return exitGeneric
	}
	curSigs, err := caseSignaturesForGlob(*current)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "suite diff: current: %v\n", err)
		return exitGeneric
	}

	added, removed := diffSignatureSets(baseSigs, curSigs)

	if *jsonOutput {
		writeJSONEnvelope(stdout, statusFor(nil), exitOK, map[string]any{
			"added":   added,
			"removed": removed,
		})
		return exitOK
	}

	if len(added) == 0 && len(removed) == 0 {
		_, _ = fmt.Fprintln(stdout, "suite diff: no drift detected")
		return exitOK
	}
	if len(added) > 0 {
		_, _ = fmt.Fprintln(stdout, "added (in current, not baseline):")
		for _, s := range added {
			_, _ = fmt.Fprintf(stdout, "  + %s\n", s)
		}
	}
	if len(removed) > 0 {
		_, _ = fmt.Fprintln(stdout, "removed (in baseline, not current):")
		for _, s := range removed {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", s)
		}
	}
	return exitOK
}

func loadSuite(suitePath string) (*suiteDocument, []string) {
	raw, err := os.ReadFile(suitePath)
	if err != nil {
		return nil, []string{fmt.Sprintf("read suite: %v", err)}
	}
	if errs := validateSuitePayload(suitePath, raw); len(errs) > 0 {
		return nil, errs
	}
	return parseSuiteDocument(suitePath, raw)
}

func expandSuiteCases(suitePath string, doc *suiteDocument, filter string) []resolvedCase {
	hasOnly := false
	for _, e := range doc.Cases {
		if e.Only {
			hasOnly = true
			break
		}
	}

	var out []resolvedCase
	for _, entry := range doc.Cases {
		if entry.Skip {
			continue
		}
		if hasOnly && !entry.Only {
			continue
		}
		globbed, _ := filepath.Glob(resolveSuiteCasePath(suitePath, entry.Path))
		if len(globbed) == 0 {
			globbed = []string{resolveSuiteCasePath(suitePath, entry.Path)}
		}
		sort.Strings(globbed)
		for _, match := range globbed {
			if filter != "" && !strings.Contains(match, filter) {
				continue
			}
			out = append(out, resolvedCase{
				DisplayName: entry.Name,
				Path:        match,
			})
		}
	}
	return out
}

type resolvedCase struct {
	DisplayName string
	Path        string
}

func resolveSuiteCasePath(suitePath, casePath string) string {
	if filepath.IsAbs(casePath) {
		return casePath
	}
	return filepath.Join(filepath.Dir(suitePath), casePath)
}

func runSuiteCases(runtimeURL string, cases []resolvedCase, parallel int, failFast bool, stderr io.Writer) []suiteCaseResult {
	results := make([]suiteCaseResult, len(cases))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var abort bool
	var abortMu sync.Mutex

	for i, c := range cases {
		abortMu.Lock()
		if abort {
			abortMu.Unlock()
			results[i] = suiteCaseResult{Path: c.Path, Status: "skipped"}
			continue
		}
		abortMu.Unlock()

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, rc resolvedCase) {
			defer wg.Done()
			defer func() { <-sem }()
			result := runSingleSuiteCase(runtimeURL, rc)
			results[idx] = result
			if failFast && result.Status == "failed" {
				abortMu.Lock()
				abort = true
				abortMu.Unlock()
			}
		}(i, c)
	}
	wg.Wait()
	return results
}

func runSingleSuiteCase(runtimeURL string, rc resolvedCase) suiteCaseResult {
	start := time.Now()
	result := suiteCaseResult{Path: rc.Path}

	caseBytes, err := os.ReadFile(rc.Path)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("read case: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	var doc caseDocument
	_ = json.Unmarshal(caseBytes, &doc)
	result.CaseID = doc.CaseID
	if result.CaseID == "" {
		result.CaseID = filepath.Base(rc.Path)
	}

	client := newHTTPClient(30 * time.Second)
	sessionID, code := suiteStartSession(client, runtimeURL, "replay")
	if code != exitOK {
		result.Status = "failed"
		result.Error = "runtime unavailable"
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	defer func() {
		_ = suiteCloseSession(client, runtimeURL, sessionID)
	}()

	if code := suitePostBytes(client, runtimeURL, sessionID, "load-case", caseBytes); code != exitOK {
		result.Status = "failed"
		result.Error = "load-case failed"
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	stats, code := fetchSessionStatsQuiet(runtimeURL, sessionID)
	if code != exitOK {
		result.Status = "failed"
		result.Error = "stats fetch failed"
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	_ = stats

	result.Status = "passed"
	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

func suiteStartSession(client *http.Client, runtimeURL, mode string) (string, int) {
	body := bytes.NewBufferString(fmt.Sprintf(`{"mode":%q}`, mode))
	req, err := newRuntimeRequest(http.MethodPost, strings.TrimRight(runtimeURL, "/")+"/v1/sessions", body)
	if err != nil {
		return "", exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", classifyTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", exitGeneric
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", exitGeneric
	}
	return created.SessionID, exitOK
}

func suitePostBytes(client *http.Client, runtimeURL, sessionID, suffix string, body []byte) int {
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(runtimeURL, "/")+"/v1/sessions/"+sessionID+"/"+suffix,
		bytes.NewReader(body),
	)
	if err != nil {
		return exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return classifyTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return classifyHTTPError(resp.StatusCode, b)
	}
	return exitOK
}

func suiteCloseSession(client *http.Client, runtimeURL, sessionID string) int {
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(runtimeURL, "/")+"/v1/sessions/"+sessionID+"/close",
		nil,
	)
	if err != nil {
		return exitGeneric
	}
	resp, err := client.Do(req)
	if err != nil {
		return classifyTransportError(err)
	}
	resp.Body.Close()
	return exitOK
}

func fetchSessionStatsQuiet(runtimeURL, sessionID string) (sessionStatsPayload, int) {
	return fetchSessionStats(runtimeURL, sessionID, io.Discard)
}

func tally(results []suiteCaseResult) (passed, failed int) {
	for _, r := range results {
		switch r.Status {
		case "passed":
			passed++
		case "failed":
			failed++
		}
	}
	return
}

func writeSuiteJSON(w io.Writer, suiteName string, results []suiteCaseResult, passed, failed int) {
	writeJSONEnvelope(w, statusForSuite(failed), exitCodeForSuite(failed), map[string]any{
		"suite":  suiteName,
		"total":  len(results),
		"passed": passed,
		"failed": failed,
		"cases":  results,
	})
}

func writeSuiteHuman(w io.Writer, suiteName string, results []suiteCaseResult, passed, failed int) {
	_, _ = fmt.Fprintf(w, "suite: %s\n", suiteName)
	for _, r := range results {
		status := r.Status
		switch r.Status {
		case "passed":
			status = "OK"
		case "failed":
			status = "FAIL"
		case "skipped":
			status = "SKIP"
		}
		if r.Error != "" {
			_, _ = fmt.Fprintf(w, "  %s %s (%dms): %s\n", status, r.Path, r.DurationMs, r.Error)
		} else {
			_, _ = fmt.Fprintf(w, "  %s %s (%dms)\n", status, r.Path, r.DurationMs)
		}
	}
	_, _ = fmt.Fprintf(w, "result: passed=%d failed=%d total=%d\n", passed, failed, len(results))
}

func statusForSuite(failed int) string {
	if failed == 0 {
		return "ok"
	}
	return "fail"
}

func exitCodeForSuite(failed int) int {
	if failed == 0 {
		return exitOK
	}
	return exitSuiteFail
}

func defaultSuiteParallelism() int {
	n := runtime.NumCPU() * 4
	if n < 1 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	return n
}

// writeJUnit emits a minimal JUnit XML report. The schema matches the
// widely-used Ant/Jenkins dialect — one testsuite with one testcase per
// case file. Failures include the error message as <failure type="…"/>
// content so CI tooling can render them inline.
func writeJUnit(path, suiteName string, results []suiteCaseResult) error {
	type failure struct {
		Message string `xml:"message,attr,omitempty"`
		Type    string `xml:"type,attr,omitempty"`
		Body    string `xml:",chardata"`
	}
	type testcase struct {
		Name      string   `xml:"name,attr"`
		Classname string   `xml:"classname,attr"`
		Time      string   `xml:"time,attr"`
		Failure   *failure `xml:"failure,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Cases    []testcase `xml:"testcase"`
	}

	ts := testsuite{Name: suiteName, Tests: len(results)}
	for _, r := range results {
		tc := testcase{Name: r.CaseID, Classname: r.Path, Time: fmt.Sprintf("%.3f", float64(r.DurationMs)/1000.0)}
		if r.Status == "failed" {
			ts.Failures++
			tc.Failure = &failure{Message: r.Error, Type: "case_failed", Body: r.Error}
		}
		ts.Cases = append(ts.Cases, tc)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(buf)
	enc.Indent("", "  ")
	if err := enc.Encode(ts); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// writeSuiteHTMLReport renders a self-contained HTML table with each case's
// outcome. The layout deliberately avoids JS so the report opens offline.
func writeSuiteHTMLReport(path, suiteName string, results []suiteCaseResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	passed, failed := tally(results)
	buf := &bytes.Buffer{}
	_, _ = fmt.Fprintf(buf, `<!doctype html><html><head><meta charset="utf-8"><title>%s — suite report</title><style>body{font-family:system-ui,sans-serif;margin:2rem;}table{border-collapse:collapse;width:100%%;}th,td{border:1px solid #ddd;padding:.4rem .6rem;}tr.failed td{background:#fee;}tr.passed td{background:#efe;}</style></head><body>`, htmlEscape(suiteName))
	_, _ = fmt.Fprintf(buf, `<h1>%s</h1><p>total=%d passed=%d failed=%d</p>`, htmlEscape(suiteName), len(results), passed, failed)
	buf.WriteString(`<table><tr><th>Status</th><th>Case</th><th>Path</th><th>ms</th><th>Error</th></tr>`)
	for _, r := range results {
		_, _ = fmt.Fprintf(buf, `<tr class="%s"><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>`,
			htmlEscape(r.Status), htmlEscape(r.Status), htmlEscape(r.CaseID), htmlEscape(r.Path), r.DurationMs, htmlEscape(r.Error))
	}
	buf.WriteString(`</table></body></html>`)
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;")
	return r.Replace(s)
}

type spanSignature struct {
	Method string `json:"method"`
	Host   string `json:"host"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

func (s spanSignature) String() string {
	return fmt.Sprintf("%s %s%s -> %s", s.Method, s.Host, s.Path, s.Status)
}

func caseSignaturesForGlob(glob string) (map[string]spanSignature, error) {
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		// Allow literal single-file paths so callers can pass one case.
		if _, err := os.Stat(glob); err == nil {
			matches = []string{glob}
		}
	}
	sigs := make(map[string]spanSignature)
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		var doc caseDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue
		}
		for _, trace := range doc.Traces {
			for _, rs := range trace.ResourceSpans {
				for _, ss := range rs.ScopeSpans {
					for _, sp := range ss.Spans {
						attrs := indexAttributes(sp.Attributes)
						sig := spanSignature{
							Method: attrs["http.request.method"],
							Host:   attrs["url.host"],
							Path:   attrs["url.path"],
							Status: attrs["http.response.status_code"],
						}
						sigs[sig.String()] = sig
					}
				}
			}
		}
	}
	return sigs, nil
}

func diffSignatureSets(baseline, current map[string]spanSignature) (added, removed []string) {
	for k := range current {
		if _, ok := baseline[k]; !ok {
			added = append(added, k)
		}
	}
	for k := range baseline {
		if _, ok := current[k]; !ok {
			removed = append(removed, k)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return
}
