package proxybackend

import "testing"

func TestReplayConsumeOnceExhaustsAfterFirstHit(t *testing.T) {
	caseFile := ReplayCase{
		Entries: []ReplayEntry{{
			MatchKey: "checkout",
			Consume:  ConsumeOnce,
			Response: MockResponse{StatusCode: 201, Body: "created"},
		}},
	}

	first, ok := caseFile.NextResponse("checkout")
	if !ok {
		t.Fatal("first matching response should exist")
	}
	if first.StatusCode != 201 {
		t.Fatalf("first status code = %d, want 201", first.StatusCode)
	}
	if first.Body != "created" {
		t.Fatalf("first body = %q, want created", first.Body)
	}

	if _, ok := caseFile.NextResponse("checkout"); ok {
		t.Fatal("second consume-once response should be exhausted")
	}
}

func TestReplayConsumeManyRepeats(t *testing.T) {
	caseFile := ReplayCase{
		Entries: []ReplayEntry{{
			MatchKey: "checkout",
			Consume:  ConsumeMany,
			Response: MockResponse{StatusCode: 202, Body: "repeat"},
		}},
	}

	first, ok := caseFile.NextResponse("checkout")
	if !ok {
		t.Fatal("first matching response should exist")
	}
	second, ok := caseFile.NextResponse("checkout")
	if !ok {
		t.Fatal("second matching response should exist for consume many")
	}

	if first.StatusCode != 202 || second.StatusCode != 202 {
		t.Fatalf("status codes = %d, %d; want 202, 202", first.StatusCode, second.StatusCode)
	}
	if first.Body != "repeat" || second.Body != "repeat" {
		t.Fatalf("bodies = %q, %q; want repeat, repeat", first.Body, second.Body)
	}
}
