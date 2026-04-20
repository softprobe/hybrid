package proxybackend

import "testing"

func TestRuleMatchesInjectRequestCoversCoreWhenFields(t *testing.T) {
	baseReq := &InjectLookupRequest{
		ServiceName:      "checkout",
		TrafficDirection: "outbound",
		URLHost:          "api.stripe.com",
		URLPath:          "/v1/payment_intents",
		RequestMethod:    "POST",
	}

	tests := []struct {
		name string
		rule injectRule
		want bool
	}{
		{
			name: "direction match",
			rule: injectRule{When: injectRuleWhen{Direction: "outbound"}},
			want: true,
		},
		{
			name: "direction mismatch",
			rule: injectRule{When: injectRuleWhen{Direction: "inbound"}},
			want: false,
		},
		{
			name: "host match",
			rule: injectRule{When: injectRuleWhen{Host: "api.stripe.com"}},
			want: true,
		},
		{
			name: "host mismatch",
			rule: injectRule{When: injectRuleWhen{Host: "example.com"}},
			want: false,
		},
		{
			name: "method match",
			rule: injectRule{When: injectRuleWhen{Method: "POST"}},
			want: true,
		},
		{
			name: "method mismatch",
			rule: injectRule{When: injectRuleWhen{Method: "GET"}},
			want: false,
		},
		{
			name: "path match",
			rule: injectRule{When: injectRuleWhen{Path: "/v1/payment_intents"}},
			want: true,
		},
		{
			name: "path mismatch",
			rule: injectRule{When: injectRuleWhen{Path: "/health"}},
			want: false,
		},
		{
			name: "path prefix match",
			rule: injectRule{When: injectRuleWhen{PathPrefix: "/v1"}},
			want: true,
		},
		{
			name: "path prefix mismatch",
			rule: injectRule{When: injectRuleWhen{PathPrefix: "/health"}},
			want: false,
		},
		{
			name: "combined fields match",
			rule: injectRule{When: injectRuleWhen{
				Direction:  "outbound",
				Host:       "api.stripe.com",
				Method:     "POST",
				PathPrefix: "/v1",
			}},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ruleMatchesInjectRequest(tc.rule, baseReq)
			if got != tc.want {
				t.Fatalf("ruleMatchesInjectRequest() = %v, want %v", got, tc.want)
			}
		})
	}
}
