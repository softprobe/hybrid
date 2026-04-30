package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runExport implements `softprobe export otlp` — reads captured case files
// and forwards their trace payloads to an OpenTelemetry HTTP/JSON endpoint.
// The captured payloads already conform to opentelemetry-proto tracesData,
// so we simply POST them individually as OTLP/JSON. Errors short-circuit
// with the usual runtime-unreachable vs generic distinction.
func runExport(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: softprobe export otlp --case GLOB --endpoint URL")
		return exitInvalidArgs
	}
	switch args[0] {
	case "otlp":
		return runExportOTLP(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "export: unknown subcommand %q\n", args[0])
		return exitInvalidArgs
	}
}

func runExportOTLP(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("export otlp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	caseGlob := fs.String("case", "", "case file glob")
	endpoint := fs.String("endpoint", "", "OTLP/HTTP traces endpoint (e.g. http://collector:4318/v1/traces)")
	jsonOutput := fs.Bool("json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return exitInvalidArgs
	}
	if *caseGlob == "" || *endpoint == "" {
		_, _ = fmt.Fprintln(stderr, "export otlp requires --case and --endpoint")
		return exitInvalidArgs
	}

	matches, err := filepath.Glob(*caseGlob)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "export otlp: invalid glob: %v\n", err)
		return exitInvalidArgs
	}
	if len(matches) == 0 {
		if _, err := os.Stat(*caseGlob); err == nil {
			matches = []string{*caseGlob}
		}
	}
	if len(matches) == 0 {
		_, _ = fmt.Fprintln(stderr, "export otlp: no case files matched")
		return exitGeneric
	}

	client := newHTTPClient(30 * time.Second)
	var sent, failed int
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(stderr, "%s: read: %v\n", path, err)
			continue
		}
		var doc caseDocument
		if err := json.Unmarshal(raw, &doc); err != nil {
			failed++
			_, _ = fmt.Fprintf(stderr, "%s: parse: %v\n", path, err)
			continue
		}
		// The captured case file stores `traces` as a list of tracesData
		// payloads. Re-encode each as stand-alone JSON and POST it.
		var tracesWrapper struct {
			Traces []json.RawMessage `json:"traces"`
		}
		if err := json.Unmarshal(raw, &tracesWrapper); err != nil {
			failed++
			continue
		}
		for _, payload := range tracesWrapper.Traces {
			if err := postOTLPJSON(client, *endpoint, payload); err != nil {
				failed++
				_, _ = fmt.Fprintf(stderr, "%s: post: %v\n", path, err)
				continue
			}
			sent++
		}
	}

	if *jsonOutput {
		writeJSONEnvelope(stdout, statusForCounts(failed), exitCodeForCounts(failed), map[string]any{
			"sent":   sent,
			"failed": failed,
		})
	} else {
		_, _ = fmt.Fprintf(stdout, "export otlp: sent=%d failed=%d\n", sent, failed)
	}
	if failed > 0 {
		return exitGeneric
	}
	return exitOK
}

func postOTLPJSON(client *http.Client, endpoint string, payload []byte) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func statusForCounts(failed int) string {
	if failed == 0 {
		return "ok"
	}
	return "fail"
}

func exitCodeForCounts(failed int) int {
	if failed == 0 {
		return exitOK
	}
	return exitGeneric
}
