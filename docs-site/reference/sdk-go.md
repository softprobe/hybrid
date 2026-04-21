# Go SDK reference

The `softprobe-go` Go module. Requires Go 1.22+.

::: warning Not yet released at a public import path
The `go get github.com/softprobe/softprobe-go@v0.5.0` command below refers to
a **planned** release. The module in this monorepo currently has the local
module name `softprobe-go` and is consumed from source via a `replace`
directive (see `softprobe-go/README.md` and the in-repo `e2e/go/` harness).
:::

## Install

```bash
# Planned — not yet published.
go get github.com/softprobe/softprobe-go@v0.5.0
```

```go
import "github.com/softprobe/softprobe-go/softprobe"
```

## `softprobe.New`

```go
sp := softprobe.New(softprobe.Config{
    BaseURL: "http://127.0.0.1:8080",
    Timeout: 5 * time.Second,
})
```

Falls back to `$SOFTPROBE_RUNTIME_URL` if `BaseURL` is empty.

| Field | Type | Default | Purpose |
|---|---|---|---|
| `BaseURL` | string | env or `127.0.0.1:8080` | Runtime URL |
| `Timeout` | `time.Duration` | `5s` | Control-plane HTTP timeout |
| `HTTPClient` | `*http.Client` | `http.DefaultClient` | Override for proxies / TLS tuning |

## `SoftprobeSession`

Methods take a `context.Context` wherever they make a network call.

```go
ctx := context.Background()

session, err := sp.StartSession(ctx, softprobe.SessionSpec{Mode: "replay"})
if err != nil { /* ... */ }
defer session.Close(ctx)

if err := session.LoadCaseFromFile(ctx, "cases/checkout.case.json"); err != nil {
    // ...
}

hit, err := session.FindInCase(softprobe.CaseSpanPredicate{
    Direction:  "outbound",
    Method:     "POST",
    HostSuffix: "stripe.com",
    PathPrefix: "/v1/payment_intents",
})
if err != nil {
    // ...
}

err = session.MockOutbound(ctx, softprobe.MockRuleSpec{
    Direction:  "outbound",
    HostSuffix: "stripe.com",
    PathPrefix: "/v1/payment_intents",
    Response:   hit.Response,
})
```

### Method table

| Method | Returns |
|---|---|
| `session.ID() string` | session id |
| `session.LoadCaseFromFile(ctx, path) error` | |
| `session.LoadCase(ctx, doc CaseDocument) error` | |
| `session.FindInCase(predicate) (CapturedHit, error)` | **synchronous**, no context |
| `session.FindAllInCase(predicate) []CapturedHit` | non-erroring variant |
| `session.MockOutbound(ctx, spec) error` | |
| `session.ClearRules(ctx) error` | |
| `session.SetPolicy(ctx, policy) error` | |
| `session.Close(ctx) error` | |

::: warning `FindInCase` returns an error on ambiguity
`FindInCase` returns `*CaseLookupError` (via `errors.As`) when **zero** or **more than one** spans match the predicate — ambiguity is surfaced at authoring time. The error's `Matches` field lists the offending spans. Use `FindAllInCase` when multiple matches are expected.
:::

::: info `MockOutbound` merges on the client, replaces on the wire
The runtime's `POST /v1/sessions/{id}/rules` **replaces** the whole rules document. The SDK keeps a local merged list so consecutive `MockOutbound` calls accumulate. Call `session.ClearRules(ctx)` to reset.
:::

## Structs

### `CaseSpanPredicate`

```go
type CaseSpanPredicate struct {
    Direction  string // "inbound" | "outbound"
    Method     string
    Host       string
    HostSuffix string
    Path       string
    PathPrefix string
    Service    string
}
```

### `CapturedResponse`

```go
type CapturedResponse struct {
    Status  int
    Headers map[string]string
    Body    string
}
```

### `CapturedHit`

```go
type CapturedHit struct {
    Response CapturedResponse
    Span     map[string]any  // raw OTLP span as JSON
}
```

### `MockRuleSpec`

```go
type MockRuleSpec struct {
    ID         string
    Priority   int
    Consume    string   // "once" | "many"
    LatencyMs  int
    Direction  string
    Method     string
    Host       string
    HostSuffix string
    Path       string
    PathPrefix string
    Response   CapturedResponse
}
```

