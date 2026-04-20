package e2etestutil

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// ValidateCaseFile runs ajv-cli against the repo's case JSON Schema.
func ValidateCaseFile(t *testing.T, path string) {
	t.Helper()

	repoRoot := RepoRoot()
	schema := filepath.Join("spec", "schemas", "case.schema.json")
	traceSchema := filepath.Join("spec", "schemas", "case-trace.schema.json")

	cmd := exec.Command(
		"npx",
		"-y",
		"ajv-cli@5",
		"validate",
		"-s",
		schema,
		"-r",
		traceSchema,
		"-d",
		path,
		"--spec=draft2020",
	)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("case validation failed: %v\n%s", err, output)
	}
}
