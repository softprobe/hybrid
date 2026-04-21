package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// The Node sidecar that loads user hook files and responds to invocation
// requests streamed over stdin/stdout as newline-delimited JSON. It is
// embedded into the `softprobe` binary so `softprobe suite run` is
// self-contained: no `node_modules/@softprobe/softprobe-js` required on
// the CI host, just a `node` that can run ES modules (≥ v18).
//
// See docs-site/guides/run-a-suite-at-scale.md#hooks--when-declarative-isnt-enough
// for the user-facing protocol description.
//
//go:embed sidecar/suite-sidecar.mjs
var suiteSidecarScript []byte

// hookSidecar owns the Node child process and multiplexes concurrent hook
// calls onto its single stdin/stdout pipe.
type hookSidecar struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu      sync.Mutex
	pending map[string]chan hookResponse
	nextID  int64
	closed  bool

	scriptPath string // temp file path holding the embedded sidecar

	// Disabled sidecars — no hook files passed — short-circuit every
	// invocation with an error. The user explicitly referenced a hook
	// name in their YAML; we'd rather fail loudly than silently skip.
	disabled bool
}

type hookResponse struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Ready  bool            `json:"ready,omitempty"`
	Log    string          `json:"log,omitempty"`
}

// startHookSidecar spawns the Node sidecar with the user's hook files.
// Passing an empty list returns a disabled sidecar — every `invoke` call
// on it will error with a clear "no hooks registered" message.
func startHookSidecar(hookFiles []string, stderr io.Writer) (*hookSidecar, error) {
	if len(hookFiles) == 0 {
		return &hookSidecar{disabled: true, pending: map[string]chan hookResponse{}}, nil
	}

	tmpDir, err := os.MkdirTemp("", "softprobe-sidecar-")
	if err != nil {
		return nil, fmt.Errorf("sidecar tempdir: %w", err)
	}
	scriptPath := filepath.Join(tmpDir, "suite-sidecar.mjs")
	if err := os.WriteFile(scriptPath, suiteSidecarScript, 0o600); err != nil {
		return nil, fmt.Errorf("sidecar write: %w", err)
	}

	// `--experimental-strip-types` lets Node 22+ load .ts hooks without
	// a transpiler. On older Node (18/20) the flag is silently ignored
	// and only .js/.mjs hooks will load, which we document.
	args := append([]string{"--experimental-strip-types", "--no-warnings", scriptPath}, hookFiles...)
	cmd := exec.Command("node", args...)
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn node sidecar: %w (install Node 18+ and ensure it's on PATH)", err)
	}

	s := &hookSidecar{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderrPipe,
		pending:    map[string]chan hookResponse{},
		scriptPath: scriptPath,
	}

	// Drain stderr into our stderr prefixed so users can see hook logs.
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			fmt.Fprintf(stderr, "sidecar: %s\n", scanner.Text())
		}
	}()

	// Read loop: dispatch responses to the waiting channel by id.
	go s.readLoop()

	// Block until the sidecar confirms it loaded all hook files. Failing
	// fast here turns "my hook file has a syntax error" into a nice
	// upfront message instead of a cryptic timeout on the first case.
	ready := make(chan error, 1)
	go func() {
		ch := make(chan hookResponse, 1)
		s.mu.Lock()
		s.pending["ready"] = ch
		s.mu.Unlock()
		select {
		case r := <-ch:
			if r.Error != "" {
				ready <- fmt.Errorf("sidecar init: %s", r.Error)
				return
			}
			ready <- nil
		case <-time.After(10 * time.Second):
			ready <- errors.New("sidecar did not signal ready within 10s")
		}
	}()
	if err := <-ready; err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *hookSidecar) readLoop() {
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp hookResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		id := resp.ID
		if resp.Ready && id == "" {
			id = "ready"
		}
		s.mu.Lock()
		ch, ok := s.pending[id]
		if ok {
			delete(s.pending, id)
		}
		s.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
	s.mu.Lock()
	for id, ch := range s.pending {
		ch <- hookResponse{ID: id, Error: "sidecar terminated before responding"}
		delete(s.pending, id)
	}
	s.closed = true
	s.mu.Unlock()
}

// Close tears down the sidecar. Safe to call on a disabled sidecar.
func (s *hookSidecar) Close() error {
	if s == nil || s.disabled {
		return nil
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if !closed {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- s.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = s.cmd.Process.Kill()
		}
	}
	if s.scriptPath != "" {
		_ = os.Remove(s.scriptPath)
		_ = os.Remove(filepath.Dir(s.scriptPath))
	}
	return nil
}

