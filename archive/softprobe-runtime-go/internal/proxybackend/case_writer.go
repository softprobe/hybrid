package proxybackend

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const captureCaseFilePath = "e2e/captured.case.json"
const captureCaseFilePathEnv = "SOFTPROBE_CAPTURE_CASE_PATH"

// WriteCapturedCase writes buffered capture payloads to the default case file path.
// Callers that want a specific destination should use WriteCapturedCaseTo.
func WriteCapturedCase(caseID string, tracePayloads [][]byte) error {
	_, err := WriteCapturedCaseTo(caseID, tracePayloads, "")
	return err
}

// WriteCapturedCaseTo writes buffered capture payloads to override (when
// non-empty), otherwise to SOFTPROBE_CAPTURE_CASE_PATH or the default path.
// It returns the path that was actually written.
//
// Templating: the {sessionId} and {ts} placeholders in the resolved path are
// interpolated with the caseID and an RFC3339 UTC timestamp respectively so
// that operators can pin down a stable file-per-session layout.
func WriteCapturedCaseTo(caseID string, tracePayloads [][]byte, override string) (string, error) {
	traces := make([]json.RawMessage, 0, len(tracePayloads))
	for _, payload := range tracePayloads {
		var td tracev1.TracesData
		if err := protojson.Unmarshal(payload, &td); err != nil {
			return "", fmt.Errorf("parse trace payload: %w", err)
		}
		out, err := protojson.MarshalOptions{Multiline: true, Indent: "  ", UseProtoNames: false}.Marshal(&td)
		if err != nil {
			return "", fmt.Errorf("marshal trace: %w", err)
		}
		traces = append(traces, json.RawMessage(out))
	}

	now := time.Now().UTC()
	doc := struct {
		Version   string            `json:"version"`
		CaseID    string            `json:"caseId"`
		Mode      string            `json:"mode"`
		CreatedAt string            `json:"createdAt"`
		Traces    []json.RawMessage `json:"traces"`
	}{
		Version:   "1.0.0",
		CaseID:    caseID,
		Mode:      "capture",
		CreatedAt: now.Format(time.RFC3339),
		Traces:    traces,
	}

	rawPath := captureOutputPath(override)
	outputPath := interpolateCapturePath(rawPath, caseID, now)

	enc, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	data := append(enc, '\n')

	// Resolve URI scheme. file:// and bare paths go to local disk.
	// s3://, gs://, azblob:// are placeholders that surface an unsupported
	// error until the relevant client is wired in.
	localPath, schemeErr := resolveLocalPath(outputPath)
	if schemeErr != nil {
		return "", schemeErr
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return "", err
	}
	file, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	return localPath, nil
}

// resolveLocalPath strips a file:// prefix from path and returns the OS path.
// For s3://, gs://, azblob:// it returns an unsupported error so callers get a
// clear message rather than a cryptic open() failure.
func resolveLocalPath(path string) (string, error) {
	// Fast path: no scheme — plain OS path.
	if !strings.Contains(path, "://") {
		return path, nil
	}
	u, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid capture path %q: %w", path, err)
	}
	switch u.Scheme {
	case "file":
		return u.Host + u.Path, nil
	case "s3", "gs", "azblob":
		return "", fmt.Errorf("unsupported scheme %q in SOFTPROBE_CAPTURE_CASE_PATH — object-storage writers not yet wired for OSS runtime", u.Scheme)
	default:
		return "", fmt.Errorf("unsupported scheme %q in SOFTPROBE_CAPTURE_CASE_PATH", u.Scheme)
	}
}

func captureOutputPath(override string) string {
	if override != "" {
		return override
	}
	if path := os.Getenv(captureCaseFilePathEnv); path != "" {
		return path
	}
	return captureCaseFilePath
}

func interpolateCapturePath(path, sessionID string, at time.Time) string {
	// Cheap, template-free substitution: the documented placeholders are a
	// tiny closed set so a full text/template engine would be overkill.
	replacements := []struct {
		placeholder, value string
	}{
		{"{sessionId}", sessionID},
		{"{ts}", at.Format("20060102T150405Z")},
		{"{mode}", "capture"},
	}
	out := path
	for _, r := range replacements {
		out = strings.ReplaceAll(out, r.placeholder, r.value)
	}
	return out
}
