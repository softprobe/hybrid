# Language Plane Unified Design

**Status:** Proposal for discussion  
**Audience:** Engineers working on `softprobe-js`, `softprobe-go`, `softprobe-python`, `softprobe-java`, and the Arex Java agent  
**Canonical hybrid design:** [design.md](./design.md)

---

## 1. Problem

The proxy path (Envoy + WASM → runtime) works well for HTTP. The language plane today (`softprobe-js`) uses a separate in-process matching model — OTel context, in-memory cassette, `SoftprobeMatcher` — that is not consistent with the control plane. Changing `London` to `New York` in a replay test hits the live network silently because the identifier no longer matches the cassette record and the fallback is passthrough. There is also no story for Redis, Postgres, MongoDB, or arbitrary method calls (Arex-style Java bytecode interception).

The goal is to make the language plane **consistent with the control plane** while keeping it **simple to implement** in each SDK and **expressive enough** for complex matching without moving logic into the runtime.

---

## 2. Core principle: local rule cache, not per-call network

The proxy calls `/v1/inject` on every intercepted request. That is fine for a sidecar that has no local state. Language SDKs can do better: they already hold the full rule set in memory (they built it via `mockOutbound`). The in-process interceptor evaluates rules locally — same `when`/`then` document, no network round-trip per call.

The runtime stays authoritative (it stores the rule set), but evaluation on the hot path is local.

```
proxy path:    intercepted call → /v1/inject (per call) → rule match → mock or miss
language path: intercepted call → local rule cache       → rule match → mock or miss
                                  (populated once at session start + after each mockOutbound)
```

The rule document format is identical. The matching semantics are identical. Only where evaluation runs differs.

---

## 3. Everything is a span

The case file stores OTLP-shaped spans. HTTP spans use `softprobe.protocol: http` and `softprobe.identifier: "GET https://..."`. The same envelope works for any protocol — the `softprobe.protocol` attribute is already the discriminator.

**New protocol values (additive, backwards-compatible):**

| `softprobe.protocol` | `softprobe.identifier` example | Request params | Response body |
|---|---|---|---|
| `http` | `GET https://api.stripe.com/v1/tokens` | (headers, body) | status + body |
| `redis` | `GET session:abc123` | key | serialized value |
| `postgres` | `SELECT * FROM users WHERE id = $1` | JSON array of params | JSON array of rows |
| `mongodb` | `find users {"id":42}` | query doc (JSON) | result doc (JSON) |
| `method` | `com.foo.Auth#doAuth` | JSON array of arg values | JSON-serialized return value |
| `grpc` | `payments.PaymentService/Charge` | proto-JSON body | proto-JSON response |

The `softprobe.identifier` is the stable matching key. For parameterized queries, params are in a separate `softprobe.request.params` attribute, so the identifier stays stable across different param values.

The case file, rule schema, and runtime store require **no structural changes**. The `then.response.body` holds whatever the mock should return; each in-process interceptor knows how to unpack it for its protocol.

---

## 4. Rule schema extension (minimal)

Add two fields to `when` in `rule.schema.json`, both optional and backwards-compatible:

```json
{
  "when": {
    "protocol": "redis",
    "identifier": "GET session:*"
  },
  "then": {
    "action": "mock",
    "response": { "body": "{\"userId\":42}" }
  }
}
```

- `when.protocol` — one of the protocol values above. Defaults to `http` when absent (existing rules unchanged).
- `when.identifier` — exact match or glob (`*` wildcard, `?` single char). When absent, matches any identifier for the given protocol.

No other schema changes needed. The proxy only evaluates HTTP rules today; adding `protocol` as a filter it ignores for non-HTTP rules is a no-op.

---

## 5. SDK changes

### 5.1 `session.run(fn)` — replaces OTel context propagation

```typescript
class SoftprobeSession {
  async run<T>(fn: () => Promise<T>): Promise<T> {
    sessionStack.push(this);
    try { return await fn(); }
    finally { sessionStack.pop(); }
  }
}

// Module-level stack (supports nested sessions, though rare)
const sessionStack: SoftprobeSession[] = [];
export function getActiveSession(): SoftprobeSession | undefined {
  return sessionStack[sessionStack.length - 1];
}
```

No OTel `AsyncHooksContextManager`, no `context.with()`. Simple and correct.

### 5.2 Local rule cache + `GET /v1/sessions/{id}/rules`

The SDK accumulates rules locally in `mockOutbound` (already does this). One new runtime endpoint:

```
GET /v1/sessions/{sessionId}/rules
→ { version: 1, rules: [...] }
```

Used by `session.attach(id)` to populate the local cache when joining an existing session. Not needed for `startSession` flows.

### 5.3 `session.mock()` — protocol-agnostic rule registration

