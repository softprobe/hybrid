package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runCapture implements `softprobe capture run` — a one-shot orchestration
// that starts a capture session, exports SOFTPROBE_SESSION_ID to the driver
// process, runs it, closes the session, and writes the captured case file
// (either to --out or to the runtime's default path).
func runCapture(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: softprobe capture run --driver CMD [--out PATH] [--timeout DURATION] [--redact-file PATH] [--json]")
		return exitInvalidArgs
	}
	switch args[0] {
	case "run":
		return runCaptureRun(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "capture: unknown subcommand %q\n", args[0])
		return exitInvalidArgs
	}
}

func runCaptureRun(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("capture run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	runtimeURL := fs.String("runtime-url", defaultRuntimeURL(), "control runtime base URL")
	driver := fs.String("driver", "", "shell command to run (receives SOFTPROBE_SESSION_ID)")
	target := fs.String("target", "", "target base URL (informational)")
	outPath := fs.String("out", "", "override capture output path")
	timeoutFlag := fs.String("timeout", "10m", "maximum driver runtime (Go duration)")
	redactFile := fs.String("redact-file", "", "redaction rules file applied before driver runs")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	_ = target

	if strings.TrimSpace(*driver) == "" {
		_, _ = fmt.Fprintln(stderr, "capture run requires --driver")
		return exitInvalidArgs
	}
	timeout, err := time.ParseDuration(*timeoutFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "capture run: invalid --timeout %q: %v\n", *timeoutFlag, err)
		return exitInvalidArgs
	}

	client := newHTTPClient(10 * time.Second)

	sessionID, code := captureStartSession(client, *runtimeURL, stderr)
	if code != exitOK {
		return code
	}

	if *redactFile != "" {
		if code := captureApplyRedaction(client, *runtimeURL, sessionID, *redactFile, stderr); code != exitOK {
			_, _ = captureClose(client, *runtimeURL, sessionID, "", nil)
			return code
		}
	}

	driverCode, driverErr := runDriver(*driver, sessionID, timeout, stdout, stderr)

	closePayload, closeCode := captureClose(client, *runtimeURL, sessionID, *outPath, stderr)
	if closeCode != exitOK && driverCode == exitOK {
		return closeCode
	}

	if *jsonOutput {
		payload := map[string]any{
			"sessionId":   sessionID,
			"exitCode":    driverCode,
			"capturePath": closePayload.CapturePath,
			"stats": map[string]any{
				"injectedSpans":  closePayload.Stats.InjectedSpans,
				"extractedSpans": closePayload.Stats.ExtractedSpans,
				"strictMisses":   closePayload.Stats.StrictMisses,
			},
		}
		if driverErr != nil {
			payload["driverError"] = driverErr.Error()
		}
		writeJSONEnvelope(stdout, statusFor(maybeErrors(driverErr)), driverCode, payload)
	} else if driverErr != nil {
		_, _ = fmt.Fprintf(stderr, "capture run: driver error: %v\n", driverErr)
	}

	if driverCode != exitOK {
		return driverCode
	}
	return exitOK
}

func captureStartSession(client *http.Client, runtimeURL string, stderr io.Writer) (string, int) {
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(runtimeURL, "/")+"/v1/sessions",
		bytes.NewBufferString(`{"mode":"capture"}`),
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "capture run: %v\n", err)
		return "", exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "capture run: runtime unreachable: %v\n", err)
		return "", classifyTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "capture run: session start failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return "", classifyHTTPError(resp.StatusCode, body)
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", exitGeneric
	}
	return created.SessionID, exitOK
}

func captureApplyRedaction(client *http.Client, runtimeURL, sessionID, redactFile string, stderr io.Writer) int {
	raw, err := os.ReadFile(redactFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "capture run: --redact-file: %v\n", err)
		return exitGeneric
	}
	normalized, err := normalizeRulesPayload(redactFile, raw)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "capture run: --redact-file: %v\n", err)
		return exitValidation
	}
	req, err := newRuntimeRequest(
		http.MethodPost,
		strings.TrimRight(runtimeURL, "/")+"/v1/sessions/"+sessionID+"/rules",
		bytes.NewReader(normalized),
	)
	if err != nil {
		return exitGeneric
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return classifyTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_, _ = fmt.Fprintf(stderr, "capture run: redact apply failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		return classifyHTTPError(resp.StatusCode, body)
	}
	return exitOK
}

func runDriver(command, sessionID string, timeout time.Duration, stdout, stderr io.Writer) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), "SOFTPROBE_SESSION_ID="+sessionID)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return exitGeneric, fmt.Errorf("driver timed out after %s", timeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), err
		}
		return exitGeneric, err
	}
	return exitOK, nil
}

type captureCloseResult struct {
	CapturePath string       `json:"capturePath"`
	Stats       statsPayload `json:"stats"`
}

func captureClose(client *http.Client, runtimeURL, sessionID, outPath string, stderr io.Writer) (captureCloseResult, int) {
	statsResp, statsCode := fetchSessionStatsQuiet(runtimeURL, sessionID)

	closeURL := strings.TrimRight(runtimeURL, "/") + "/v1/sessions/" + sessionID + "/close"
	if strings.TrimSpace(outPath) != "" {
		closeURL += "?out=" + percentEscape(outPath)
	}
	req, err := newRuntimeRequest(http.MethodPost, closeURL, nil)
	if err != nil {
		return captureCloseResult{}, exitGeneric
	}
	resp, err := client.Do(req)
	if err != nil {
		return captureCloseResult{}, classifyTransportError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if stderr != nil {
			_, _ = fmt.Fprintf(stderr, "capture run: close failed: status %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return captureCloseResult{}, classifyHTTPError(resp.StatusCode, body)
	}
	var closed struct {
		CapturePath string `json:"capturePath"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&closed)

	result := captureCloseResult{CapturePath: closed.CapturePath}
	if statsCode == exitOK {
		result.Stats = statsPayload{
			InjectedSpans:  statsResp.Stats.InjectedSpans,
			ExtractedSpans: statsResp.Stats.ExtractedSpans,
			StrictMisses:   statsResp.Stats.StrictMisses,
		}
	}
	return result, exitOK
}

// percentEscape is a tiny stand-in for url.QueryEscape that keeps `/` legible
// in human-readable paths while still escaping whitespace.
func percentEscape(s string) string {
	b := &bytes.Buffer{}
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteString("%20")
		case r == '+':
			b.WriteString("%2B")
		case r == '&':
			b.WriteString("%26")
		case r == '#':
			b.WriteString("%23")
		case r == '?':
			b.WriteString("%3F")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func maybeErrors(err error) []string {
	if err == nil {
		return nil
	}
	return []string{err.Error()}
}
