package proxybackend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const captureCaseFilePath = "e2e/captured.case.json"
const captureCaseFilePathEnv = "SOFTPROBE_CAPTURE_CASE_PATH"

// WriteCapturedCase writes buffered capture payloads to the default case file path.
func WriteCapturedCase(caseID string, tracePayloads [][]byte) error {
	traces := make([]json.RawMessage, 0, len(tracePayloads))
	for _, payload := range tracePayloads {
		var td tracev1.TracesData
		if err := protojson.Unmarshal(payload, &td); err != nil {
			return fmt.Errorf("parse trace payload: %w", err)
		}
		out, err := protojson.MarshalOptions{Multiline: true, Indent: "  ", UseProtoNames: false}.Marshal(&td)
		if err != nil {
			return fmt.Errorf("marshal trace: %w", err)
		}
		traces = append(traces, json.RawMessage(out))
	}

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
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Traces:    traces,
	}

	outputPath := captureOutputPath()
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	enc, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	_, err = file.Write(append(enc, '\n'))
	return err
}

func captureOutputPath() string {
	if path := os.Getenv(captureCaseFilePathEnv); path != "" {
		return path
	}
	return captureCaseFilePath
}
