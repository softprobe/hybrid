package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// runValidate dispatches `softprobe validate {case,rules,suite}` FILE. All
// three share a common control flow: read file, normalize YAML→JSON, run
// shape validation, print a human-readable or --json report, exit 5 on any
// schema/validation error per docs-site/reference/cli.md.
func runValidate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: softprobe validate {case|rules|suite} FILE [--json]")
		return exitInvalidArgs
	}
	kind := args[0]
	switch kind {
	case "case", "rules", "suite":
	default:
		_, _ = fmt.Fprintf(stderr, "validate: unknown kind %q (want case, rules, or suite)\n", kind)
		return exitInvalidArgs
	}

	fs := flag.NewFlagSet("validate "+kind, flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args[1:]); err != nil {
		return exitInvalidArgs
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "validate %s: expected one file path\n", kind)
		return exitInvalidArgs
	}
	path := fs.Arg(0)

	raw, err := os.ReadFile(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "validate %s failed: %v\n", kind, err)
		return exitGeneric
	}

	errs := validatePayload(kind, path, raw)

	if *jsonOutput {
		writeValidateJSON(stdout, path, errs)
	} else {
		writeValidateHuman(stdout, stderr, kind, path, errs)
	}

	if len(errs) > 0 {
		return exitValidation
	}
	return exitOK
}

// validatePayload runs the right kind-specific checker. Returns a list of
// error strings (one per violation). An empty list means valid.
func validatePayload(kind, path string, raw []byte) []string {
	switch kind {
	case "case":
		return validateCasePayload(raw)
	case "rules":
		return validateRulesPayload(path, raw)
	case "suite":
		return validateSuitePayload(path, raw)
	}
	return []string{fmt.Sprintf("unsupported kind %q", kind)}
}

func validateCasePayload(raw []byte) []string {
	var doc caseDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return []string{fmt.Sprintf("invalid json: %v", err)}
	}
	var errs []string
	if doc.Version == "" {
		errs = append(errs, "missing required field: version")
	}
	if doc.CaseID == "" {
		errs = append(errs, "missing required field: caseId")
	}
	if doc.Traces == nil {
		errs = append(errs, "missing required field: traces")
	}
	for i, trace := range doc.Traces {
		if len(trace.ResourceSpans) == 0 {
			errs = append(errs, fmt.Sprintf("traces[%d]: missing resourceSpans", i))
		}
	}
	return errs
}

func validateRulesPayload(path string, raw []byte) []string {
	normalized, err := normalizeRulesPayload(path, raw)
	if err != nil {
		return []string{fmt.Sprintf("invalid document: %v", err)}
	}
	// The runtime accepts either a bare array or `{"rules":[…]}`. Accept both.
	var wrapped struct {
		Rules []map[string]any `json:"rules"`
	}
	if err := json.Unmarshal(normalized, &wrapped); err == nil && wrapped.Rules != nil {
		return validateRuleObjects(wrapped.Rules)
	}
	var bare []map[string]any
	if err := json.Unmarshal(normalized, &bare); err == nil {
		return validateRuleObjects(bare)
	}
	return []string{"rules must be a list or {rules:[…]} object"}
}

func validateRuleObjects(rules []map[string]any) []string {
	var errs []string
	if len(rules) == 0 {
		errs = append(errs, "rules: at least one rule required")
	}
	for i, rule := range rules {
		if _, ok := rule["match"]; !ok {
			errs = append(errs, fmt.Sprintf("rules[%d]: missing required field: match", i))
		}
		if _, ok := rule["response"]; !ok {
			// The proxy also accepts {action:"…"} shortcuts; only flag when
			// neither is present.
			if _, hasAction := rule["action"]; !hasAction {
				errs = append(errs, fmt.Sprintf("rules[%d]: missing response or action", i))
			}
		}
	}
	return errs
}

func validateSuitePayload(path string, raw []byte) []string {
	doc, parseErrs := parseSuiteDocument(path, raw)
	if len(parseErrs) > 0 {
		return parseErrs
	}
	var errs []string
	if doc.Name == "" {
		errs = append(errs, "missing required field: name")
	}
	if len(doc.Cases) == 0 {
		errs = append(errs, "suite must declare at least one case entry")
	}
	for i, entry := range doc.Cases {
		if strings.TrimSpace(entry.Path) == "" {
			errs = append(errs, fmt.Sprintf("cases[%d]: missing path", i))
		}
	}
	return errs
}

func writeValidateJSON(w io.Writer, path string, errs []string) {
	writeJSONEnvelope(w, statusFor(errs), exitCodeFor(errs), map[string]any{
		"path":   path,
		"valid":  len(errs) == 0,
		"errors": errOrEmpty(errs),
	})
}

func writeValidateHuman(stdout, stderr io.Writer, kind, path string, errs []string) {
	if len(errs) == 0 {
		_, _ = fmt.Fprintf(stdout, "%s: %s valid\n", kind, path)
		return
	}
	_, _ = fmt.Fprintf(stderr, "%s: %s invalid\n", kind, path)
	for _, e := range errs {
		_, _ = fmt.Fprintf(stderr, "  - %s\n", e)
	}
}

func statusFor(errs []string) string {
	if len(errs) == 0 {
		return "ok"
	}
	return "fail"
}

func exitCodeFor(errs []string) int {
	if len(errs) == 0 {
		return exitOK
	}
	return exitValidation
}

func errOrEmpty(errs []string) []string {
	if errs == nil {
		return []string{}
	}
	return errs
}
