package proxybackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileSchemeWritesToDisk verifies that a file:// URL in the capture path
// is treated identically to a plain OS path.
func TestFileSchemeWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.case.json")
	t.Setenv(captureCaseFilePathEnv, "file://"+outPath)

	if err := WriteCapturedCase("s1", nil); err != nil {
		t.Fatalf("WriteCapturedCase: %v", err)
	}

	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected file at %s: %v", outPath, err)
	}
}

// TestUnrecognizedSchemeReturnsError verifies that an unrecognized URI scheme
// returns a descriptive error rather than silently failing.
func TestUnrecognizedSchemeReturnsError(t *testing.T) {
	t.Setenv(captureCaseFilePathEnv, "ftp://host/path/out.case.json")

	err := WriteCapturedCase("s2", nil)
	if err == nil {
		t.Fatal("expected error for unsupported scheme, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected 'unsupported' in error, got: %v", err)
	}
}