### `Policy`

```go
type Policy struct {
    ExternalHTTP      string   // "strict" | "allow"
    ExternalAllowlist []string
    DefaultOnMiss     string   // "error" | "passthrough" | "mock"
}
```

## Errors

All SDK errors implement the standard `error` interface. Use `errors.As` to recover typed information.

### Error catalog

| Condition | Type | Typical cause | Recovery |
|---|---|---|---|
| **Runtime unreachable** | `*RuntimeError` (wraps `*net.OpError` / `context.DeadlineExceeded`) | Runtime not running, wrong `BaseURL`, firewall | Start the runtime; `softprobe doctor` |
| **Unknown session** | `*RuntimeError` with `.Status == 404` | Session closed, wrong id | Start a fresh session |
| **Strict miss** (proxy returns error to app) | Not an SDK error — surfaces as an HTTP error in the SUT | Missing `MockOutbound` | Add the rule; see [Debug strict miss](/guides/troubleshooting#_403-forbidden-on-outbound-under-strict-policy) |
| **Invalid rule payload** | `*RuntimeError` with `.Status == 400` | Rule body doesn't validate against [rule-schema](/reference/rule-schema) | Fix the spec |
| **`FindInCase` zero matches** | `*CaseLookupError` with `len(e.Matches) == 0` | Predicate too narrow | Relax predicate; re-capture |
| **`FindInCase` multiple matches** | `*CaseLookupError` with `len(e.Matches) > 1` | Predicate too broad | Narrow predicate; use `FindAllInCase` |

### Example

```go
import (
    "errors"
    "fmt"
    "github.com/softprobe/softprobe-go/softprobe"
)

_, err := session.FindInCase(softprobe.CaseSpanPredicate{
    Direction:  "outbound",
    HostSuffix: "stripe.com",
})

var lookupErr *softprobe.CaseLookupError
var runtimeErr *softprobe.RuntimeError
var loadErr *softprobe.CaseLoadError

switch {
case errors.As(err, &lookupErr):
    ids := make([]string, 0, len(lookupErr.Matches))
    for _, m := range lookupErr.Matches {
        ids = append(ids, m.SpanID)
    }
    fmt.Printf("findInCase: %d matches: %v\n", len(lookupErr.Matches), ids)
case errors.As(err, &runtimeErr):
    fmt.Printf("runtime %d at %s: %s\n", runtimeErr.Status, runtimeErr.URL, runtimeErr.Body)
case errors.As(err, &loadErr):
    fmt.Printf("case load failed: %s: %v\n", loadErr.Path, loadErr)
case err != nil:
    return err
}
```

### Types

| Type | Fields | When returned |
|---|---|---|
| `*RuntimeError` | `Status int`, `Body string`, `URL string` | Runtime returned non-2xx |
| `*CaseLookupError` | `Matches []Span`, `Predicate CaseSpanPredicate` | `FindInCase` saw 0 or >1 matches |
| `*CaseLoadError` | `Path string` | `LoadCaseFromFile` failed to parse / validate |

## Testing helper

For ergonomic `go test` integration, use the `softprobetest` subpackage:

```go
import "github.com/softprobe/softprobe-go/softprobetest"

func TestCheckout(t *testing.T) {
    session := softprobetest.StartSession(t, "replay")
    session.LoadCaseFromFile(t, "cases/checkout.case.json")
    // ...
}
```

The helper calls `t.Cleanup(session.Close)` automatically and uses `t.Fatal` on errors — less boilerplate in the common case.

## Parallel tests

```go
func TestCases(t *testing.T) {
    for _, p := range []string{"cases/a.case.json", "cases/b.case.json"} {
        p := p
        t.Run(filepath.Base(p), func(t *testing.T) {
            t.Parallel()
            session := softprobetest.StartSession(t, "replay")
            session.LoadCaseFromFile(t, p)
            // ...
        })
    }
}
```

The runtime handles hundreds of concurrent sessions comfortably.

## Logging

The SDK uses `log/slog`. To enable:

```go
softprobe.SetLogger(slog.Default())
```

Or env: `SOFTPROBE_LOG=debug go test ./...`.

## See also

- [Replay in Go](/guides/replay-in-go) — tutorial
- [HTTP control API](/reference/http-control-api) — wire-level spec
