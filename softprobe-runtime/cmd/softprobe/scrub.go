package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// runScrub applies redaction rules to a case file (or glob) in place. When
// no --rules file is supplied we apply a conservative default set that
// blanks out common PII/credential patterns (email, bearer tokens, basic
// card number shapes). Callers who want stricter redaction pass a rules
// file; otherwise the default is intentionally low-risk.
func runScrub(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scrub", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rulesPath := fs.String("rules", "", "redaction rules file (YAML/JSON)")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(stderr, "scrub requires one or more FILE arguments (or a glob)")
		return exitInvalidArgs
	}

	rules, err := loadScrubRules(*rulesPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "scrub failed: %v\n", err)
		return exitValidation
	}

	type scrubResult struct {
		Path        string `json:"path"`
		Replaced    int    `json:"replaced"`
		Error       string `json:"error,omitempty"`
		UpdatedAtMS int64  `json:"updatedAtMs,omitempty"`
	}
	var results []scrubResult

	var targets []string
	for _, arg := range fs.Args() {
		globbed, _ := filepath.Glob(arg)
		if len(globbed) == 0 {
			targets = append(targets, arg)
			continue
		}
		targets = append(targets, globbed...)
	}

	for _, path := range targets {
		raw, err := os.ReadFile(path)
		if err != nil {
			results = append(results, scrubResult{Path: path, Error: err.Error()})
			continue
		}
		scrubbed, n := applyScrubRules(raw, rules)
		if n == 0 {
			results = append(results, scrubResult{Path: path, Replaced: 0})
			continue
		}
		if err := os.WriteFile(path, scrubbed, 0o644); err != nil {
			results = append(results, scrubResult{Path: path, Error: err.Error()})
			continue
		}
		results = append(results, scrubResult{Path: path, Replaced: n, UpdatedAtMS: time.Now().UnixMilli()})
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"files": results,
		})
	} else {
		for _, r := range results {
			if r.Error != "" {
				_, _ = fmt.Fprintf(stderr, "%s: error: %s\n", r.Path, r.Error)
			} else {
				_, _ = fmt.Fprintf(stdout, "%s: %d replacements\n", r.Path, r.Replaced)
			}
		}
	}
	return exitOK
}

type scrubRule struct {
	Pattern     *regexp.Regexp
	Replacement string
	RawPattern  string `json:"pattern"`
	Replace     string `json:"replace"`
}

func loadScrubRules(path string) ([]scrubRule, error) {
	if path == "" {
		return defaultScrubRules(), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	normalized, err := yamlToJSON(raw)
	if err != nil {
		return nil, err
	}
	var wrapped struct {
		Rules []scrubRule `json:"rules"`
	}
	if err := json.Unmarshal(normalized, &wrapped); err != nil || wrapped.Rules == nil {
		// Accept bare array form too.
		var bare []scrubRule
		if err := json.Unmarshal(normalized, &bare); err != nil {
			return nil, fmt.Errorf("parse rules: %w", err)
		}
		wrapped.Rules = bare
	}
	for i := range wrapped.Rules {
		re, err := regexp.Compile(wrapped.Rules[i].RawPattern)
		if err != nil {
			return nil, fmt.Errorf("rules[%d]: invalid pattern: %w", i, err)
		}
		wrapped.Rules[i].Pattern = re
		wrapped.Rules[i].Replacement = wrapped.Rules[i].Replace
	}
	return wrapped.Rules, nil
}

func defaultScrubRules() []scrubRule {
	patterns := []struct {
		pattern string
		replace string
	}{
		{`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`, "REDACTED_EMAIL"},
		{`Bearer\s+[A-Za-z0-9\-._~+/]+=*`, "Bearer REDACTED"},
		{`\b(?:\d[ -]*?){13,19}\b`, "REDACTED_CARD"},
	}
	rules := make([]scrubRule, 0, len(patterns))
	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		rules = append(rules, scrubRule{Pattern: re, Replacement: p.replace, RawPattern: p.pattern, Replace: p.replace})
	}
	return rules
}

func applyScrubRules(raw []byte, rules []scrubRule) ([]byte, int) {
	text := string(raw)
	total := 0
	for _, rule := range rules {
		matches := rule.Pattern.FindAllStringIndex(text, -1)
		total += len(matches)
		text = rule.Pattern.ReplaceAllString(text, rule.Replacement)
	}
	// Preserve trailing newline if present.
	if strings.HasSuffix(string(raw), "\n") && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text), total
}
