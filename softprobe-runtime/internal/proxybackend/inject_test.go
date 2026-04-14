package proxybackend

import "testing"

func TestReplayResponseFromCaseMatchesCapturedExtractSpan(t *testing.T) {
	caseBytes := []byte(`{
		"traces": [
			{
				"resourceSpans": [
					{
						"resource": {
							"attributes": [
								{"key": "service.name", "value": {"stringValue": "checkout"}}
							]
						},
						"scopeSpans": [
							{
								"spans": [
									{
										"name": "GET /hello",
										"attributes": [
											{"key": "sp.span.type", "value": {"stringValue": "extract"}},
											{"key": "sp.service.name", "value": {"stringValue": "checkout"}},
											{"key": "sp.traffic.direction", "value": {"stringValue": "outbound"}},
											{"key": "url.host", "value": {"stringValue": "app:8081"}},
											{"key": "url.path", "value": {"stringValue": "/hello"}},
											{"key": "http.response.status_code", "value": {"intValue": 200}},
											{"key": "http.response.body", "value": {"stringValue": "{\"dep\":\"ok\",\"message\":\"hello\"}\n"}}
										]
									}
								]
							}
						]
					}
				]
			}
		]
	}`)

	req := &InjectLookupRequest{
		ServiceName:      "checkout",
		TrafficDirection: "outbound",
		URLHost:          "app:8081",
		URLPath:          "/hello",
	}

	response, ok := replayResponseFromCase(caseBytes, req)
	if !ok {
		t.Fatal("expected captured extract span to match replay lookup")
	}
	if response.StatusCode != 200 {
		t.Fatalf("response.StatusCode = %d, want 200", response.StatusCode)
	}
	if response.Body != "{\"dep\":\"ok\",\"message\":\"hello\"}\n" {
		t.Fatalf("response.Body = %q, want captured body", response.Body)
	}
}

func TestStrictExternalHTTPPolicyIsDetected(t *testing.T) {
	if !isStrictExternalHTTPPolicy([]byte(`{"externalHttp":"strict"}`)) {
		t.Fatal("expected strict externalHttp policy to be detected")
	}
	if isStrictExternalHTTPPolicy([]byte(`{"externalHttp":"allow"}`)) {
		t.Fatal("allow policy must not be treated as strict")
	}
}