```typescript
await session.mock({
  protocol: 'redis',
  identifier: 'GET session:*',
  response: { body: '{"userId":42}' },
});

// HTTP shorthand (backwards-compatible)
await session.mockOutbound({
  method: 'GET',
  path: '/fragment',
  response: hit.response,
});
// mockOutbound is sugar for: session.mock({ protocol: 'http', direction: 'outbound', ... })
```

### 5.4 `findInCase()` for any protocol

No change needed. `findInCase` already walks OTLP spans and matches on arbitrary attributes. Users pass `{ protocol: 'redis', identifier: 'GET session:*' }` and it works.

### 5.5 In-process interceptors

Each SDK ships one interceptor per protocol, each following the same interface:

```typescript
interface Interceptor {
  // CAPTURE: call real dep, record span, return real result
  // REPLAY: evaluate local rules, return mock or passthrough
}
```

The interceptor calls `getActiveSession()` to find the active session, then `session.evaluate(protocol, identifier, params)` for rule matching.

**HTTP (existing, refactored):** MSW `FetchInterceptor` — already in `softprobe-js`.  
**Redis:** wrap `@redis/client` `sendCommand` — one method, covers all commands.  
**Postgres:** wrap `pg.Client.query` — one method.  
**MongoDB:** wrap `MongoClient` command execution.  
**Method (Java/Arex-style):** Java agent bytecode instrumentation — class+method name as identifier.

### 5.6 Capture path

CAPTURE: interceptor passes through to real dependency, records a span with `softprobe.protocol`, `softprobe.identifier`, `softprobe.request.params` (if applicable), and response body. On `session.close()` the SDK writes `{cassetteDirectory}/{traceId}.case.json` directly to disk.

No runtime involvement needed during capture. The case file is a local artifact.

### 5.7 Replay path

REPLAY: interceptor calls `session.evaluate(protocol, identifier, params)`. The evaluator walks `session.localRules` in priority order, matches `when.protocol` + `when.identifier` (glob), and returns the `then.response` payload. On match: return mock. On miss: strict error (default) or passthrough (if `policy.externalHttp: allow`).

---

## 6. Authoring flow (full example)

### Capture once

```typescript
const session = await softprobe.startSession({
  mode: 'capture',
  cassetteDirectory: 'cases',
  traceId: 'checkout-happy',
});

await session.run(async () => {
  // Real HTTP, Redis, and Postgres calls happen here.
  // Each interceptor records a span.
  await processCheckout({ orderId: 'ord_123' });
});

await session.close();
// → writes cases/checkout-happy.case.json
```

### Replay in CI

```typescript
const session = await softprobe.startSession({ mode: 'replay' });
await session.loadCaseFromFile('cases/checkout-happy.case.json');

// HTTP mock
const httpHit = session.findInCase({ direction: 'outbound', method: 'POST', pathPrefix: '/v1/payment' });
await session.mockOutbound({ method: 'POST', pathPrefix: '/v1/payment', response: httpHit.response });

// Redis mock
const redisHit = session.findInCase({ protocol: 'redis', identifier: 'GET session:*' });
await session.mock({ protocol: 'redis', identifier: 'GET session:*', response: redisHit.response });

// Postgres mock
const pgHit = session.findInCase({ protocol: 'postgres', identifier: 'SELECT * FROM users WHERE id = $1' });
await session.mock({ protocol: 'postgres', identifier: 'SELECT * FROM users WHERE id = $1', response: pgHit.response });

await session.run(async () => {
  const result = await processCheckout({ orderId: 'ord_123' });
  expect(result.status).toBe('succeeded');
});

await session.close();
```

### Arex-style method shortcut (no prior capture needed)

```typescript
await session.mock({
  protocol: 'method',
  identifier: 'com.foo.Auth#doAuth',
  response: { body: 'true' },
});
```

### suite.yaml (no-code path)

```yaml
name: checkout-happy
version: 1
cases:
  - path: cases/checkout-happy.case.json
mocks:
  - name: payment
    match:
      direction: outbound
      method: POST
      pathPrefix: /v1/payment
    hook: mock-response.rotateToken

  - name: session-cache
    match:
      protocol: redis
      identifier: "GET session:*"

  - name: user-lookup
    match:
      protocol: postgres
      identifier: "SELECT * FROM users WHERE id = $1"

  - name: auth-bypass
    match:
      protocol: method
      identifier: "com.foo.Auth#doAuth"
    response:
      body: "true"
```

Hooks receive a `capturedResponse` regardless of protocol. The hook returns a mutated response. The `runSuite` adapter calls `session.mock()` instead of `session.mockOutbound()` for non-HTTP entries.

---

## 7. What each component owns

