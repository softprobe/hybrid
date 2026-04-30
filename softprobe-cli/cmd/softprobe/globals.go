package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// verboseStderr is set by `run()` when --verbose is enabled so that the
// verbose HTTP transport has somewhere to emit per-request lines without
// threading an io.Writer through every subcommand. A nil value disables
// logging.
var verboseStderr io.Writer

// newHTTPClient returns an *http.Client honoring the global verbose flag:
// when --verbose is active, every outbound request/response pair is logged
// to stderr on a single line each (method, URL, status). Header values are
// intentionally omitted so Authorization tokens do not leak into CI logs.
func newHTTPClient(timeout time.Duration) *http.Client {
	base := http.DefaultTransport
	if globals != nil && globals.verbose && verboseStderr != nil {
		base = &verboseRoundTripper{base: base, out: verboseStderr}
	}
	return &http.Client{Timeout: timeout, Transport: base}
}

type verboseRoundTripper struct {
	base http.RoundTripper
	out  io.Writer
}

func (v *verboseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	_, _ = fmt.Fprintf(v.out, "%s %s\n", req.Method, req.URL)
	resp, err := v.base.RoundTrip(req)
	if err != nil {
		_, _ = fmt.Fprintf(v.out, "← error: %v\n", err)
		return nil, err
	}
	_, _ = fmt.Fprintf(v.out, "← %s %s\n", strings.TrimSpace(resp.Status), req.URL)
	return resp, nil
}

// globalFlags is the snapshot of global CLI flags (--verbose, --quiet,
// --no-color) extracted from the command-line before subcommand dispatch.
// Subcommands read this via the package-level `globals` variable. The value
// is reset on every `run()` call to keep tests hermetic.
type globalFlags struct {
	verbose bool
	quiet   bool
	noColor bool
	help    bool
}

var globals = &globalFlags{}

// extractGlobalFlags strips documented global flags out of args and returns
// the remaining argv along with the captured state. NO_COLOR is consulted
// from the environment to match the de-facto cross-ecosystem standard.
func extractGlobalFlags(args []string) ([]string, *globalFlags) {
	state := &globalFlags{
		noColor: os.Getenv("NO_COLOR") != "",
	}
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--verbose":
			state.verbose = true
		case "--quiet", "-q":
			state.quiet = true
		case "--no-color":
			state.noColor = true
		default:
			out = append(out, a)
		}
	}
	// -v alone on the command line is documented as --verbose; but the
	// existing contract (and many scripts) also accept "-v" as a shortcut for
	// --version. Resolve via position: if -v is the *first* argument and no
	// subcommand follows, treat it as --version; otherwise it's --verbose.
	resolved := make([]string, 0, len(out))
	for i, a := range out {
		if a == "-v" {
			if i == len(out)-1 {
				resolved = append(resolved, "--version")
				continue
			}
			state.verbose = true
			continue
		}
		resolved = append(resolved, a)
	}
	// --help sticks around so subcommand FlagSets can also see it.
	for _, a := range resolved {
		if a == "--help" || a == "-h" {
			state.help = true
			break
		}
	}
	return resolved, state
}

