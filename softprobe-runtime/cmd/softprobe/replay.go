package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// runReplay dispatches `softprobe replay {run}` diagnostics. `replay run`
// (per docs-site/reference/cli.md#replay-run) is a thin wrapper around
// `session stats` tailored for humans monitoring a live replay.
func runReplay(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: softprobe replay run --session ID [--json]")
		return exitInvalidArgs
	}
	switch args[0] {
	case "run":
		return runReplayRun(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "replay: unknown subcommand %q\n", args[0])
		return exitInvalidArgs
	}
}

func runReplayRun(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("replay run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "replay run requires --session")
		return exitInvalidArgs
	}

	stats, code := fetchSessionStats(*runtimeURL, *sessionID, stderr)
	if code != exitOK {
		return code
	}

	// docs-site/reference/cli.md promises `hits` and `misses`. Mirror
	// session stats counters onto those names so agents can diff the two.
	hits := stats.Stats.InjectedSpans
	misses := stats.Stats.StrictMisses

	if *jsonOutput {
		writeJSONEnvelope(stdout, "ok", exitOK, map[string]any{
			"sessionId": stats.SessionID,
			"exitCode":  exitOK,
			"stats": map[string]any{
				"hits":           hits,
				"misses":         misses,
				"injectedSpans":  stats.Stats.InjectedSpans,
				"extractedSpans": stats.Stats.ExtractedSpans,
				"strictMisses":   stats.Stats.StrictMisses,
			},
		})
		return exitOK
	}

	_, _ = fmt.Fprintf(
		stdout,
		"session %s\nhits: %d\nmisses: %d\ninjectedSpans: %d\nextractedSpans: %d\n",
		stats.SessionID, hits, misses, stats.Stats.InjectedSpans, stats.Stats.ExtractedSpans,
	)
	return exitOK
}

type sessionStatsPayload struct {
	SessionID       string `json:"sessionId"`
	SessionRevision int    `json:"sessionRevision"`
	Mode            string `json:"mode"`
	Stats           struct {
		InjectedSpans  int `json:"injectedSpans"`
		ExtractedSpans int `json:"extractedSpans"`
		StrictMisses   int `json:"strictMisses"`
	} `json:"stats"`
}

func fetchSessionStats(runtimeURL, sessionID string, stderr io.Writer) (sessionStatsPayload, int) {
	client := newHTTPClient(5 * time.Second)
	req, err := newRuntimeRequest(
		http.MethodGet,
		strings.TrimRight(runtimeURL, "/")+"/v1/sessions/"+sessionID+"/stats",
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "fetch session stats failed: %v\n", err)
		return sessionStatsPayload{}, exitGeneric
	}
	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "fetch session stats failed: %v\n", err)
		return sessionStatsPayload{}, classifyTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "fetch session stats failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return sessionStatsPayload{}, classifyHTTPError(resp.StatusCode, body)
	}
	var payload sessionStatsPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		_, _ = fmt.Fprintf(stderr, "fetch session stats failed: invalid body: %v\n", err)
		return sessionStatsPayload{}, exitGeneric
	}
	return payload, exitOK
}
