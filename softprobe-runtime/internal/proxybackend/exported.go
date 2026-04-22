package proxybackend

import (
	"encoding/json"
	"fmt"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// ExtractRequest is the parsed result of an OTLP extract upload.
type ExtractRequest struct {
	SessionID string
	SpanCount int
}

// ParseExtractRequest parses an OTLP JSON payload and returns the session ID
// and span count. Used by hostedbackend.
func ParseExtractRequest(payload []byte) (*ExtractRequest, error) {
	r, err := parseExtractUploadRequest(payload)
	if err != nil {
		return nil, err
	}
	return &ExtractRequest{SessionID: r.SessionID, SpanCount: r.SpanCount}, nil
}

// NormalizeOTLPJSON normalizes a protobuf or JSON OTLP payload to JSON.
// Exported for use by hostedbackend.
func NormalizeOTLPJSON(payload []byte) ([]byte, error) {
	return normalizeOTLPJSON(payload)
}

// BuildCaseJSON merges OTLP extract payloads into the case JSON format.
// Exported for use by hostedbackend.
func BuildCaseJSON(caseID string, tracePayloads [][]byte) ([]byte, error) {
	traces := make([]json.RawMessage, 0, len(tracePayloads))
	for _, payload := range tracePayloads {
		var td tracev1.TracesData
		if err := protojson.Unmarshal(payload, &td); err != nil {
			return nil, fmt.Errorf("parse trace payload: %w", err)
		}
		out, err := protojson.MarshalOptions{Multiline: true, Indent: "  ", UseProtoNames: false}.Marshal(&td)
		if err != nil {
			return nil, fmt.Errorf("marshal trace: %w", err)
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

	return json.MarshalIndent(doc, "", "  ")
}
