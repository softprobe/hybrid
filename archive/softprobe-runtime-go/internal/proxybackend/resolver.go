package proxybackend

// RuleLayer defines the precedence layer for an inject decision candidate.
type RuleLayer int

const (
	SessionPolicy RuleLayer = iota
	CaseEmbedded
	SessionRules
)

// Rule is the minimal resolution input used by the composition order logic.
type Rule struct {
	ID       string
	Priority int64
	Layer    RuleLayer
	Order    int
}

// AttributeValue is a minimal OTLP-style attribute value used by mock responses.
type AttributeValue struct {
	Int    *int64
	String *string
}

// Attribute is a minimal OTLP-style attribute key/value pair.
type Attribute struct {
	Key   string
	Value AttributeValue
}

// MockResponse is the minimal mock payload emitted by `then.action = mock`.
type MockResponse struct {
	StatusCode int
	Headers    [][2]string
	Body       string
}

// ConsumeMode controls whether a replay entry is consumed once or repeatedly.
type ConsumeMode int

const (
	ConsumeOnce ConsumeMode = iota
	ConsumeMany
)

// ReplayEntry is an ordered replay candidate from a loaded case.
type ReplayEntry struct {
	MatchKey string
	Consume  ConsumeMode
	Response MockResponse
	consumed bool
}

// ReplayCase is an ordered collection of replay entries.
type ReplayCase struct {
	Entries []ReplayEntry
}

// DecisionKind names the explicit fallback decision.
type DecisionKind int

const (
	DecisionPassthrough DecisionKind = iota
	DecisionError
)

// Decision is the explicit fallback result for a miss or policy match.
type Decision struct {
	Kind    DecisionKind
	Message string
}

// ResolveWinner returns the highest-precedence rule across the three layers.
//
// Ordering is total and stable:
// 1. Higher priority wins.
// 2. On equal priority, later layers win: session policy < case embedded < session rules.
// 3. On equal priority within a layer, later entries win.
func ResolveWinner(policyRules, caseRules, sessionRules []Rule) *Rule {
	var winner *Rule

	for _, candidate := range append(append(policyRules, caseRules...), sessionRules...) {
		candidate := candidate
		if winner == nil || betterRule(candidate, *winner) {
			winner = &candidate
		}
	}

	return winner
}

func betterRule(candidate, current Rule) bool {
	if candidate.Priority != current.Priority {
		return candidate.Priority > current.Priority
	}
	if candidate.Layer != current.Layer {
		return candidate.Layer > current.Layer
	}
	return candidate.Order > current.Order
}

// MockResponseAttributes converts a mock response into OTLP-style response attributes.
func MockResponseAttributes(response *MockResponse) []Attribute {
	if response == nil {
		return nil
	}

	status := int64(response.StatusCode)
	attrs := []Attribute{{
		Key: "http.response.status_code",
		Value: AttributeValue{
			Int: &status,
		},
	}}

	for _, header := range response.Headers {
		name := header[0]
		value := header[1]
		attrs = append(attrs, Attribute{
			Key: "http.response.header." + name,
			Value: AttributeValue{
				String: &value,
			},
		})
	}

	if response.Body != "" {
		body := response.Body
		attrs = append(attrs, Attribute{
			Key: "http.response.body",
			Value: AttributeValue{
				String: &body,
			},
		})
	}

	return attrs
}

// NextResponse returns the next matching replay response, respecting consume mode.
func (c *ReplayCase) NextResponse(matchKey string) (MockResponse, bool) {
	for i := range c.Entries {
		entry := &c.Entries[i]
		if entry.MatchKey != matchKey {
			continue
		}

		switch entry.Consume {
		case ConsumeMany:
			return entry.Response, true
		case ConsumeOnce:
			if entry.consumed {
				continue
			}
			entry.consumed = true
			return entry.Response, true
		}
	}

	return MockResponse{}, false
}

// PassthroughDecision returns the explicit passthrough decision.
func PassthroughDecision() Decision {
	return Decision{Kind: DecisionPassthrough}
}

// ErrorDecision returns an explicit error decision with a message.
func ErrorDecision(message string) Decision {
	return Decision{Kind: DecisionError, Message: message}
}

// MissDecision returns the fallback decision for strict or permissive policy.
func MissDecision(strictPolicy bool) Decision {
	if strictPolicy {
		return ErrorDecision("strict policy requires a mock rule match")
	}
	return PassthroughDecision()
}
