package proxybackend

import "testing"

func TestResolveWinnerPrefersLaterLayerOnEqualPriority(t *testing.T) {
	selected := ResolveWinner(
		[]Rule{{ID: "policy-default", Priority: 10, Layer: SessionPolicy, Order: 0}},
		[]Rule{{ID: "case-replay", Priority: 50, Layer: CaseEmbedded, Order: 0}},
		[]Rule{
			{ID: "session-overrides-case", Priority: 50, Layer: SessionRules, Order: 0},
			{ID: "session-later-entry", Priority: 50, Layer: SessionRules, Order: 1},
		},
	)

	if selected == nil {
		t.Fatal("selected rule is nil")
	}
	if selected.ID != "session-later-entry" {
		t.Fatalf("selected rule = %q, want session-later-entry", selected.ID)
	}
}

func TestResolveWinnerPrefersHigherPriorityEvenFromLowerLayer(t *testing.T) {
	selected := ResolveWinner(
		[]Rule{{ID: "policy-default", Priority: 10, Layer: SessionPolicy, Order: 0}},
		[]Rule{{ID: "case-high", Priority: 80, Layer: CaseEmbedded, Order: 0}},
		[]Rule{{ID: "session-low", Priority: 50, Layer: SessionRules, Order: 0}},
	)

	if selected == nil {
		t.Fatal("selected rule is nil")
	}
	if selected.ID != "case-high" {
		t.Fatalf("selected rule = %q, want case-high", selected.ID)
	}
}
