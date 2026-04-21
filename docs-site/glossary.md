# Glossary

Short definitions for the terms that appear across this documentation. Each term links to the primary concept page where it's explained in context.

---

## Author-time vs request-time

The invariant that **all case-file lookup happens in the SDK, at authoring time**, and that the runtime never walks `traces[]` on the inject hot path. See [Capture & replay — replay mode](/concepts/capture-and-replay#replay-mode) and [Rules & policy](/concepts/rules-and-policy).

## Capture

A session `mode` that records every ingress and egress HTTP hop as OTLP spans, which are flushed into a `.case.json` file when the session closes. See [Capture & replay](/concepts/capture-and-replay).

## Case

One JSON document containing metadata, an OTLP-shaped `traces[]` array, and optional embedded `rules[]` and `fixtures[]`. The on-disk artifact a capture session produces and a replay session consumes. See [Sessions & cases](/concepts/sessions-and-cases#case) and [Case file schema](/reference/case-schema).

## `capture_only`

A `then.action` that records live traffic for a hop without mocking it — the proxy still forwards the request upstream and captures what happens. Useful for observing traffic patterns without changing behavior. See [Mock an external dependency — observe-only](/guides/mock-external-dependency#observe-only-capture-only-rules).

## Egress / egress proxy

The outbound hop where the app calls a dependency. The egress proxy listener (port `8084` in the reference topology) is where `/v1/inject` is consulted to decide mock vs. forward. See [Architecture](/concepts/architecture).

## Extract

The proxy's call to `POST /v1/traces` after observing actual traffic (usually on `passthrough`) so the runtime can record the real request/response as an OTLP span. See [Proxy OTLP API — `/v1/traces`](/reference/proxy-otel-api#post-v1-traces).

## `findInCase`

An SDK method that does a synchronous, in-memory lookup against a case file loaded via `loadCaseFromFile`. Throws if zero or more than one spans match. See the SDK reference for [TypeScript](/reference/sdk-typescript), [Python](/reference/sdk-python), [Java](/reference/sdk-java), or [Go](/reference/sdk-go).

## Fixture

Non-HTTP authentication material (tokens, cookies, OAuth artifacts) carried in a case's `fixtures[]` or installed via `POST /v1/sessions/{id}/fixtures/auth`. See [Auth fixtures](/guides/auth-fixtures).

## Hook

User-defined code — TypeScript/JavaScript in the CLI v1, native code in SDK tests — that customizes matching, mocking, or assertions beyond the built-in rule grammar. See [Write a hook](/guides/write-a-hook).

## Ingress / ingress proxy

The inbound hop between the test client and the app. The ingress proxy listener (port `8082` in the reference topology) tags requests with `x-softprobe-session-id` for session correlation. See [Architecture](/concepts/architecture).

## Inject

The proxy's call to `POST /v1/inject` on each intercepted hop. The runtime either returns `200` with a synthesized response (hit → mock) or `404` (miss → forward upstream). See [Proxy OTLP API — `/v1/inject`](/reference/proxy-otel-api#post-v1-inject).

## `mockOutbound`

An SDK method that registers a `mock` rule with a concrete response on the runtime. SDKs merge rule payloads client-side across consecutive calls; the runtime itself replaces the full rules document per `POST`. See the SDK reference for [TypeScript](/reference/sdk-typescript), [Python](/reference/sdk-python), [Java](/reference/sdk-java), or [Go](/reference/sdk-go).

## Passthrough

A `then.action` (and the default behavior on `/v1/inject` miss) where the proxy forwards the request to the real upstream. Captures still happen; the response is not synthesized. See [Rules & policy](/concepts/rules-and-policy).

## Policy

Session-level defaults applied via `POST /v1/sessions/{id}/policy`, notably `externalHttp: strict` which blocks unmatched outbound calls. See [Rules & policy](/concepts/rules-and-policy).

## Replay

A session `mode` that drives the app through the proxy with `/v1/inject` consulting pre-loaded cases and rules instead of live upstreams. See [Capture & replay](/concepts/capture-and-replay).

## Rule

A `when` matcher paired with a `then` action (`mock`, `error`, `passthrough`, or `capture_only`). Applied at the session level via `POST /v1/sessions/{id}/rules`, embedded in case files, or registered through an SDK. See [Rule schema](/reference/rule-schema).

## Session

A bounded test-run context holding `mode`, policy, loaded case bytes, session rules, and fixtures. Identified by `sessionId` returned from `POST /v1/sessions`. See [Sessions & cases](/concepts/sessions-and-cases#session).

## `sessionRevision`

A monotonically increasing counter on each session that bumps whenever the session's rules, policy, case, or fixtures change. Used by the optional proxy inject cache for invalidation. See [Architecture — proxy inject cache](/concepts/architecture#proxy-inject-cache-optional-sessionrevision-keyed).

## `sp_backend_url`

The WASM plugin config field telling the proxy where to POST `/v1/inject` and `/v1/traces`. In OSS deployments this is the same URL as `SOFTPROBE_RUNTIME_URL`. See [Standalone Envoy deployment](/deployment/envoy-standalone).

## Strict / strict policy

A session policy (`externalHttp: strict`) that fails unmatched outbound calls with HTTP `599` plus the `x-softprobe-strict-miss: 1` header, preventing silent passthroughs. See [Debug a strict-policy miss](/guides/debug-strict-miss).

## Suite

A declarative `suite.yaml` description of many cases to replay, driven by `softprobe suite run`. See [Suite YAML reference](/reference/suite-yaml).

## Trace context / `traceparent` / `tracestate`

The W3C HTTP headers (`traceparent`, `tracestate`) that propagate the session ID across hops. If your app drops these headers between ingress and egress, session correlation breaks — see [Architecture — trace context propagation](/concepts/architecture#trace-context-propagation-critical).

## `x-softprobe-session-id`

The HTTP header the ingress proxy stamps on every request to identify the session for downstream hops. See [Session headers reference](/reference/session-headers).

---

Missing a term? Open a PR against `docs-site/glossary.md` — small, definition-sized entries are always welcome.
