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

// runInspectSession is a read-only dump of a live session — policy, rules,
// loaded case summary, and stats. The runtime does not currently expose a
// single composite endpoint, so we stitch the public reads together and
// normalise the result per docs-site/reference/cli.md#inspect-session.
func runInspectSession(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect session", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", "http://127.0.0.1:8080", "control runtime base URL")
	sessionID := fs.String("session", "", "session ID")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	if *sessionID == "" {
		_, _ = fmt.Fprintln(stderr, "inspect session requires --session")
		return exitInvalidArgs
	}

	// /v1/sessions/{id}/state aggregates mode, policy, rules, case summary,
	// and stats in a single read so we don't paper over inconsistency with
	// multiple non-atomic calls. Older runtimes without that route fall back
	// to stats-only.
	client := newHTTPClient(5 * time.Second)
	stateReq, err := newRuntimeRequest(
		http.MethodGet,
		strings.TrimRight(*runtimeURL, "/")+"/v1/sessions/"+*sessionID+"/state",
		nil,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "inspect session failed: %v\n", err)
		return exitGeneric
	}
	stateResp, err := client.Do(stateReq)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "inspect session failed: %v\n", err)
		return classifyTransportError(err)
	}
	defer stateResp.Body.Close()

	if stateResp.StatusCode == http.StatusNotFound {
		body, _ := io.ReadAll(stateResp.Body)
		if matchesErrorCode(body, "unknown_session") {
			_, _ = fmt.Fprintf(stderr, "inspect session failed: %s\n", strings.TrimSpace(string(body)))
			return exitSessionNotFound
		}
		// Route genuinely missing — fall back to stats so old runtimes still
		// produce a useful (if partial) answer.
		stats, code := fetchSessionStats(*runtimeURL, *sessionID, stderr)
		if code != exitOK {
			return code
		}
		return emitInspectSession(stdout, *jsonOutput, sessionState{
			SessionID:       stats.SessionID,
			SessionRevision: stats.SessionRevision,
			Mode:            stats.Mode,
			Stats: statsPayload{
				InjectedSpans:  stats.Stats.InjectedSpans,
				ExtractedSpans: stats.Stats.ExtractedSpans,
				StrictMisses:   stats.Stats.StrictMisses,
			},
		})
	}
	if stateResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(stateResp.Body)
		_, _ = fmt.Fprintf(stderr, "inspect session failed: status %d: %s\n", stateResp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(stateResp.StatusCode, body)
	}

	var state sessionState
	if err := json.NewDecoder(stateResp.Body).Decode(&state); err != nil {
		_, _ = fmt.Fprintf(stderr, "inspect session failed: invalid body: %v\n", err)
		return exitGeneric
	}
	return emitInspectSession(stdout, *jsonOutput, state)
}

type sessionState struct {
	SessionID       string          `json:"sessionId"`
	SessionRevision int             `json:"sessionRevision"`
	Mode            string          `json:"mode"`
	Policy          json.RawMessage `json:"policy,omitempty"`
	Rules           json.RawMessage `json:"rules,omitempty"`
	CaseSummary     caseSummaryDoc  `json:"caseSummary"`
	Stats           statsPayload    `json:"stats"`
}

type caseSummaryDoc struct {
	CaseID     string `json:"caseId,omitempty"`
	TraceCount int    `json:"traceCount"`
}

type statsPayload struct {
	InjectedSpans  int `json:"injectedSpans"`
	ExtractedSpans int `json:"extractedSpans"`
	StrictMisses   int `json:"strictMisses"`
}

func emitInspectSession(stdout io.Writer, jsonOutput bool, state sessionState) int {
	if jsonOutput {
		payload := map[string]any{
			"sessionId":       state.SessionID,
			"sessionRevision": state.SessionRevision,
			"mode":            state.Mode,
			"caseSummary":     state.CaseSummary,
			"stats":           state.Stats,
		}
		if len(state.Policy) > 0 {
			payload["policy"] = json.RawMessage(state.Policy)
		}
		if len(state.Rules) > 0 {
			payload["rules"] = json.RawMessage(state.Rules)
		}
		writeJSONEnvelope(stdout, "ok", exitOK, payload)
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "session %s\n", state.SessionID)
	_, _ = fmt.Fprintf(stdout, "revision: %d\n", state.SessionRevision)
	_, _ = fmt.Fprintf(stdout, "mode: %s\n", state.Mode)
	if state.CaseSummary.CaseID != "" {
		_, _ = fmt.Fprintf(stdout, "case: %s (%d traces)\n", state.CaseSummary.CaseID, state.CaseSummary.TraceCount)
	} else {
		_, _ = fmt.Fprintln(stdout, "case: (none loaded)")
	}
	_, _ = fmt.Fprintf(stdout, "stats: injected=%d extracted=%d strictMisses=%d\n",
		state.Stats.InjectedSpans, state.Stats.ExtractedSpans, state.Stats.StrictMisses)
	if len(state.Policy) > 0 {
		_, _ = fmt.Fprintf(stdout, "policy: %s\n", string(state.Policy))
	}
	if len(state.Rules) > 0 {
		_, _ = fmt.Fprintf(stdout, "rules: %s\n", string(state.Rules))
	}
	return exitOK
}
