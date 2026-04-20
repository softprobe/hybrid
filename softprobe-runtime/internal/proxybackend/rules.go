package proxybackend

import (
	"encoding/json"
	"strconv"
	"strings"
)

const strictPolicyErrorMessage = "strict policy requires a mock rule match"

type injectRuleDocument struct {
	Version int          `json:"version"`
	Rules   []injectRule `json:"rules"`
}

type injectRule struct {
	ID       string         `json:"id"`
	Priority int64          `json:"priority"`
	Consume  string         `json:"consume"`
	When     injectRuleWhen `json:"when"`
	Then     injectRuleThen `json:"then"`
}

type injectRuleWhen struct {
	Direction  string `json:"direction"`
	Service    string `json:"service"`
	Host       string `json:"host"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	PathPrefix string `json:"pathPrefix"`
}

type injectRuleThen struct {
	Action   string               `json:"action"`
	Response *injectMockResponse  `json:"response,omitempty"`
	Error    *injectErrorResponse `json:"error,omitempty"`
}

type injectMockResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

type injectErrorResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type injectRuleMatch struct {
	Layer  RuleLayer
	Order  int
	Rule   injectRule
	Source string
}

func parseInjectRulesDocument(payload []byte) ([]injectRule, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	var doc injectRuleDocument
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, err
	}
	return doc.Rules, nil
}

func caseEmbeddedRules(caseBytes []byte) []injectRule {
	if len(caseBytes) == 0 {
		return nil
	}
	var env caseFileEnvelope
	if err := json.Unmarshal(caseBytes, &env); err != nil {
		return nil
	}
	return env.Rules
}

func ruleMatchesInjectRequest(rule injectRule, req *InjectLookupRequest) bool {
	if rule.When.Direction != "" && rule.When.Direction != req.TrafficDirection {
		return false
	}
	if rule.When.Service != "" && rule.When.Service != req.ServiceName {
		return false
	}
	if rule.When.Host != "" && rule.When.Host != req.URLHost {
		return false
	}
	if rule.When.Method != "" && rule.When.Method != req.RequestMethod {
		return false
	}
	if rule.When.Path != "" && rule.When.Path != req.URLPath {
		return false
	}
	if rule.When.PathPrefix != "" && !strings.HasPrefix(req.URLPath, rule.When.PathPrefix) {
		return false
	}
	return true
}

func selectInjectRule(req *InjectLookupRequest, policyStrict bool, caseRules, sessionRules []injectRule) *injectRuleMatch {
	var winner *injectRuleMatch

	consider := func(layer RuleLayer, source string, rules []injectRule) {
		for i, rule := range rules {
			if !ruleMatchesInjectRequest(rule, req) {
				continue
			}
			candidate := injectRuleMatch{
				Layer:  layer,
				Order:  i,
				Rule:   rule,
				Source: source,
			}
			if winner == nil || betterInjectRule(candidate, *winner) {
				candidate := candidate
				winner = &candidate
			}
		}
	}

	consider(SessionPolicy, "policy", policyRulesForInject(policyStrict))
	consider(CaseEmbedded, "case", caseRules)
	consider(SessionRules, "session", sessionRules)

	return winner
}

func policyRulesForInject(strict bool) []injectRule {
	if !strict {
		return nil
	}

	return []injectRule{{
		ID:       "policy-strict-miss",
		Priority: 0,
		Then: injectRuleThen{
			Action: "error",
			Error: &injectErrorResponse{
				Status: 500,
				Body:   json.RawMessage(strconv.Quote(strictPolicyErrorMessage)),
			},
		},
	}}
}

func betterInjectRule(candidate, current injectRuleMatch) bool {
	if candidate.Rule.Priority != current.Rule.Priority {
		return candidate.Rule.Priority > current.Rule.Priority
	}
	if candidate.Layer != current.Layer {
		return candidate.Layer > current.Layer
	}
	return candidate.Order > current.Order
}

func buildMockResponse(rule injectRule) *MockResponse {
	if rule.Then.Response == nil {
		return nil
	}

	response := &MockResponse{
		StatusCode: rule.Then.Response.Status,
	}
	for name, value := range rule.Then.Response.Headers {
		response.Headers = append(response.Headers, [2]string{name, value})
	}
	if body := normalizeMockBody(rule.Then.Response.Body); body != "" {
		response.Body = body
	}
	if response.StatusCode == 0 {
		response.StatusCode = 200
	}
	return response
}

func buildErrorResponse(rule injectRule) (int, string) {
	if rule.Then.Error == nil {
		return httpStatusInternalServerError(), ""
	}
	status := rule.Then.Error.Status
	if status == 0 {
		status = httpStatusInternalServerError()
	}
	return status, normalizeMockBody(rule.Then.Error.Body)
}

func normalizeMockBody(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return string(raw)
	}
	compact, err := json.Marshal(data)
	if err != nil {
		return string(raw)
	}
	return string(compact)
}

func httpStatusInternalServerError() int {
	return 500
}
