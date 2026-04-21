# Session headers

This page documents the HTTP headers Softprobe reads and writes. The canonical specification lives at [`spec/protocol/session-headers.md`](https://github.com/softprobe/softprobe/blob/main/spec/protocol/session-headers.md); this is the user-facing summary.

## Required inbound header

| Header | Value | Who sets it |
|---|---|---|
| `x-softprobe-session-id` | The `sessionId` returned from `POST /v1/sessions` | The test client (or `curl`/the CLI driving traffic) |

The test client sets this on the **first** request to the SUT (the ingress hop). The proxy reads it and embeds the session correlation into the W3C Trace Context it injects on forwarded requests. From that point forward, every hop in the same traced flow carries the session id **inside `tracestate`**, not in a second `x-softprobe-session-id` header.

```http
GET /checkout HTTP/1.1
Host: app.example.com
x-softprobe-session-id: sess_01H7P8Q4XYZ7...
```

## Optional inbound headers

| Header | Purpose |
|---|---|
| `x-softprobe-case-id` | If the session has loaded a specific case, this tags the traffic with the case id for downstream observability. |
| `x-softprobe-mode` | Optional override: `"capture"`, `"replay"`, `"generate"`. |
| `x-softprobe-test-name` | Free-form label for human-readable correlation in logs and reports. |

These are informational — the session is still uniquely identified by `x-softprobe-session-id`.

## Headers Softprobe writes

On ingress, the proxy rewrites / injects:

- `traceparent` (W3C Trace Context, standard form)
- `tracestate` (adds `sp=<sessionId>,<revision>`)

**The application must not manually forward `x-softprobe-session-id` on outbound calls.** The proxy strips it from outbound requests it forwards. Session correlation flows through `tracestate` instead, carried by your OpenTelemetry HTTP client.

## What the app is responsible for

- **Propagate `traceparent` and `tracestate`** on outbound HTTP. Every OpenTelemetry HTTP instrumentation does this by default.
- **Do not strip these headers** in any middleware.
- **Do not generate new `traceparent` values mid-request** — that breaks correlation.

## Language-specific hints

| Language | Recommended instrumentation |
|---|---|
| Node.js | `@opentelemetry/instrumentation-http` (auto) |
| Python | `opentelemetry-instrumentation-requests`, `opentelemetry-instrumentation-httpx` |
| Java | OpenTelemetry Java Agent with default HTTP auto-instrumentation |
| Go | `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` |
| Ruby | `opentelemetry-instrumentation-net_http` |

If you're not using OpenTelemetry today, propagate manually: read `traceparent` and `tracestate` from the incoming request, attach the same values to every outbound request.

## Debugging header propagation

### Verify the test client sets the header

```bash
curl -v -H "x-softprobe-session-id: $SESSION_ID" http://127.0.0.1:8082/checkout
# Look for: > x-softprobe-session-id: sess_...
```

### Verify the proxy received the header

```bash
docker logs softprobe-proxy-1 | grep softprobe-session
```

### Verify the proxy sets `tracestate`

```bash
# In the app container, log incoming request headers briefly:
curl -v -H "x-softprobe-session-id: $SESSION_ID" http://127.0.0.1:8082/echo-headers
# Expect: tracestate: sp=sess_01H...,<revision>
```

### Verify the app's outbound calls carry `traceparent`

```bash
# Upstream container logs:
docker logs upstream-1 | grep -i traceparent
# Each outbound call should log a traceparent value
```

If **any** of these links is missing, capture will record the hops it sees but fail to stitch them into one trace — which looks like "my egress span isn't in the case file."

## Security considerations

- **Treat `sessionId` as a capability.** Anyone who knows it can drive mock/replay behavior for that session. Scope sessions to the minimum TTL required.
- **In hosted deployments**, sessions are namespaced per org — knowing a session id from another tenant is not sufficient to access it.
- **In production canaries**, require auth on the runtime endpoints before exposing them broadly. Bearer-token support for the OSS runtime is planned contract work; hosted deployments enforce their own auth separately.

## See also

- [HTTP control API](/reference/http-control-api) — how `sessionId` is created.
- [Architecture](/concepts/architecture) — where the proxy sits and what it does with these headers.
- [Troubleshooting](/guides/troubleshooting#my-egress-mocks-arent-hit) — common propagation failures.
