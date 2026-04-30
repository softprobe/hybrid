package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// assertResponse compares a SUT response against the case's captured
// response using the assertions config from `suite.yaml`. Returns a list
// of issue strings; empty means "pass".
//
// The comparator is intentionally small. Hooks (`body.custom`) handle
// everything this function can't express natively; the idea is to let
// YAML authors say *what* they care about and punt custom invariants to
// TypeScript/Python code.
func assertResponse(spec *suiteAssertions, actual sutResponse, captured *capturedResponse) []string {
	var issues []string

	// ---- status ----
	statusSpec := json.RawMessage(nil)
	if spec != nil {
		statusSpec = spec.Status
	}
	if len(statusSpec) == 0 && captured != nil {
		// Default expectation: match the captured status exactly. This is
		// the right behavior for `replay` mode — you captured a 200, you
		// expect a 200 today.
		statusSpec = json.RawMessage(fmt.Sprintf("%d", captured.Status))
	}
	if ok, msg := matchStatusExpectation(statusSpec, actual.Status); !ok {
		issues = append(issues, msg)
	}

	// ---- headers ----
	if spec != nil && spec.Headers != nil {
		for name, pattern := range spec.Headers.Include {
			value, present := lookupHeaderCI(actual.Headers, name)
			if !present {
				issues = append(issues, fmt.Sprintf("header %q: missing", name))
				continue
			}
			if pattern == "" {
				continue
			}
			matched, err := regexp.MatchString(pattern, value)
			if err != nil {
				issues = append(issues, fmt.Sprintf("header %q: invalid regex %q: %v", name, pattern, err))
				continue
			}
			if !matched {
				issues = append(issues, fmt.Sprintf("header %q: %q does not match /%s/", name, value, pattern))
			}
		}
	}

	// ---- body ----
	mode := "json-subset"
	var ignore []string
	if spec != nil && spec.Body != nil {
		if spec.Body.Mode != "" {
			mode = spec.Body.Mode
		}
		ignore = spec.Body.Ignore
	}
	if mode != "ignore" && captured != nil {
		if msg := compareBodies(mode, captured.Body, actual.Body, ignore); msg != "" {
			issues = append(issues, msg)
		}
	}

	return issues
}

func compareBodies(mode, expectedBody, actualBody string, ignore []string) string {
	switch mode {
	case "string":
		if expectedBody != actualBody {
			return fmt.Sprintf("body (string): expected %q, got %q", truncate(expectedBody), truncate(actualBody))
		}
		return ""
	case "exact":
		expected, err1 := parseJSONLoose(expectedBody)
		actual, err2 := parseJSONLoose(actualBody)
		if err1 != nil || err2 != nil {
			if expectedBody != actualBody {
				return fmt.Sprintf("body (exact, non-JSON): expected %q, got %q", truncate(expectedBody), truncate(actualBody))
			}
			return ""
		}
		expected = stripJSONPaths(expected, ignore)
		actual = stripJSONPaths(actual, ignore)
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Sprintf("body (exact) differs: expected=%s actual=%s",
				truncate(mustMarshal(expected)), truncate(mustMarshal(actual)))
		}
		return ""
	case "json-subset", "":
		expected, err1 := parseJSONLoose(expectedBody)
		actual, err2 := parseJSONLoose(actualBody)
		if err1 != nil || err2 != nil {
			// Non-JSON bodies fall back to a byte-level compare under
			// `json-subset`, matching the TS adapter's behavior.
			if expectedBody != actualBody {
				return fmt.Sprintf("body: non-JSON mismatch: expected %q, got %q", truncate(expectedBody), truncate(actualBody))
			}
			return ""
		}
		expected = stripJSONPaths(expected, ignore)
		actual = stripJSONPaths(actual, ignore)
		if diff := jsonSubsetDiff(expected, actual, ""); diff != "" {
			return "body (json-subset): " + diff
		}
		return ""
	}
	return fmt.Sprintf("body: unsupported mode %q", mode)
}

// jsonSubsetDiff returns the first path where `actual` fails to contain
// all key/value pairs in `expected`. Arrays are compared by index.
func jsonSubsetDiff(expected, actual any, path string) string {
	switch exp := expected.(type) {
	case map[string]any:
		act, ok := actual.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: expected object, got %T", orRoot(path), actual)
		}
		for k, v := range exp {
			nested := path + "." + k
			if path == "" {
				nested = "$." + k
			}
			sub, present := act[k]
			if !present {
				return fmt.Sprintf("%s: missing key", nested)
			}
			if d := jsonSubsetDiff(v, sub, nested); d != "" {
				return d
			}
		}
	case []any:
		act, ok := actual.([]any)
		if !ok {
			return fmt.Sprintf("%s: expected array, got %T", orRoot(path), actual)
		}
		if len(exp) > len(act) {
			return fmt.Sprintf("%s: expected >= %d items, got %d", orRoot(path), len(exp), len(act))
		}
		for i, v := range exp {
			nested := fmt.Sprintf("%s[%d]", path, i)
			if d := jsonSubsetDiff(v, act[i], nested); d != "" {
				return d
			}
		}
	default:
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Sprintf("%s: expected %v, got %v", orRoot(path), expected, actual)
		}
	}
	return ""
}

func orRoot(p string) string {
	if p == "" {
		return "$"
	}
	return p
}

// stripJSONPaths removes simple top-level `$.foo.bar` paths from a JSON
// value. Does not support wildcards or array indexing yet — enough for
// the common "ignore createdAt / paymentId" use case.
func stripJSONPaths(v any, paths []string) any {
	if len(paths) == 0 {
		return v
	}
	for _, p := range paths {
		p = strings.TrimPrefix(p, "$.")
		if p == "" {
			continue
		}
		segments := strings.Split(p, ".")
		deleteJSONPath(v, segments)
	}
	return v
}

func deleteJSONPath(v any, segments []string) {
	if len(segments) == 0 {
		return
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	if len(segments) == 1 {
		delete(obj, segments[0])
		return
	}
	child, ok := obj[segments[0]]
	if !ok {
		return
	}
	deleteJSONPath(child, segments[1:])
}

func parseJSONLoose(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func mustMarshal(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func truncate(s string) string {
	const max = 160
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func lookupHeaderCI(headers map[string]string, name string) (string, bool) {
	lower := strings.ToLower(name)
	for k, v := range headers {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return "", false
}

// sutResponse is the minimal shape we need from the SUT call. We keep it
// separate from `capturedResponse` because the types carry different
// semantics (captured is authored; SUT is observed live).
type sutResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}
