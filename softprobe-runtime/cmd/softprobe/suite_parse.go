package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// suiteDocument is the parsed shape of `*.suite.yaml` files.
// docs-site/reference/suite-yaml.md documents this surface; we keep the Go
// type conservative so we can evolve the schema in future versions without
// breaking CI for valid-today suites.
type suiteDocument struct {
	Name     string            `json:"name"`
	Version  int               `json:"version,omitempty"`
	Hooks    []string          `json:"hooks,omitempty"`
	Cases    []suiteCaseEntry  `json:"cases"`
	Env      map[string]string `json:"env,omitempty"`
	Defaults suiteDefaults     `json:"defaults,omitempty"`

	// Some suites declare `mocks:` / `request:` / `assertions:` at the
	// top level instead of nested under `defaults:`. That reads more
	// naturally for the single-case-per-suite smoke tests. We accept
	// both and fold the top-level forms into `Defaults` during parse.
	Request    *suiteRequest    `json:"request,omitempty"`
	Mocks      []suiteMock      `json:"mocks,omitempty"`
	Assertions *suiteAssertions `json:"assertions,omitempty"`
}

type suiteCaseEntry struct {
	Path      string         `json:"path"`
	Name      string         `json:"name,omitempty"`
	Only      bool           `json:"only,omitempty"`
	Skip      bool           `json:"skip,omitempty"`
	Overrides *suiteDefaults `json:"overrides,omitempty"`
}

// suiteDefaults mirrors `defaults:` in suite YAML. All fields are optional.
// Per-case `overrides` reuse the same shape and are shallow-merged into
// defaults by `mergedDefaults`.
type suiteDefaults struct {
	Request    *suiteRequest    `json:"request,omitempty"`
	Mocks      []suiteMock      `json:"mocks,omitempty"`
	Assertions *suiteAssertions `json:"assertions,omitempty"`
	Policy     json.RawMessage  `json:"policy,omitempty"`
}

// suiteRequest describes how the runner builds the HTTP request sent to
// the SUT. Today we support `source: case.ingress` (which requires an
// inbound span in the case) and the inline form (fields set directly).
type suiteRequest struct {
	Source    string            `json:"source,omitempty"`
	URL       string            `json:"url,omitempty"`
	Method    string            `json:"method,omitempty"`
	Path      string            `json:"path,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	Transform string            `json:"transform,omitempty"`
}

type suiteMock struct {
	Name      string               `json:"name,omitempty"`
	Match     suiteMockMatch       `json:"match"`
	Source    string               `json:"source,omitempty"`   // `case` | `inline` (default: case when Response is nil)
	Response  *suiteMockResponse   `json:"response,omitempty"` // inline response — `source: inline` or simply providing `response:`
	Hook      string               `json:"hook,omitempty"`
	Priority  *int                 `json:"priority,omitempty"`
	Consume   string               `json:"consume,omitempty"`
	LatencyMs *int                 `json:"latencyMs,omitempty"`
	_         struct{}             `json:"-"`
	Custom    map[string]any       `json:"-"` // reserved for future use
}

type suiteMockMatch struct {
	Direction  string `json:"direction,omitempty"`
	Service    string `json:"service,omitempty"`
	Host       string `json:"host,omitempty"`
	HostSuffix string `json:"hostSuffix,omitempty"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	PathPrefix string `json:"pathPrefix,omitempty"`
}

type suiteMockResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

type suiteAssertions struct {
	Status  json.RawMessage      `json:"status,omitempty"`
	Headers *suiteHeaderAssert   `json:"headers,omitempty"`
	Body    *suiteBodyAssert     `json:"body,omitempty"`
}

type suiteHeaderAssert struct {
	Include map[string]string `json:"include,omitempty"`
	Ignore  []string          `json:"ignore,omitempty"`
	Custom  string            `json:"custom,omitempty"`
}

type suiteBodyAssert struct {
	Mode       string   `json:"mode,omitempty"` // json-subset | exact | string | ignore
	Ignore     []string `json:"ignore,omitempty"`
	Redactions []struct {
		Path        string `json:"path"`
		Replacement string `json:"replacement"`
	} `json:"redactions,omitempty"`
	Custom string `json:"custom,omitempty"`
}

