package datalake

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) IngestTraces(ctx context.Context, payload []byte) error {
	// Datalake accepts OTLP protobuf robustly; converting here avoids JSON-field
	// naming differences between implementations.
	var td tracev1.TracesData
	if err := protojson.Unmarshal(payload, &td); err != nil {
		return fmt.Errorf("datalake: decode otlp json payload: %w", err)
	}
	wire, err := proto.Marshal(&td)
	if err != nil {
		return fmt.Errorf("datalake: encode otlp protobuf payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/traces", bytes.NewReader(wire))
	if err != nil {
		return fmt.Errorf("datalake: build ingest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("datalake: ingest request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("datalake: ingest failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type SQLQueryResponse struct {
	Columns  []string            `json:"columns"`
	Rows     [][]json.RawMessage `json:"rows"`
	RowCount int                 `json:"row_count"`
}

func (c *Client) QuerySQL(ctx context.Context, sql string) (SQLQueryResponse, error) {
	reqBody, err := json.Marshal(map[string]string{"sql": sql})
	if err != nil {
		return SQLQueryResponse{}, fmt.Errorf("datalake: marshal sql request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/query/sql", bytes.NewReader(reqBody))
	if err != nil {
		return SQLQueryResponse{}, fmt.Errorf("datalake: build sql request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return SQLQueryResponse{}, fmt.Errorf("datalake: sql request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return SQLQueryResponse{}, fmt.Errorf("datalake: sql query failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out SQLQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return SQLQueryResponse{}, fmt.Errorf("datalake: decode sql response: %w", err)
	}
	return out, nil
}
