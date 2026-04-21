# Go SDK reference

The `github.com/softprobe/softprobe-go` Go module. Requires Go 1.22+.

## Install

```bash
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

Typed errors you may want to type-assert:

```go
import "errors"

_, err := session.FindInCase(softprobe.CaseSpanPredicate{...})

var lookupErr *softprobe.CaseLookupError
if errors.As(err, &lookupErr) {
    fmt.Printf("expected 1 match, got %d\n", len(lookupErr.Matches))
}

var runtimeErr *softprobe.RuntimeError
if errors.As(err, &runtimeErr) {
    fmt.Printf("runtime returned %d: %s\n", runtimeErr.Status, runtimeErr.Body)
}
```

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
