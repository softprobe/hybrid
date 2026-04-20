package e2etestutil

import (
	"path/filepath"
	"runtime"
)

// ModuleRoot returns the absolute path to the e2e directory (the folder
// that contains go.mod for the e2e module and captured.case.json). Used to
// resolve paths regardless of which subpackage's tests are executing.
func ModuleRoot() string {
	_, file, _, _ := runtime.Caller(0)
	// .../e2e/go/e2etestutil/module_root.go -> .../e2e
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// RepoRoot returns the repository root (parent of e2e/).
func RepoRoot() string {
	return filepath.Clean(filepath.Join(ModuleRoot(), ".."))
}
