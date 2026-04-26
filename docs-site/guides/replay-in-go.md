# Replay in Go

The Go flow mirrors [Replay in Jest](/guides/replay-in-jest). Same case file, same control API, same `FindInCase` + `MockOutbound` pattern — just `go test` and `net/http`.

## 1. Add the module

```bash
go get github.com/softprobe/softprobe-go@v0.5.0
```

## 2. The minimum working test

```go
// checkout_replay_test.go
package checkout_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/softprobe/softprobe-go/softprobe"
)

func TestChargesTheCapturedCard(t *testing.T) {
	appURL := envOr("APP_URL", "http://127.0.0.1:8082")

	sp := softprobe.New(softprobe.Config{})  // reads SOFTPROBE_RUNTIME_URL; defaults to https://runtime.softprobe.dev
	ctx := context.Background()

	session, err := sp.StartSession(ctx, softprobe.SessionSpec{Mode: "replay"})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	t.Cleanup(func() { _ = session.Close(ctx) })

	casePath := filepath.Join("cases", "checkout-happy-path.case.json")
	if err := session.LoadCaseFromFile(ctx, casePath); err != nil {
		t.Fatalf("load case: %v", err)
	}

	hit, err := session.FindInCase(softprobe.CaseSpanPredicate{
		Direction:  "outbound",
		Method:     "POST",
		HostSuffix: "stripe.com",
		PathPrefix: "/v1/payment_intents",
	})
	if err != nil {
		t.Fatalf("find in case: %v", err)
	}

	if err := session.MockOutbound(ctx, softprobe.MockRuleSpec{
		Direction:  "outbound",
		Method:     "POST",
		HostSuffix: "stripe.com",
		PathPrefix: "/v1/payment_intents",
		Response:   hit.Response,
	}); err != nil {
		t.Fatalf("mock outbound: %v", err)
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", appURL+"/checkout",
		strings.NewReader(`{"amount":1000,"currency":"usd"}`))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-softprobe-session-id", session.ID())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("checkout request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	var parsed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if parsed.Status != "paid" {
		t.Fatalf(`status: got %q, want "paid"`, parsed.Status)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

## 3. Run it

```bash
go test -v ./...
```

Expected:

```
=== RUN   TestChargesTheCapturedCard
--- PASS: TestChargesTheCapturedCard (0.04s)
PASS
ok      example.com/checkout      0.047s
```

## API parity

| JavaScript | Go |
|---|---|
| `new Softprobe({ baseUrl })` | `softprobe.New(softprobe.Config{BaseURL: ...})` |
| `softprobe.startSession({ mode: 'replay' })` | `sp.StartSession(ctx, softprobe.SessionSpec{Mode: "replay"})` |
| `session.loadCaseFromFile(path)` | `session.LoadCaseFromFile(ctx, path)` |
| `session.findInCase({ direction, ... })` | `session.FindInCase(softprobe.CaseSpanPredicate{...})` |
| `session.mockOutbound({ ..., response })` | `session.MockOutbound(ctx, softprobe.MockRuleSpec{..., Response: ...})` |
| `session.clearRules()` | `session.ClearRules(ctx)` |
| `session.setPolicy({ externalHttp: 'strict' })` | `session.SetPolicy(ctx, softprobe.Policy{ExternalHTTP: "strict"})` |
| `session.close()` | `session.Close(ctx)` |

Go's signature uses `context.Context` everywhere; `FindInCase` is synchronous and does not take a context (it's an in-memory operation).

## Mutating a captured response

```go
var body map[string]any
_ = json.Unmarshal([]byte(hit.Response.Body), &body)
body["servedAt"] = time.Now().UTC().Format(time.RFC3339)
mutatedBody, _ := json.Marshal(body)

_ = session.MockOutbound(ctx, softprobe.MockRuleSpec{
    Direction:  "outbound",
    HostSuffix: "stripe.com",
    Response: softprobe.CapturedResponse{
        Status:  hit.Response.Status,
        Headers: hit.Response.Headers,
        Body:    string(mutatedBody),
    },
})
```

## Parallel tests

`go test -parallel N` and `t.Parallel()` work as usual — each parallel test creates its own session.

```go
func TestAll(t *testing.T) {
    cases := []string{
        "cases/checkout-happy-path.case.json",
        "cases/checkout-declined.case.json",
    }

    for _, casePath := range cases {
        casePath := casePath
        t.Run(filepath.Base(casePath), func(t *testing.T) {
            t.Parallel()
            // ... per-session setup + assertions
        })
    }
}
```

## Next

- [Go SDK reference](/reference/sdk-go) — complete API.
- [Run a suite at scale](/guides/run-a-suite-at-scale) — for hundreds of cases.
- [CI integration](/guides/ci-integration) — running `go test` against the Softprobe stack.
