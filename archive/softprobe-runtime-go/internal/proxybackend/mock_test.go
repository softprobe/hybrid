package proxybackend

import "testing"

func TestMockResponseAttributesEmitsHttpResponseAttributes(t *testing.T) {
	attrs := MockResponseAttributes(&MockResponse{
		StatusCode: 200,
		Headers:    [][2]string{{"content-type", "application/json"}},
		Body:       "{" + `"ok":true` + "}",
	})

	if len(attrs) != 3 {
		t.Fatalf("len(attrs) = %d, want 3", len(attrs))
	}

	if attrs[0].Key != "http.response.status_code" {
		t.Fatalf("attrs[0].Key = %q, want http.response.status_code", attrs[0].Key)
	}
	if attrs[0].Value.Int == nil || *attrs[0].Value.Int != 200 {
		t.Fatalf("attrs[0].Value.Int = %v, want 200", attrs[0].Value.Int)
	}

	if attrs[1].Key != "http.response.header.content-type" {
		t.Fatalf("attrs[1].Key = %q, want http.response.header.content-type", attrs[1].Key)
	}
	if attrs[1].Value.String == nil || *attrs[1].Value.String != "application/json" {
		t.Fatalf("attrs[1].Value.String = %v, want application/json", attrs[1].Value.String)
	}

	if attrs[2].Key != "http.response.body" {
		t.Fatalf("attrs[2].Key = %q, want http.response.body", attrs[2].Key)
	}
	if attrs[2].Value.String == nil || *attrs[2].Value.String != `{"ok":true}` {
		t.Fatalf("attrs[2].Value.String = %v, want {\"ok\":true}", attrs[2].Value.String)
	}
}
