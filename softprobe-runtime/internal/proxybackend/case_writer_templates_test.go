package proxybackend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInterpolateCapturePathSessionId(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "{sessionId}.case.json")
	t.Setenv(captureCaseFilePathEnv, tmpl)

	if err := WriteCapturedCase("sess_abc123", nil); err != nil {
		t.Fatalf("WriteCapturedCase: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	var found bool
	for _, e := range entries {
		if strings.Contains(e.Name(), "sess_abc123") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected file with sess_abc123 in name, got: %v", entries)
	}
}

func TestInterpolateCapturePathTs(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "case-{ts}.json")
	t.Setenv(captureCaseFilePathEnv, tmpl)

	if err := WriteCapturedCase("s1", nil); err != nil {
		t.Fatalf("WriteCapturedCase: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("no file written")
	}
	name := entries[0].Name()
	if strings.Contains(name, "{ts}") {
		t.Fatalf("placeholder not expanded: %s", name)
	}
}

func TestInterpolateCapturePathMode(t *testing.T) {
	dir := t.TempDir()
	tmpl := filepath.Join(dir, "{mode}-{sessionId}.case.json")
	t.Setenv(captureCaseFilePathEnv, tmpl)

	if err := WriteCapturedCase("s2", nil); err != nil {
		t.Fatalf("WriteCapturedCase: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("no file written")
	}
	name := entries[0].Name()
	if strings.Contains(name, "{mode}") {
		t.Fatalf("placeholder not expanded: %s", name)
	}
	if !strings.HasPrefix(name, "capture-") {
		t.Fatalf("expected mode=capture prefix, got: %s", name)
	}
}
