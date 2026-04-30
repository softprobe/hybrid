package main

import (
	"fmt"

	"sigs.k8s.io/yaml"
)

// yamlToJSON converts a YAML document to JSON so that downstream runtime
// handlers (which only speak JSON) can consume it unchanged. The function
// tolerates inputs that are already JSON — the sigs.k8s.io/yaml library
// treats JSON as a YAML subset.
func yamlToJSON(raw []byte) ([]byte, error) {
	out, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return out, nil
}
