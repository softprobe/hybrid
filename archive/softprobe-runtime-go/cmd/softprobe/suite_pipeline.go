package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// runSuitePipelineCase drives one case through the full
// `softprobe suite run` loop:
//
//   1. start session         (runtime)
//   2. set policy            (runtime, optional)
//   3. load case             (runtime)
//   4. resolve mocks         (local findInCase + optional hook)
//   5. register rules        (runtime)
//   6. build request         (local, optional transform hook)
//   7. send to SUT           (appUrl)
//   8. assert                (local + optional body-assert hook)
//   9. close session         (runtime)
//
// Hooks run in a single Node sidecar shared across the suite. The
// runtime never sees the hook code; the sidecar is always in-process
// with the Go CLI.
func runSuitePipelineCase(
	ctx context.Context,
	env suitePipelineEnv,
	rc resolvedCase,
	defaults suiteDefaults,
) suiteCaseResult {
	start := time.Now()
	result := suiteCaseResult{Path: rc.Path}

	// Read and name the case up front so errors always carry the caseId.
	caseBytes, err := os.ReadFile(rc.Path)
	if err != nil {
		return failResult(result, fmt.Sprintf("read case: %v", err), start)
	}
	var top struct {
		CaseID string `json:"caseId"`
	}
	_ = json.Unmarshal(caseBytes, &top)
	result.CaseID = top.CaseID
	if result.CaseID == "" {
		result.CaseID = rc.Path
	}

	client := newHTTPClient(30 * time.Second)

	sessionID, code := suiteStartSession(client, env.RuntimeURL, "replay")
	if code != exitOK {
		return failResult(result, "runtime unavailable (session start)", start)
	}
	defer func() { _ = suiteCloseSession(client, env.RuntimeURL, sessionID) }()

	// Apply policy before the case loads so a `strict` policy is
	// enforced for the first outbound call too.
	if len(defaults.Policy) > 0 {
		if code := suitePostBytes(client, env.RuntimeURL, sessionID, "policy", defaults.Policy); code != exitOK {
			return failResult(result, "policy apply failed", start)
		}
	}

	if code := suitePostBytes(client, env.RuntimeURL, sessionID, "load-case", caseBytes); code != exitOK {
		return failResult(result, "load-case failed", start)
	}

	// findInCase needs the parsed case in memory for resolving
	// `source: case` mocks and `source: case.ingress` requests.
	spans, err := loadCaseSpans(caseBytes)
	if err != nil {
		return failResult(result, err.Error(), start)
	}

	// --- mocks -------------------------------------------------------
	rules, capturedByMock, err := resolveMocks(ctx, env, spans, defaults.Mocks, sessionID, result.CaseID)
	if err != nil {
		return failResult(result, "resolve mocks: "+err.Error(), start)
	}
	if len(rules) > 0 {
		body, _ := json.Marshal(map[string]any{"version": 1, "rules": rules})
		if code := suitePostBytes(client, env.RuntimeURL, sessionID, "rules", body); code != exitOK {
			return failResult(result, "register rules failed", start)
		}
	}

	// --- request -----------------------------------------------------
	reqSpec, err := buildRequest(ctx, env, defaults.Request, spans, sessionID, result.CaseID)
	if err != nil {
		return failResult(result, "build request: "+err.Error(), start)
	}
	if reqSpec == nil {
		// No request was declared and no ingress span was present in
		// the case. That's legitimate for "smoke" suites that just
		// assert the runtime can load the case (the e2e setup in CI
		// often looks exactly like this). Mocks and policy already ran,
		// so this is a pass, not a skip.
		result.Status = "passed"
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	actual, err := sendToSUT(client, env, sessionID, *reqSpec)
	if err != nil {
		return failResult(result, "sut call: "+err.Error(), start)
	}

	// --- assert ------------------------------------------------------
	var capturedResp *capturedResponse
	if h, ok := capturedByMock["__request__"]; ok {
		capturedResp = &h
	}
	issues := assertResponse(defaults.Assertions, actual, capturedResp)

	if defaults.Assertions != nil && defaults.Assertions.Body != nil && defaults.Assertions.Body.Custom != "" {
		hookCtx := map[string]any{"caseId": result.CaseID, "direction": "inbound", "path": reqSpec.Path}
		actualParsed, _ := parseJSONLoose(actual.Body)
		var capturedParsed any
		if capturedResp != nil {
			capturedParsed, _ = parseJSONLoose(capturedResp.Body)
		}
		hookIssues, err := env.Sidecar.runBodyAssertHook(ctx, defaults.Assertions.Body.Custom, actualParsed, capturedParsed, hookCtx)
		if err != nil {
			issues = append(issues, err.Error())
		} else {
			issues = append(issues, hookIssues...)
		}
	}
	if defaults.Assertions != nil && defaults.Assertions.Headers != nil && defaults.Assertions.Headers.Custom != "" {
		hookCtx := map[string]any{"caseId": result.CaseID, "direction": "inbound", "path": reqSpec.Path}
		var capturedHeaders map[string]string
		if capturedResp != nil {
			capturedHeaders = capturedResp.Headers
		}
		hookIssues, err := env.Sidecar.runHeadersAssertHook(ctx, defaults.Assertions.Headers.Custom, actual.Headers, capturedHeaders, hookCtx)
		if err != nil {
			issues = append(issues, err.Error())
		} else {
			issues = append(issues, hookIssues...)
		}
	}

	if len(issues) > 0 {
		result.Status = "failed"
		result.Error = strings.Join(issues, "; ")
	} else {
		result.Status = "passed"
	}
	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

type suitePipelineEnv struct {
	RuntimeURL string
	AppURL     string
	SuiteName  string
	Sidecar    *hookSidecar
}

func failResult(r suiteCaseResult, msg string, start time.Time) suiteCaseResult {
	r.Status = "failed"
	r.Error = msg
	r.DurationMs = time.Since(start).Milliseconds()
	return r
}

// resolveMocks walks the `defaults.mocks` list and produces (a) the JSON
// rules to POST to the runtime, and (b) a map from mock name → captured
// response (so the assertion phase can access the captured body for
// `source: case` assertions).
func resolveMocks(
	ctx context.Context,
	env suitePipelineEnv,
	spans []capturedSpan,
	mocks []suiteMock,
	sessionID, caseID string,
) ([]map[string]any, map[string]capturedResponse, error) {
	var rules []map[string]any
	captured := map[string]capturedResponse{}
	for i, mock := range mocks {
		var resp capturedResponse
		var span capturedSpan
		source := mock.Source
		if source == "" {
			if mock.Response != nil {
				source = "inline"
			} else {
				source = "case"
			}
		}
		switch source {
		case "case":
			pred := predicateFromSuiteMatch(mock.Match)
			hit, err := findCaseSpanOne(spans, pred)
			if err != nil {
				return nil, nil, fmt.Errorf("mocks[%d] %q: %v", i, mock.Name, err)
			}
			r, err := responseFromSpan(hit)
			if err != nil {
				return nil, nil, fmt.Errorf("mocks[%d] %q: %v", i, mock.Name, err)
			}
			resp = r
			span = hit
		case "inline":
			if mock.Response == nil {
				return nil, nil, fmt.Errorf("mocks[%d] %q: source=inline requires response:", i, mock.Name)
			}
			resp = capturedResponse{
				Status:  mock.Response.Status,
				Headers: mock.Response.Headers,
				Body:    mock.Response.Body,
			}
		default:
			return nil, nil, fmt.Errorf("mocks[%d] %q: unsupported source %q", i, mock.Name, source)
		}

		if mock.Hook != "" {
			hookCtx := map[string]any{"caseId": caseID, "sessionId": sessionID, "mockName": mock.Name}
			mutated, err := env.Sidecar.applyMockResponseHook(ctx, mock.Hook, mock.Name, resp, span, hookCtx)
			if err != nil {
				return nil, nil, fmt.Errorf("mocks[%d] %q: %v", i, mock.Name, err)
			}
			resp = mutated
		}
		captured[mock.Name] = resp

		rules = append(rules, buildRuleJSON(mock, resp))
	}
	return rules, captured, nil
}

// buildRuleJSON shapes a runtime mock rule identical to what the SDKs
// produce when calling `session.mockOutbound()`. Keeping the shapes in
// lockstep ensures the runtime treats CLI-registered rules the same way
// as SDK-registered ones.
func buildRuleJSON(mock suiteMock, resp capturedResponse) map[string]any {
	when := map[string]any{}
	if mock.Match.Direction != "" {
		when["direction"] = mock.Match.Direction
	}
	if mock.Match.Service != "" {
		when["service"] = mock.Match.Service
	}
	if mock.Match.Host != "" {
		when["host"] = mock.Match.Host
	}
	if mock.Match.HostSuffix != "" {
		when["hostSuffix"] = mock.Match.HostSuffix
	}
	if mock.Match.Method != "" {
		when["method"] = mock.Match.Method
	}
	if mock.Match.Path != "" {
		when["path"] = mock.Match.Path
	}
	if mock.Match.PathPrefix != "" {
		when["pathPrefix"] = mock.Match.PathPrefix
	}
	rule := map[string]any{
		"when": when,
		"then": map[string]any{
			"action": "mock",
			"response": map[string]any{
				"status":  resp.Status,
				"headers": resp.Headers,
				"body":    resp.Body,
			},
		},
	}
	if mock.Name != "" {
		rule["id"] = mock.Name
	}
	if mock.Priority != nil {
		rule["priority"] = *mock.Priority
	}
	if mock.Consume != "" {
		rule["consume"] = mock.Consume
	}
	return rule
}

// buildRequest materializes the HTTP request to send to the SUT. Returns
// `nil` when no request can be built (no inline fields AND no ingress
// span), which the caller translates to `status: skipped`.
func buildRequest(
	ctx context.Context,
	env suitePipelineEnv,
	spec *suiteRequest,
	spans []capturedSpan,
	sessionID, caseID string,
) (*capturedRequest, error) {
	var req capturedRequest
	var source string
	if spec != nil {
		source = spec.Source
	}

	// Explicit inline fields dominate when the user passes both source:
	// and method/path/… so a single suite can declare "use the captured
	// ingress but override the path".
	if source == "case.ingress" || (source == "" && spec != nil && spec.Method == "" && spec.Path == "") {
		hit := findIngressSpan(spans)
		if hit != nil {
			req = requestFromSpan(*hit)
		} else if source == "case.ingress" {
			return nil, fmt.Errorf("request.source=case.ingress but no inbound span present in case")
		}
	}
	if spec != nil {
		if spec.Method != "" {
			req.Method = spec.Method
		}
		if spec.Path != "" {
			req.Path = spec.Path
		}
		if len(spec.Headers) > 0 {
			if req.Headers == nil {
				req.Headers = map[string]string{}
			}
			for k, v := range spec.Headers {
				req.Headers[k] = v
			}
		}
		if spec.Body != "" {
			req.Body = spec.Body
		}
		if spec.URL != "" {
			req.URL = spec.URL
		}
	}

	if req.Method == "" && req.Path == "" {
		return nil, nil
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	if req.Headers == nil {
		req.Headers = map[string]string{}
	}

	if spec != nil && spec.Transform != "" {
		hookCtx := map[string]any{"caseId": caseID, "sessionId": sessionID}
		mutated, err := env.Sidecar.applyRequestHook(ctx, spec.Transform, req, hookCtx)
		if err != nil {
			return nil, err
		}
		req = mutated
	}
	return &req, nil
}

func findIngressSpan(spans []capturedSpan) *capturedSpan {
	for i, sp := range spans {
		if readOTLPString(sp.Attributes, "sp.span.type") == "extract" &&
			readOTLPString(sp.Attributes, "sp.traffic.direction") == "inbound" {
			return &spans[i]
		}
	}
	return nil
}

// sendToSUT issues the HTTP request and returns the observed response.
// The session id is always forwarded so the runtime can tie the call
// back to the loaded case, mirroring the SDK adapter's behavior.
func sendToSUT(client *http.Client, env suitePipelineEnv, sessionID string, req capturedRequest) (sutResponse, error) {
	target := req.URL
	if target == "" {
		if env.AppURL == "" {
			return sutResponse{}, fmt.Errorf("no target URL: pass --app-url or set request.url in the suite")
		}
		target = strings.TrimRight(env.AppURL, "/") + "/" + strings.TrimLeft(req.Path, "/")
	} else if req.Path != "" && !strings.Contains(target, req.Path) {
		target = strings.TrimRight(target, "/") + "/" + strings.TrimLeft(req.Path, "/")
	}

	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = bytes.NewBufferString(req.Body)
	}
	method := req.Method
	if method == "" {
		method = "GET"
	}
	httpReq, err := http.NewRequest(method, target, bodyReader)
	if err != nil {
		return sutResponse{}, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("x-softprobe-session-id", sessionID)

	resp, err := client.Do(httpReq)
	if err != nil {
		return sutResponse{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	headers := map[string]string{}
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return sutResponse{Status: resp.StatusCode, Headers: headers, Body: string(body)}, nil
}
