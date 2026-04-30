// Package version holds the softprobe CLI release identifier, normally
// injected at link time.
package version

import (
	"fmt"
	"strings"
)

// Version is the semver-ish label for this CLI build. Release pipelines
// should set it explicitly, for example:
//
//	go build -ldflags "-X softprobe-runtime/internal/version.Version=v0.5.0" -o softprobe ./cmd/softprobe
//
// When not injected, the default dev sentinel is used.
var Version = "0.0.0-dev"

// SemverTag returns a normalized v-prefixed label (e.g. "v0.5.0",
// "v0.0.0-dev") for diagnostics and drift checks.
func SemverTag() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		v = "0.0.0-dev"
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// CLIDetail is the string printed after "softprobe " for --version and the
// first line of `softprobe doctor`, e.g. "v0.5.0 (spec http-control-api@v1)".
func CLIDetail(specVersion string) string {
	return fmt.Sprintf("%s (spec %s)", SemverTag(), strings.TrimSpace(specVersion))
}