// invoke sends one `{id, hook, kind, payload}` request to the sidecar and
// blocks until it returns. Concurrent invocations are safe — each gets
// its own reply channel keyed by id.
func (s *hookSidecar) invoke(ctx context.Context, hookName, kind string, payload any) (json.RawMessage, error) {
	if s == nil || s.disabled {
		return nil, fmt.Errorf("hook %q: no --hooks file was passed to `softprobe suite run`", hookName)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("sidecar is closed")
	}
	s.nextID++
	id := strconv.FormatInt(s.nextID, 10)
	ch := make(chan hookResponse, 1)
	s.pending[id] = ch
	s.mu.Unlock()

	raw, err := json.Marshal(map[string]any{
		"id":      id,
		"hook":    hookName,
		"kind":    kind,
		"payload": payload,
	})
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')

	s.mu.Lock()
	_, writeErr := s.stdin.Write(raw)
	s.mu.Unlock()
	if writeErr != nil {
		return nil, fmt.Errorf("write to sidecar: %w", writeErr)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != "" {
			return nil, fmt.Errorf("hook %q: %s", hookName, resp.Error)
		}
		return resp.Result, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("hook %q: timed out after 30s", hookName)
	}
}

// applyRequestHook calls a RequestHook and folds its partial return value
// into the outgoing request.
func (s *hookSidecar) applyRequestHook(ctx context.Context, hookName string, req capturedRequest, hookCtx map[string]any) (capturedRequest, error) {
	raw, err := s.invoke(ctx, hookName, "request", map[string]any{
		"request": req,
		"ctx":     hookCtx,
		"env":     envSnapshot(),
	})
	if err != nil {
		return req, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return req, nil
	}
	var patch capturedRequest
	if err := json.Unmarshal(raw, &patch); err != nil {
		return req, fmt.Errorf("hook %q: invalid return shape: %w", hookName, err)
	}
	return mergeRequest(req, patch), nil
}

// applyMockResponseHook calls a MockResponseHook and merges its partial
// return value into the captured response that was selected by findInCase.
func (s *hookSidecar) applyMockResponseHook(ctx context.Context, hookName, mockName string, captured capturedResponse, span capturedSpan, hookCtx map[string]any) (capturedResponse, error) {
	raw, err := s.invoke(ctx, hookName, "mock_response", map[string]any{
		"capturedResponse": captured,
		"capturedSpan":     span,
		"mockName":         mockName,
		"ctx":              hookCtx,
		"env":              envSnapshot(),
	})
	if err != nil {
		return captured, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return captured, nil
	}
	var patch capturedResponse
	if err := json.Unmarshal(raw, &patch); err != nil {
		return captured, fmt.Errorf("hook %q: invalid return shape: %w", hookName, err)
	}
	return mergeResponse(captured, patch), nil
}

// runBodyAssertHook calls a BodyAssertHook and returns its `Issue[]`
// converted to strings.
func (s *hookSidecar) runBodyAssertHook(ctx context.Context, hookName string, actualBody, capturedBody any, hookCtx map[string]any) ([]string, error) {
	raw, err := s.invoke(ctx, hookName, "assert_body", map[string]any{
		"actual":   actualBody,
		"captured": capturedBody,
		"ctx":      hookCtx,
		"env":      envSnapshot(),
	})
	if err != nil {
		return nil, err
	}
	return decodeHookIssues(raw, hookName)
}

// runHeadersAssertHook calls a HeadersAssertHook and returns its `Issue[]`.
func (s *hookSidecar) runHeadersAssertHook(ctx context.Context, hookName string, actualHeaders, capturedHeaders map[string]string, hookCtx map[string]any) ([]string, error) {
	raw, err := s.invoke(ctx, hookName, "assert_headers", map[string]any{
		"actual":   actualHeaders,
		"captured": capturedHeaders,
		"ctx":      hookCtx,
		"env":      envSnapshot(),
	})
	if err != nil {
		return nil, err
	}
	return decodeHookIssues(raw, hookName)
}

func decodeHookIssues(raw json.RawMessage, hookName string) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("hook %q: expected Issue[] return, got %s", hookName, string(raw))
	}
	var out []string
	for _, issue := range arr {
		if len(issue) == 0 {
			continue
		}
		msg, _ := json.Marshal(issue)
		out = append(out, string(msg))
	}
	return out, nil
}

// --- merge helpers (partial returns) ---------------------------------------

func mergeRequest(base, patch capturedRequest) capturedRequest {
	out := base
	if patch.Method != "" {
		out.Method = patch.Method
	}
	if patch.URL != "" {
		out.URL = patch.URL
	}
	if patch.Path != "" {
		out.Path = patch.Path
	}
	if patch.Host != "" {
		out.Host = patch.Host
	}
	if patch.Body != "" {
		out.Body = patch.Body
	}
	if len(patch.Headers) > 0 {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		for k, v := range patch.Headers {
			out.Headers[k] = v
		}
	}
	return out
}

// envSnapshot is the set of environment variables passed to hooks as
// `payload.env`. Mirroring the Jest adapter keeps user hook files
// portable across the CLI and the SDK (a hook that reads
// `payload.env.TEST_CARD` works from either driver).
func envSnapshot() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		idx := indexByte(kv, '=')
		if idx <= 0 {
			continue
		}
		out[kv[:idx]] = kv[idx+1:]
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func mergeResponse(base, patch capturedResponse) capturedResponse {
	out := base
	if patch.Status != 0 {
		out.Status = patch.Status
	}
	if patch.Body != "" {
		out.Body = patch.Body
	}
	if len(patch.Headers) > 0 {
		if out.Headers == nil {
			out.Headers = map[string]string{}
		}
		for k, v := range patch.Headers {
			out.Headers[k] = v
		}
	}
	return out
}
