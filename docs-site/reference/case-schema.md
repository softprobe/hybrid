# Case file schema

A case file is one JSON document describing a single test scenario — its captured HTTP hops (as OTLP traces) plus optional rules and fixtures. This page is the user-facing summary; the canonical schema is [`spec/schemas/case.schema.json`](https://github.com/softprobe/softprobe/blob/main/spec/schemas/case.schema.json).

## Top-level shape

```json
{
  "version": "1.0.0",
  "caseId": "checkout-happy-path",
  "suite": "payments",
  "mode": "replay",
  "createdAt": "2026-04-20T10:00:00Z",
  "traces":   [ /* OTLP ExportTraceServiceRequest payloads */ ],
  "rules":    [ /* optional default rules */ ],
  "fixtures": [ /* optional fixtures */ ]
}
```

| Field | Type | Required | Purpose |
|---|---|---|---|
| `version` | string (semver) | yes | Case schema version |
| `caseId` | string | yes | Stable identifier (for diffing and cross-referencing) |
| `suite` | string | no | Logical grouping (free-form) |
| `mode` | `"capture"` / `"replay"` / `"generate"` | no | Suggested mode when this case is loaded |
| `createdAt` | RFC3339 string | yes | When the case was captured |
| `traces` | array | yes | OTLP-compatible traces (see below) |
| `rules` | array | no | Rules to apply whenever this case is loaded |
| `fixtures` | array | no | Auth tokens, service metadata |

## The `traces` array

Each element is an **OTLP ExportTraceServiceRequest** / **TracesData** JSON payload — the same shape that OpenTelemetry SDKs produce. This makes cases interoperable with any OTLP consumer (collectors, Jaeger, Tempo, Honeycomb, etc.).

```json
{
  "traces": [
    {
      "resourceSpans": [
        {
          "resource": {
            "attributes": [
              { "key": "service.name", "value": { "stringValue": "checkout-api" } }
            ]
          },
          "scopeSpans": [
            {
              "spans": [
                {
                  "traceId": "5ef...",
                  "spanId":  "01a...",
                  "parentSpanId": "fab...",
                  "name": "HTTP POST",
                  "kind": 3,
                  "startTimeUnixNano": "1713616200000000000",
                  "endTimeUnixNano":   "1713616200035000000",
                  "attributes": [
                    { "key": "sp.session.id",         "value": { "stringValue": "sess_01H..." } },
                    { "key": "sp.traffic.direction",  "value": { "stringValue": "outbound" } },
                    { "key": "http.method",           "value": { "stringValue": "POST" } },
                    { "key": "url.full",              "value": { "stringValue": "https://api.stripe.com/v1/payment_intents" } },
                    { "key": "http.status_code",      "value": { "intValue": "200" } },
                    { "key": "http.request.body",     "value": { "stringValue": "..." } },
                    { "key": "http.response.body",    "value": { "stringValue": "..." } }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

## OTLP attribute vocabulary

Case file spans use the OTLP attribute vocabulary defined in [`spec/protocol/case-otlp-json.md`](https://github.com/softprobe/softprobe/blob/main/spec/protocol/case-otlp-json.md). The same vocabulary is used on the wire in [`spec/protocol/proxy-otel-api.md`](https://github.com/softprobe/softprobe/blob/main/spec/protocol/proxy-otel-api.md), so case files can round-trip through the proxy OTLP API without reshaping.

### Softprobe identity attributes

These attributes identify the Softprobe context of a captured span and **SHOULD** be present on every span produced by the proxy:

| Attribute | OTLP value type | Required | Purpose |
|---|---|---|---|
| `sp.session.id` | `stringValue` | yes | Session that captured this span |
| `sp.traffic.direction` | `stringValue` (`"inbound"` / `"outbound"`) | yes | Which leg of the proxy |
| `sp.span.type` | `stringValue` (`"extract"` / `"inject"`) | yes | `extract` in case files; `inject` only on inject requests |
| `sp.service.name` | `stringValue` | recommended | Logical service name (may also be on `resource.attributes`) |

### HTTP identity attributes

Required on every span that represents HTTP traffic (i.e. virtually all case-file spans):

| Attribute | OTLP value type | Required | Purpose |
|---|---|---|---|
| `url.host` | `stringValue` | yes | Target host (outbound) or listener host (inbound) |
| `url.path` | `stringValue` | yes | Request path (starting with `/`) |
| `http.request.method` | `stringValue` | yes | HTTP method (`GET`, `POST`, `PUT`, …) |

### HTTP payload attributes

Present on all request/response pairs; optional only when the body/header set is legitimately empty:

| Attribute | OTLP value type | Required | Purpose |
|---|---|---|---|
| `http.request.header.<name>` | `stringValue` | no | One attribute per request header, lowercase name |
| `http.request.body` | `stringValue` | no | Request body (UTF-8 or base64 if binary) |
| `http.response.status_code` | `intValue` | yes | Response status (200, 401, 503, …) |
| `http.response.header.<name>` | `stringValue` | no | One attribute per response header, lowercase name |
| `http.response.body` | `stringValue` | no | Response body (UTF-8 or base64 if binary) |

### Legacy attribute aliases

Some tooling still emits older OpenTelemetry conventions. Softprobe treats these as equivalent during capture, but the canonical on-disk form is the vocabulary above:

| Legacy attribute | Canonical equivalent |
|---|---|
| `http.method` | `http.request.method` |
| `http.status_code` | `http.response.status_code` |
| `http.target` | `url.path` |
| `url.full` | Derived from `url.host` + `url.path` (not required to match separately) |

If you hand-author a case file, prefer the canonical attributes.

### Size guidance

For predictable diffing, validation, and AI-generated cases, v1 tooling SHOULD enforce:

- at most **100 spans** per case file,
- at most **128 attributes** per span,
- at most **64 KiB** for any string attribute value,
- at most **1 MiB** for a single trace document.

Exceeding these is not a hard error but should prompt review.

## Embedded rules

Same shape as session rules (see [Rules and policy](/concepts/rules-and-policy)). Applied by the runtime beneath any session rules, so a test can always override a case default.

```json
{
  "rules": [
    {
      "name": "redact-auth-headers",
      "priority": 10000,
      "when": { "direction": "outbound" },
      "then": {
        "action": "capture_only",
        "captureOnly": {
          "redactHeaders": ["authorization", "cookie"]
        }
      }
    }
  ]
}
```

## Embedded fixtures

Free-form key/value pairs the runtime surfaces to matchers or codegen.

```json
{
  "fixtures": [
    { "name": "auth_token", "value": "stub_eyJ..." }
  ]
}
```

## File naming convention

- `cases/<scenario>.case.json` — single scenarios.
- `cases/<suite>/<scenario>.case.json` — grouped.
- Names by **business scenario**, not by test: `checkout-happy-path`, `checkout-declined-card`, not `test_27`.

## Size guidance

Case files are plain JSON. A typical HTTP session (5–10 hops with modest JSON bodies) produces a 5–20 KB file. Very large captures (> 5 MB) are a sign that multiple unrelated scenarios got rolled into one — split them.

## Validation

```bash
softprobe validate case cases/checkout.case.json
```

Exit code 0 = valid. Failures print JSON Schema validation errors with the offending path.

## Tools that understand case files

- **`softprobe inspect case`** — pretty-print spans.
- **`softprobe export otlp`** — stream to an OTLP collector.
- **`jq`** — any JSON tool works on case files.
- **OpenTelemetry Collector** — ingest case traces directly for observability.
- **[case-diff](https://github.com/softprobe/case-diff)** — semantic diff tool (planned).

## Generating cases from scratch

While cases are most often produced by capture, they can also be hand-written or produced by an AI agent prompted with the schema. Hand-written cases validate against the same schema and run through the same replay path.

## See also

- [Sessions and cases](/concepts/sessions-and-cases) — the mental model.
- [Rules and policy](/concepts/rules-and-policy) — `rules` shape.
- [Capture your first session](/guides/capture-your-first-session) — producing real cases.
