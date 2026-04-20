package proxybackend

import "testing"

func TestPassthroughDecisionIsExplicit(t *testing.T) {
	if got := PassthroughDecision(); got != (Decision{Kind: DecisionPassthrough}) {
		t.Fatalf("got %+v, want passthrough decision", got)
	}
}

func TestStrictMissBecomesError(t *testing.T) {
	decision := MissDecision(true)
	if decision.Kind != DecisionError {
		t.Fatalf("kind = %v, want error", decision.Kind)
	}
	if decision.Message != "strict policy requires a mock rule match" {
		t.Fatalf("message = %q, want strict policy requires a mock rule match", decision.Message)
	}
}

func TestErrorDecisionCarriesMessage(t *testing.T) {
	got := ErrorDecision("blocked by rule")
	want := Decision{Kind: DecisionError, Message: "blocked by rule"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
