package main

import (
	"encoding/json"
	"fmt"
)

// suiteDocument is the parsed shape of `*.suite.yaml` files.
// docs-site/reference/suite-yaml.md documents this surface; we keep the Go
// type conservative so we can evolve the schema in future versions without
// breaking CI for valid-today suites.
type suiteDocument struct {
	Name  string            `json:"name"`
	Hooks []string          `json:"hooks,omitempty"`
	Cases []suiteCaseEntry  `json:"cases"`
	Env   map[string]string `json:"env,omitempty"`
}

type suiteCaseEntry struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
	Only bool   `json:"only,omitempty"`
	Skip bool   `json:"skip,omitempty"`
}

// parseSuiteDocument returns the parsed document plus any structural errors
// discovered while parsing (invalid YAML, missing top-level shape, etc.).
func parseSuiteDocument(path string, raw []byte) (*suiteDocument, []string) {
	normalized, err := yamlToJSON(raw)
	if err != nil {
		return nil, []string{fmt.Sprintf("invalid suite document: %v", err)}
	}
	var doc suiteDocument
	if err := json.Unmarshal(normalized, &doc); err != nil {
		return nil, []string{fmt.Sprintf("invalid suite document: %v", err)}
	}
	return &doc, nil
}