| Component | Change |
|---|---|
| `spec/schemas/rule.schema.json` | Add `when.protocol` (enum, optional), `when.identifier` (string, optional) |
| `softprobe-runtime` | Add `GET /v1/sessions/{id}/rules`; store `protocol`/`identifier` as opaque JSON (already does) |
| `softprobe-js` | `session.run()` (stack-based), `session.mock()`, local rule cache, `evaluateRules()`, refactored fetch interceptor, new Redis/Postgres interceptors built on reusable protocol adapter interfaces |
| `softprobe-go`, `softprobe-python`, `softprobe-java` | Same pattern per language: `session.Run(fn)`, `session.Mock(...)`, protocol interceptors built as reusable adapter modules |
| `suite.yaml` + `runSuite` | `protocol` field in `mocks[].match`; call `session.mock()` for non-HTTP entries |
| Proxy / WASM | No changes |
| CLI | No changes |

---

## 8. What stays unchanged

- Case file format (OTLP envelope, `traces[]`, `rules[]`, `fixtures[]`)
- `findInCase` / `findAllInCase` — works for any protocol today
- `mockOutbound` — kept as HTTP shorthand, `mock()` is the new generic form
- `suite.yaml` `mocks[].hook` pattern — `MockResponseHook` receives `capturedResponse` regardless of protocol
- The proxy path for HTTP — entirely unchanged
- The runtime control API — one new GET endpoint only

---

## 9. Phase 2 option: manual instrumentation API

Phase 1 can proceed with auto instrumentation as the default path. In Phase 2, we can offer a manual instrumentation option for teams that prefer explicit control and easier debugging over zero-code-change setup.

### 9.1 API sketch (additive)

```typescript
// Wrap a dependency client once; call sites remain unchanged.
const redis = session.wrapClient(redisRaw, {
  protocol: 'redis',
  identify(method, args) {
    return `${method.toUpperCase()} ${String(args[0] ?? '')}`;
  },
});

// Method-level instrumentation for internal business logic (Arex-style use cases).
const doAuth = session.wrapMethod('com.foo.Auth#doAuth', async (...args) => {
  return await realDoAuth(...args);
});
```

This is additive to `session.mock()` / `session.mockOutbound()`. Manual wrappers still record and replay using the same span model (`softprobe.protocol`, `softprobe.identifier`, params, response body), the same rule schema, and the same evaluator.

### 9.2 Why offer it

- Predictable behavior: no hidden monkey-patching at runtime.
- Easier troubleshooting: call boundaries are explicit in user code.
- Faster protocol expansion: users can instrument unsupported clients using custom `identify()` logic.
- Lower SDK maintenance risk: less dependence on internal changes in third-party client libraries.

### 9.3 Trade-off

Manual instrumentation requires code changes. To reduce friction, we can provide an AI-assisted codemod flow that proposes wrapper insertions (and keeps edits reviewable in PR form) for common clients.

### 9.4 Design for reuse now (Phase 1 requirement)

To avoid rework, Phase 1 auto instrumentation modules should be structured as reusable components that manual APIs can call directly:

1. Keep protocol-specific identifier extraction in standalone adapter modules (no direct coupling to monkey-patch bootstrapping).
2. Keep capture/replay decision logic in one shared evaluator path (`session.evaluate(...)`) used by both auto and manual paths.
3. Keep span serialization/deserialization protocol-agnostic and shared.
4. Keep strict-miss and passthrough policy enforcement centralized, not embedded per interceptor.
5. Expose stable internal interfaces (`buildIdentifier`, `captureSpan`, `mockFromRule`) so `wrapClient` / `wrapMethod` can compose existing logic.

With this split, Phase 2 manual APIs become mostly new entry points, not a rewrite of protocol logic.

---

## 10. Open questions

1. **`mock()` vs `mockOutbound()`** — ship `mock()` as the new canonical form, deprecate `mockOutbound()` gradually, or keep both as equals with `mockOutbound()` = `mock({ protocol: 'http', direction: 'outbound', ... })`?

2. **`when.identifier` glob syntax** — `*` prefix/suffix wildcard (simple, covers 95% of cases) or full regex? Recommend starting with `*`-glob and adding regex behind a `identifierRegex` field later if needed.

3. **Parameterized query matching** — should `when.params` exist as an optional deeper matcher (e.g. match only when `$1 = 42`)? Recommend: no for v1. Identifier-only matching is enough for most cases; a hook can handle edge cases.

4. **Capture without runtime** — the capture path writes case files locally with no runtime required. Acceptable? Simpler for the language path; the proxy path still requires the runtime.

5. **Java agent (Arex) integration** — the Java agent identifies methods by `ClassName#methodName`. Does the SDK expose `session.mock({ protocol: 'method', ... })` as a first-class API, or does the agent read rules directly from the runtime via `GET /v1/sessions/{id}/rules`? Recommendation: agent calls `GET /v1/sessions/{id}/rules` at session start, caches locally, evaluates inline — same pattern as other SDKs.