// parseSuiteDocument returns the parsed document plus any structural errors
// discovered while parsing (invalid YAML, missing top-level shape, etc.).
// It also expands `${VAR}` references against the process environment +
// anything declared in the suite's `env:` block.
func parseSuiteDocument(path string, raw []byte) (*suiteDocument, []string) {
	// Env expansion runs on raw bytes so YAML structure is preserved.
	expanded := expandEnvReferences(raw)
	normalized, err := yamlToJSON(expanded)
	if err != nil {
		return nil, []string{fmt.Sprintf("invalid suite document: %v", err)}
	}
	var doc suiteDocument
	if err := json.Unmarshal(normalized, &doc); err != nil {
		return nil, []string{fmt.Sprintf("invalid suite document: %v", err)}
	}
	// Fold top-level `mocks`/`request`/`assertions` into `defaults` so the
	// runner has a single source of truth. Top-level wins iff `defaults.*`
	// is unset — users don't normally set both.
	if doc.Defaults.Request == nil && doc.Request != nil {
		doc.Defaults.Request = doc.Request
	}
	if doc.Defaults.Mocks == nil && doc.Mocks != nil {
		doc.Defaults.Mocks = doc.Mocks
	}
	if doc.Defaults.Assertions == nil && doc.Assertions != nil {
		doc.Defaults.Assertions = doc.Assertions
	}
	return &doc, nil
}

var envReference = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// expandEnvReferences expands `${VAR}` and `${VAR:-default}` tokens in the
// suite document. Undefined variables without a default remain unexpanded
// (emitted verbatim) so validation can catch them instead of silently
// substituting the empty string.
func expandEnvReferences(raw []byte) []byte {
	return envReference.ReplaceAllFunc(raw, func(match []byte) []byte {
		parts := envReference.FindSubmatch(match)
		if len(parts) == 0 {
			return match
		}
		name := string(parts[1])
		fallback := ""
		hasDefault := false
		if len(parts) >= 3 && len(parts[2]) > 0 {
			fallback = string(parts[2])
			hasDefault = true
		}
		if val, ok := os.LookupEnv(name); ok {
			return []byte(val)
		}
		if hasDefault {
			return []byte(fallback)
		}
		return match
	})
}

// mergedDefaults shallow-merges per-case overrides over the suite defaults.
// `mocks` is unioned by `name` (override replaces entry with the same
// name; new entries append); everything else is replace-wholesale.
func mergedDefaults(defaults suiteDefaults, overrides *suiteDefaults) suiteDefaults {
	if overrides == nil {
		return defaults
	}
	out := defaults
	if overrides.Request != nil {
		out.Request = overrides.Request
	}
	if overrides.Assertions != nil {
		out.Assertions = overrides.Assertions
	}
	if len(overrides.Policy) > 0 {
		out.Policy = overrides.Policy
	}
	if len(overrides.Mocks) > 0 {
		merged := append([]suiteMock{}, out.Mocks...)
		for _, o := range overrides.Mocks {
			replaced := false
			for i, m := range merged {
				if m.Name != "" && m.Name == o.Name {
					merged[i] = o
					replaced = true
					break
				}
			}
			if !replaced {
				merged = append(merged, o)
			}
		}
		out.Mocks = merged
	}
	return out
}

// matchStatusExpectation reports whether `actual` satisfies the YAML-shaped
// status spec. Accepts int, []int, or {min,max}. A nil/empty spec means
// "any 2xx is fine".
func matchStatusExpectation(spec json.RawMessage, actual int) (bool, string) {
	if len(spec) == 0 || string(spec) == "null" {
		if actual >= 200 && actual < 300 {
			return true, ""
		}
		return false, fmt.Sprintf("expected 2xx, got %d", actual)
	}
	// Exact int
	var asInt int
	if err := json.Unmarshal(spec, &asInt); err == nil {
		if asInt == actual {
			return true, ""
		}
		return false, fmt.Sprintf("expected status %d, got %d", asInt, actual)
	}
	// List of accepted codes
	var asList []int
	if err := json.Unmarshal(spec, &asList); err == nil {
		for _, v := range asList {
			if v == actual {
				return true, ""
			}
		}
		return false, fmt.Sprintf("expected status in %v, got %d", asList, actual)
	}
	// Range object
	var asRange struct {
		Min *int `json:"min"`
		Max *int `json:"max"`
	}
	if err := json.Unmarshal(spec, &asRange); err == nil {
		if (asRange.Min == nil || actual >= *asRange.Min) &&
			(asRange.Max == nil || actual <= *asRange.Max) {
			return true, ""
		}
		return false, fmt.Sprintf("expected status in range (min=%v max=%v), got %d", asRange.Min, asRange.Max, actual)
	}
	return false, fmt.Sprintf("unsupported status expectation: %s", strings.TrimSpace(string(spec)))
}
