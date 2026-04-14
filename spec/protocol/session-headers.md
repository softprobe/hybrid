# Softprobe Session Headers

This document defines the initial shared request headers and how they relate to **OpenTelemetry** propagation.

## Required header (test / edge ingress)

- `x-softprobe-session-id`

Tests, browsers, and other **callers outside the mesh** send this header on **inbound** traffic to the workload so the proxy can bind traffic to a Softprobe session (see [http-control-api.md](./http-control-api.md)).

## Optional related headers

- `x-softprobe-mode`
- `x-softprobe-case-id`
- `x-softprobe-test-name`

## OpenTelemetry propagation (applications)

The **Softprobe Envoy/WASM extension** merges session correlation into **W3C Trace Context** on forwarded requests: standard **`traceparent`** (OpenTelemetry TraceContext) for trace and span identity, and **`tracestate`** only for Softprobe **`x-softprobe-session-id=…`** (plus any third-party `tracestate` entries the caller already sent). It does **not** duplicate trace identity inside `tracestate`. See `softprobe-proxy` (`inject_trace_context_headers`, `build_new_tracestate`).

**Application code MUST NOT manually copy `x-softprobe-session-id` onto outbound HTTP calls.** Propagate context with your language’s **OpenTelemetry** distribution using the default **W3C TraceContext** (and **Baggage** if you use it) propagators so **`traceparent` / `tracestate`** reach dependencies. That keeps one mechanism for trace + session correlation across hops.

## Rules

- **Test framework helpers** attach `x-softprobe-session-id` to requests **they** initiate toward the app (or ingress).
- **Applications** use **OTel propagation** for service-to-service HTTP; do not duplicate the session header by hand.
- **Proxy** continues to parse session from headers and from `tracestate` entries as implemented in `softprobe-proxy`.
- Matching in the runtime must not depend on ad hoc private headers outside this contract and the published OTel attribute mapping ([proxy-otel-api.md](./proxy-otel-api.md), `sp.session.id`).
