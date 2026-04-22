# Architecture

This page is the **mental model** you need to debug anything in Softprobe. No CLI flags, no SDK signatures — just what-talks-to-what and why.

Softprobe supports two instrumentation placements:

- **Proxy instrumentation (canonical)**: Envoy + Softprobe WASM sits on ingress/egress.
- **Language instrumentation (Node compatibility path)**: interception runs in-process, but still uses the same runtime/session control plane.

## The topology

In the canonical deployment, Softprobe sits **under** your application, not inside it. Your app and its HTTP dependencies are unchanged; a single sidecar proxy sees every request and response on both directions.

```text
  ┌───────────────┐                                    ┌───────────────┐
  │  Test client  │                                    │   HTTP        │
  │ (Jest/pytest/ │                                    │   dependency  │
  │  CLI / curl)  │                                    │ (Stripe, …)   │
  └──────┬────────┘                                    └──────▲────────┘
         │ (1) ingress                                        │ (4) egress
         ▼                                                    │
  ┌─────────────────┐   (2) forward     ┌────────────────┐   │ forward
  │ Envoy + Softprobe│ ───────────────► │  Application    │───┘
  │  WASM filter    │                   │  under test     │
  └──────┬──────────┘                   └────────────────┘
         │ (3) OTLP /v1/inject + /v1/traces
         ▼
  ┌─────────────────┐    JSON HTTP       ┌────────────────┐
  │ softprobe-      │ ◄────────────────► │ Your test code │
  │ runtime         │                    │   + CLI        │
  │ (Go service)    │                    └────────────────┘
  └─────────────────┘
         │
         ▼
  *.case.json (OTLP traces on disk)
```

Each numbered edge is one of the two HTTP flows:

1. **Ingress** — the test client hits the proxy, which forwards to the app.
2. **Egress** — the app makes an outbound call, which the same proxy intercepts.
3. **Control channel** — the proxy asks the runtime, per hop, *"is this mocked? should I capture it?"*
4. **Forward / not forward** — on miss, the proxy forwards to the real dependency. On hit (mock), it returns the canned response without touching the dependency.

::: info One proxy, two directions
In a real Istio mesh, `ingress` and `egress` are the **same** sidecar — the routing layer just invokes it twice. In the local Docker Compose harness we model this with **one** Envoy with **two** listeners (`:8082` for ingress, `:8084` for egress) to avoid iptables redirection.
:::

## Two instrumentation models, one control plane

Both models share the same runtime APIs, session lifecycle, and case schema.

| Dimension | Proxy instrumentation (canonical) | Language instrumentation (Node compatibility) |
|---|---|---|
| Interception point | Envoy/WASM on ingress + egress | In-process Node hooks/interceptors |
| Test authoring APIs | `startSession` / `loadCaseFromFile` / `mockOutbound` / `close` | Same |
| Runtime | Required | Required |
| Session semantics | `x-softprobe-session-id` + runtime session state | Same runtime session state; app wiring may differ |
| Capture artifact | `*.case.json` | `*.case.json` |
| Recommended for new deployments | Yes | Only when sidecar proxying is not available yet |

For a side-by-side setup walkthrough, see [Proxy vs language instrumentation](/guides/proxy-vs-language-instrumentation).

## The four moving parts

### 1. The application under test (SUT)

Ordinary HTTP service. It knows nothing about Softprobe. The only requirement is that **outbound** HTTP calls propagate standard W3C `traceparent` / `tracestate` — which any OpenTelemetry HTTP client does by default.

In proxy mode, you do not add Softprobe imports, mock wrappers, or test hooks to your application code.

In language instrumentation mode, Node apps load `@softprobe/softprobe-js/init` first; optional framework auto-patches are opt-in via `@softprobe/softprobe-js/legacy`.

### 2. The proxy (data plane)

**Envoy** with the **Softprobe WASM filter**. It does three things:

- Observes ingress and egress HTTP headers, bodies, and status codes.
- For every hop, issues `POST /v1/inject` to the runtime with an OTLP span describing the request. The runtime answers `200` (mock hit — use this response) or `404` (miss — forward to the real upstream).
- Asynchronously ships observed exchanges to the runtime via `POST /v1/traces` for capture.

The proxy is **deliberately dumb**. It doesn't know what a "case" is, what a "rule" is, or what session the traffic belongs to beyond what's in the OTLP span attributes. All policy lives in the runtime.

### 3. The runtime (control plane + OTLP handler)

**One Go binary.** It serves two API surfaces from the same process, backed by a single in-memory session store:

| Surface | Called by | Spec |
|---|---|---|
| HTTP control API (JSON) | tests, CLI, SDKs | [`http-control-api.md`](/reference/http-control-api) |
| OTLP trace API | proxy only | [`proxy-otel-api.md`](https://github.com/softprobe/softprobe/blob/main/spec/protocol/proxy-otel-api.md) |

Because both handlers read from the same store, any rule registered by a test is visible to the proxy on the **very next** inject lookup — no cache, no sync, no database (v1).

The runtime's responsibilities:

- Own **session** state (`sessionId`, `sessionRevision`, mode, policy, rules, loaded case bytes).
- Match OTLP inject spans against stored rules and return `200 + response attrs` on hit or `404` on miss.
- Buffer extracted spans during capture mode and flush them to a `*.case.json` file on session close.

The runtime is **not** a routing control plane. It does not push config to Envoy. It reacts to the proxy's OTLP requests, nothing more.

### 4. The SDKs and the CLI

Both are clients of the runtime's HTTP control API. They never speak OTLP to the proxy.

- **SDKs** (`softprobe-js`, `softprobe-python`, `softprobe-java`, `softprobe-go`) expose ergonomic test-authoring APIs (`findInCase`, `mockOutbound`, `loadCaseFromFile`, `clearRules`, `close`). They compile those calls into JSON payloads against `/v1/sessions/{id}/...`.
- **CLI** (`softprobe`) is a single static Go binary. It is language-agnostic and is the preferred interface for humans, CI pipelines, and AI agents. It covers orchestration (sessions, suites, capture, export, doctor) without you writing code.

## Control plane vs. data plane

```text
       ┌─────────────────────────────────────────────┐
       │  Control plane                              │
       │  ─────────────                              │
       │   Tests, CLI, SDKs ─── JSON HTTP ───► Runtime
       │   (sessions, load-case, rules, policy)      │
       └─────────────────────────────────────────────┘
                          │ (shared in-memory store)
       ┌──────────────────┴──────────────────────────┐
       │  Data plane                                 │
       │  ──────────                                 │
       │   Proxy ───── OTLP /v1/inject + /v1/traces ───► Runtime
       │   (per-request lookup; async capture)       │
       └─────────────────────────────────────────────┘
```

**Three invariants** to remember:

1. **Tests never call `/v1/inject` directly.** Only the proxy does.
2. **The proxy never calls the control API on the request path.** Only OTLP.
3. **Both halves share one in-memory store.** A rule posted by a test is visible to the proxy's next inject call with no latency.

## The session model

A **session** is a test-time scope. Creating a session gives you a UUID (`sessionId`) that you attach to HTTP requests via `x-softprobe-session-id`. The runtime uses that id to look up the session's policy, rules, and loaded case when the proxy asks.

```text
POST /v1/sessions           → session created, sessionRevision = 1
POST /v1/sessions/$ID/load-case  → case loaded,   sessionRevision = 2
POST /v1/sessions/$ID/rules  → rules replaced,   sessionRevision = 3
                               ─── proxy sees rev 3 on the next /v1/inject ───
POST /v1/sessions/$ID/close → state deleted
```

Every mutating control call bumps `sessionRevision`. Proxy-side inject caches, if any, key on `(sessionId, sessionRevision, requestFingerprint)` so stale hits can never survive a rule change.

**Session lifetime and teardown**: always call `close()` in an `afterAll` (Jest / JUnit) or `teardown` (pytest / `go test`). Closing removes all session state from the runtime. Between cases, you can call `clearRules()` to drop just the mock rules while keeping the session alive.

See [Sessions and cases](/concepts/sessions-and-cases) for the full lifecycle.

### Proxy inject cache (optional, `sessionRevision`-keyed)

To keep the inject hot path fast under load, a proxy implementation **may** cache inject decisions locally. The cache is a per-proxy-instance dictionary; it is **not** part of the runtime.

**If a proxy implements this cache, it MUST:**

1. Key each entry on the tuple `(sessionId, sessionRevision, requestFingerprint)`. The fingerprint is implementation-defined but typically includes method, host, path, and normalized body hash.
2. Treat any entry whose `sessionRevision` does not match the current session's revision as **invalid** (ignore, do not serve).
3. Bump the fetched revision on every `/v1/inject` response — the runtime echoes `sessionRevision` in the inject response's OTLP attributes when it cares to.
4. Honor `close` (session delete) by dropping all cache entries for that `sessionId`.

Because `clearRules()`, `mockOutbound()`, `loadCaseFromFile()`, and `setPolicy()` all bump the revision, authors get strong guarantees: **after any rule change, the next `/v1/inject` either returns the fresh decision or a cache miss (never a stale hit)**.

The OSS reference proxy (Envoy + Rust WASM) does **not** cache inject decisions; every hop is a fresh runtime lookup. Hosted deployments may enable caching for throughput. See [Proxy OTLP API](/reference/proxy-otel-api) for the wire contract.

### Trace context propagation (critical)

Softprobe relies on **standard W3C Trace Context** — `traceparent` and `tracestate` — to correlate inbound test requests with the outbound calls your app makes. The chain:

1. The test sends `x-softprobe-session-id: <id>` on the request to the app.
2. The ingress proxy reads that header and writes the session id into `tracestate` (per [session-headers](/reference/session-headers)).
3. The app receives `traceparent` + `tracestate` like any other instrumented service.
4. When the app makes an outbound call, its OpenTelemetry HTTP client propagates **both** headers.
5. The egress proxy reads `tracestate`, decodes the session id, and includes it in `sp.session.id` on `/v1/inject`.

::: warning This is the #1 integration risk
If outbound OpenTelemetry propagation is broken (missing instrumentation, wrong propagator config, a plain `fetch` that drops headers), the egress proxy **will not see the session id** and will treat every outbound call as an untagged, un-mocked request. Typical symptoms: mocks never fire on egress, tests hit real upstreams in "replay" mode, strict policy never triggers.

To verify, run a capture session and inspect the resulting case file — if you see ingress hops but not egress hops, propagation is broken. See the [troubleshooting guide](/guides/troubleshooting#my-egress-mocks-arent-hit) for specifics per language.
:::

## The capture artifact

A **case file** is one JSON document on disk, typically named `cases/<scenario>.case.json`. Its top level looks like:

```json
{
  "version": "1.0.0",
  "caseId": "checkout-happy-path",
  "createdAt": "2026-04-15T10:00:00Z",
  "traces": [ /* array of OTLP ExportTraceServiceRequest payloads */ ],
  "rules":   [ /* optional: ship with default rules */ ],
  "fixtures":[ /* optional: auth tokens, metadata */ ]
}
```

Each entry in `traces[]` is an OTLP-compatible JSON trace describing a single HTTP hop (one request + its response). The schema is defined in [`spec/schemas/case.schema.json`](/reference/case-schema). You can export case-shaped traces to an OpenTelemetry collector for analysis when you choose (for example via `softprobe export otlp` when available). Separately, the **Envoy WASM** filter sends live capture OTLP to **Softprobe runtime** at **`sp_backend_url`** by default — that stream is **out-of-band** from your production APM; see [Proxy integration posture](https://github.com/softprobe/hybrid/blob/main/docs/proxy-integration-posture.md).

Because the file is plain JSON, you can:

- diff two captures in a code review,
- edit a span by hand (e.g. to redact a token),
- regenerate cases from an LLM prompt,
- ship example cases in `spec/examples/cases/` for tutorials.

## Where decisions are made

One design principle drives the rest of the system: **the proxy is a dumb mirror; the runtime is a dumb interpreter; the SDK is where cleverness lives.**

| Decision | Made by | Why |
|---|---|---|
| Is the incoming HTTP traffic tagged with a session? | Proxy | It's the only thing that sees the header. |
| Is there a matching rule for this hop? | Runtime | It stores the rules. |
| What response bytes should the mock return? | SDK (at authoring time) | The test author is the only one who knows what the test needs. |
| Is this outbound call allowed under strict policy? | Runtime | Policy is just a synthesized rule. |
| Should `sessionRevision` bump? | Runtime | It owns the store. |

Replay selection used to happen in the runtime ("walk traces to find the next matching response"). **It no longer does.** The SDK's `findInCase` runs a synchronous, in-memory lookup over the loaded case and produces a concrete `response` that the SDK hands to the runtime as an explicit `mock` rule. This keeps the runtime deterministic and the SDK expressive — you can mutate the captured response before mocking (bump a timestamp, swap a test card, fix a date).

## Why proxy-first, not framework-patching?

Softprobe's predecessor monkey-patched Node.js HTTP clients. Every new framework (Fastify, Koa, Undici, Postgres driver, Redis client) was a new patch to write and maintain — and none of it worked for Python or Java.

Moving interception below the app swaps that cost for a modest one-time operational setup:

| Approach | Upfront cost | Ongoing cost | Cross-language? |
|---|---|---|---|
| Framework patches | Low | **High** (every dependency upgrade) | No |
| Proxy-first (Softprobe) | Moderate (run a sidecar) | **Low** | Yes |

If your team already runs Istio or Linkerd, the upfront cost is near-zero — you add a `WasmPlugin` to your existing mesh config.

---

**Next:** [Sessions and cases →](/concepts/sessions-and-cases)
