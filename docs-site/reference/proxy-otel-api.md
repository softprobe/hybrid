# Proxy OTLP API

The **proxy OTLP API** is the wire contract between the Softprobe proxy (Envoy + WASM) and the hosted runtime at `https://runtime.softprobe.dev`. It is served alongside the [HTTP control API](/reference/http-control-api), sharing the same tenant-scoped session state.

**You only need this page if you are:**

- building or auditing a custom proxy integration,
- debugging inject/extract traffic at the OTLP layer, or
- implementing a hosted backend (like `runtime.softprobe.dev`) against the same contract.

Most users interact with the runtime through [SDKs](/reference/sdk-typescript) and the [CLI](/reference/cli). Tests never call these endpoints directly.

The normative source is [`spec/protocol/proxy-otel-api.md`](https://github.com/softprobe/hybrid/blob/main/spec/protocol/proxy-otel-api.md). This page summarizes it in user-oriented form.

## Transport

- HTTP/1.1 or HTTP/2 over TCP (no streaming; request/response only).
- Payloads are **OTLP `TracesData`** — the same envelope standard OpenTelemetry SDKs produce.
- `Content-Type`: `application/x-protobuf` **or** `application/json`.
- `Accept`: `application/x-protobuf` **or** `application/json` on endpoints that return OTLP payloads.

The proxy negotiates protobuf by default; JSON is supported for ease of debugging with `curl` and for third-party proxies.

## Endpoints

There are exactly two:

| Method | Path | Purpose | Blocking? |
|---|---|---|---|
| `POST` | `/v1/inject` | Request-path lookup: should this outbound be mocked, failed, or forwarded? | Yes — on the hot path of every intercepted HTTP hop |
| `POST` | `/v1/traces` | Async upload of observed (passthrough) traffic for later replay | No — fire-and-forget from the proxy's POV |

Both endpoints share the same OTLP envelope, differing only in the `sp.span.type` attribute and the server's response semantics.

## `POST /v1/inject`

Called on every intercepted HTTP hop (ingress or egress) that carries a Softprobe session. The proxy builds an OTLP trace describing the candidate exchange and waits for the runtime's decision.

### Request body

OTLP `TracesData` containing **one** inject span. Key attributes:

| Attribute | Required | Purpose |
|---|---|---|
| `sp.span.type` | yes | Must be `"inject"` |
| `sp.session.id` | yes | Session the request belongs to (from `x-softprobe-session-id` via `tracestate`) |
| `sp.traffic.direction` | yes | `"inbound"` or `"outbound"` |
| `sp.service.name` | yes | Logical name of the service the proxy is attached to |
| `url.host` | yes | Target host (outbound) or listener host (inbound) |
| `url.path` | yes | Request path |
| `http.request.method` | yes | `GET`, `POST`, etc. |
| `http.request.header.<name>` | no | Per-header values |
| `http.request.body` | no | Request body, UTF-8 string (or base64 for binary) |

### Response

| Status | Meaning | Proxy behavior |
|---|---|---|
| `200 OK` + OTLP body | **Hit** — a rule matched, use the supplied response | Synthesize HTTP response from `http.response.*` attributes; do **not** call upstream |
| `404 Not Found` | **Miss** — no rule matched | Forward the request to the real upstream (normal proxying) |
| `5xx` / timeout | **Error** from the runtime | Apply local fallback per proxy config; strict mode returns `5xx` to the caller |

### Response body on hit

The returned OTLP span carries:

| Attribute | Required | Purpose |
|---|---|---|
| `http.response.status_code` | yes | Numeric status (200, 401, 503, …) |
| `http.response.header.<name>` | no | Per-header values |
| `http.response.body` | no | Response body, UTF-8 string (or base64 for binary) |

The proxy ignores span identity (`traceId`, `spanId`, timestamps) on the response — only attributes matter.

### Worked request example

```json
{
  "resourceSpans": [
    {
      "resource": {
        "attributes": [
          { "key": "sp.service.name", "value": { "stringValue": "checkout" } }
        ]
      },
      "scopeSpans": [
        {
          "spans": [
            {
              "traceId": "0af7651916cd43dd8448eb211c80319c",
              "spanId":  "b7ad6b7169203331",
              "name": "HTTP POST",
              "kind": 3,
              "attributes": [
                { "key": "sp.span.type",          "value": { "stringValue": "inject" } },
                { "key": "sp.session.id",         "value": { "stringValue": "sess_01H..." } },
                { "key": "sp.traffic.direction",  "value": { "stringValue": "outbound" } },
                { "key": "url.host",              "value": { "stringValue": "api.stripe.com" } },
                { "key": "url.path",              "value": { "stringValue": "/v1/payment_intents" } },
                { "key": "http.request.method",   "value": { "stringValue": "POST" } },
                { "key": "http.request.body",     "value": { "stringValue": "amount=1000&currency=usd" } }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

### Worked hit response

```json
{
  "resourceSpans": [
    {
      "scopeSpans": [
        {
          "spans": [
            {
              "attributes": [
                { "key": "http.response.status_code",      "value": { "intValue": "200" } },
                { "key": "http.response.header.content-type", "value": { "stringValue": "application/json" } },
                { "key": "http.response.body",             "value": { "stringValue": "{\"id\":\"pi_test\",\"status\":\"succeeded\"}" } }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

## `POST /v1/traces`

Called **after** the proxy forwards a passthrough request and has the real upstream's response. The proxy uploads the full request/response pair so the runtime can record it (capture mode) or ignore it (replay mode, depending on policy).

The request body uses the **standard OTLP `TracesData`** shape. In production the proxy sends these payloads **out-of-band** to **`sp_backend_url`** (the Softprobe runtime), not into your existing vendor APM pipeline by default — large `http.*.body` attributes would be truncated or rejected there. See [Proxy integration posture](https://github.com/softprobe/hybrid/blob/main/docs/proxy-integration-posture.md). (You may optionally **tee** a filtered copy to a collector in advanced setups; that is not the default install path.)

### Request body

OTLP `TracesData` containing **one or more** extract spans. Key attributes:

| Attribute | Required | Purpose |
|---|---|---|
| `sp.span.type` | yes | Must be `"extract"` |
| `sp.session.id` | yes | Session that owns this observation |
| `sp.traffic.direction` | yes | `"inbound"` or `"outbound"` |
| `sp.service.name` | yes | Service name |
| `url.host`, `url.path` | yes | Request target |
| `http.request.method` | yes | HTTP method |
| `http.request.header.<name>`, `http.request.body` | no | Request details |
| `http.response.status_code` | yes | Observed upstream status |
| `http.response.header.<name>`, `http.response.body` | no | Response details |

### Response

| Status | Meaning |
|---|---|
| `2xx` | Accepted; runtime will include in capture output |
| `4xx` | Rejected (unknown session, schema violation) — proxy logs and drops |
| `5xx` | Transient failure — proxy may retry with exponential backoff within a bounded deadline |

The proxy treats this endpoint as **fire-and-forget**: the outbound HTTP response to the original caller has already been sent when extract is uploaded. Extract upload must not extend request latency beyond the configured deadline.

## Session correlation

Session identity flows through the proxy on every hop via **W3C Trace Context**, not through a custom HTTP header:

1. The test sends `x-softprobe-session-id: <id>` on the **inbound** request.
2. The ingress proxy reads that header, encodes the session id into `tracestate` (per [session-headers.md](/reference/session-headers)), and forwards `traceparent` / `tracestate` to the app.
3. The app propagates both headers on outbound calls via OpenTelemetry.
4. The egress proxy reads `tracestate`, decodes the session id, and puts it in `sp.session.id` on every `/v1/inject` or `/v1/traces` call.

This means the runtime only ever sees session id via OTLP attribute — it never parses `x-softprobe-session-id` directly.

## Error handling

The runtime returns machine-readable errors per [`spec/schemas/session-error.response.schema.json`](https://github.com/softprobe/hybrid/blob/main/spec/schemas/session-error.response.schema.json). Common cases:

| Condition | Status | Proxy behavior |
|---|---|---|
| Unknown / closed `sp.session.id` | `404` | Forward upstream (same as "no matching rule") |
| Malformed OTLP body | `400` | Log + drop; proxy should emit a local telemetry error |
| Runtime overload / circuit-breaker | `503` | Apply local fallback per proxy config |
| Timeout exceeding proxy deadline | n/a | Proxy aborts its own call and applies local fallback |

## Performance targets

Recommended SLOs for the inject path (measured at the runtime, not the proxy):

| Metric | Target |
|---|---|
| p50 `/v1/inject` latency | < 1 ms |
| p99 `/v1/inject` latency | < 5 ms |
| Max inject throughput | Hosted-runtime capacity managed by Softprobe |

The extract path has no hard latency SLO since it's async; aim for < 100 ms p99 end-to-end to keep capture buffers small.

## Observability

Runtime implementations **should** expose the following (not normative, but strongly recommended):

- Prometheus metrics: `softprobe_inject_requests_total{outcome="hit|miss|error"}`, `softprobe_inject_latency_seconds`, `softprobe_extract_bytes_total`.
- Structured logs with `sp.session.id` for correlation.
- An `/health` endpoint for proxy liveness checks.

## See also

- [HTTP control API](/reference/http-control-api) — tests and CLI write rules via the control API; the proxy reads them via OTLP.
- [Case file schema](/reference/case-schema) — extract spans become case file entries.
- [Session headers](/reference/session-headers) — `x-softprobe-session-id` and its relationship to `tracestate`.
- [Rule schema](/reference/rule-schema) — what the runtime evaluates on each `/v1/inject`.
- [Architecture](/concepts/architecture) — how control plane and data plane share one process.
