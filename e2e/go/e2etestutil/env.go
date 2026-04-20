package e2etestutil

import (
	"os"
	"testing"
	"time"
)

// MustEnv returns os.Getenv(key) or def if empty.
func MustEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// WaitForFile polls until path exists or times out.
func WaitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("file %s not created", path)
}
