# Rules and policy

Rules are how Softprobe expresses **"when this kind of request happens, do this"**. Policy is how you set safe defaults for anything without a matching rule. Together they define the entire decision space the runtime operates over.

This page is a concept reference. For the wire shape, see [`spec/schemas/rule.schema.json`](https://github.com/softprobe/softprobe/blob/main/spec/schemas/rule.schema.json) or the [rule schema reference](/reference/rule-schema).

::: info Author-time vs request-time
**The runtime never walks captured `traces[]` on the inject hot path.** Rules are the only thing the runtime evaluates on each `/v1/inject`. Choosing *which* captured response to return for a given request is **authoring-time work** done in the SDK via `findInCase`; the resulting response is then registered as a concrete `mock` rule.

This is why you won't find a "replay from case" action — the SDK turns case lookup into regular rules. See [design §5.3](https://github.com/softprobe/softprobe/blob/main/docs/design.md#53-inject-resolution-placement-normative) for the rationale.
:::

## A rule has two halves

```yaml
- id: stripe-payments-mock
  priority: 100
  when:                           # the matcher
    direction: outbound
    host: api.stripe.com
    method: POST
    pathPrefix: /v1/payment_intents
  then:                           # the action
    action: mock
    response:
      status: 200
      headers:
        content-type: application/json
      body: '{"id":"pi_mock","status":"succeeded"}'
```

The **`when`** is an AND of predicates. The **`then`** is exactly one of four actions. You never mix actions in one rule.

## The four actions

| Action | What the proxy returns on match | Typical use |
|---|---|---|
| `mock` | 200 + `then.response` fields | Replay a captured upstream |
| `error` | Custom error response (any status) | Simulate a failing dependency |
| `passthrough` | Forwards to the real upstream | "I want this one real" overrides |
| `capture_only` | Forwards + records via `/v1/traces` | Fine-grained capture during a mostly-mocked run |

Most rules you will ever write are `mock` (including all rules produced by `mockOutbound`).

## Predicates in `when`

| Key | Matches | Notes |
|---|---|---|
| `direction` | `"inbound"` \| `"outbound"` | Which leg of the proxy |
| `host` | exact host | Case-insensitive |
| `hostSuffix` | host ends with | `"stripe.com"` matches `api.stripe.com` |
| `method` | HTTP method | Case-insensitive |
| `path` | exact path | Leading slash included |
| `pathPrefix` | path starts with | |
| `headerMatch` | header name → regex | Multiple entries AND'd |
| `bodyJsonPathEquals` | JSONPath → literal value | Useful for content-based routing |
| `traceTagEquals` | OTEL tag → literal | Last-resort escape hatch |

Predicates are combined with AND. There is no `or` operator in v1. To express disjunction, register two rules.

## Policy: defaults for the unmatched

Policy sets the runtime's behavior for requests no rule matched.

```yaml
policy:
  externalHttp: strict   # block unmatched outbound (default: allow)
  internalHttp: allow    # forward unmatched internal hops
  defaultOnMiss: error   # fail the test (alternative: "passthrough")
```

Under the hood, policy is compiled into a synthesized **lowest-priority** rule that the inject handler evaluates last. You can think of it as "the catch-all rule your test didn't write."

### `externalHttp: strict`

The most common policy choice in CI. Any outbound call to a host that isn't on an allowlist or covered by a matched rule returns an error to the app.

```yaml
policy:
  externalHttp: strict
  externalAllowlist:
    - internal.svc.cluster.local
    - localhost
```

This catches the classic "I forgot to mock Stripe" bug *before* it hits the real Stripe.

### `defaultOnMiss: error | passthrough | mock`

Fine-grained control over what happens on a miss:

| Value | Behavior |
|---|---|
| `passthrough` (default) | Forward to the real upstream |
| `error` | Return the policy's error response |
| `mock` | Return a policy-defined canned response (rare) |

## How precedence works

When multiple rules could match, the runtime picks exactly one. The algorithm is deterministic:

1. **Highest `priority`** wins.
2. On a tie across layers, **later layer wins**: session rules (from `mockOutbound`) beat case-embedded rules, which beat policy defaults.
3. On a tie within one layer, **later entry in the array wins** (the last `mockOutbound` call or last rule in `case.rules[]`).

These two tie-breakers compose: to force a rule to override a same-priority rule in a higher layer, lift its `priority` up.

### Worked example

Case file ships:

```yaml
# case.rules[]
- id: partner-default
  priority: 100
  when:  { direction: outbound, host: partner.example.com }
  then:  { action: mock, response: { status: 200, body: "{\"source\":\"case\"}" } }
```

Test does:

```ts
await session.mockOutbound({
  name: 'partner-override',
  priority: 100,
  direction: 'outbound',
  host: 'partner.example.com',
  response: { status: 200, body: JSON.stringify({ source: 'test' }) },
});
```

Both have `priority: 100`. The **session rule wins** because it's in a later layer, and the test sees `{"source":"test"}`. No need to bump the priority.

```text
  Session rules  (your mockOutbound calls)   ◄ highest
        ↑
  Case-embedded rules (shipped with .case.json)
        ↑
  Policy-synthesized catch-all                ◄ lowest
```

Practical consequence: you can load a case with default rules and then selectively override individual behaviors from your test without editing the case file. This is the main reason case-embedded rules exist.

## Consume: once vs. many

A rule can declare how many times it applies:

```yaml
- id: first-call-returns-503
  consume: once
  priority: 200
  when: { direction: outbound, host: flaky.svc }
  then:
    action: error
    error: { status: 503, body: { reason: "simulated outage" } }

- id: subsequent-calls-succeed
  consume: many
  priority: 100
  when: { direction: outbound, host: flaky.svc }
  then:
    action: mock
    response: { status: 200, body: { ok: true } }
```

In v1, `consume` on `mock` rules is honored by the session rule list (the SDK removes the rule after one match). It is **not** a signal to the runtime to walk captured traces — see the architecture decision in [Capture and replay](/concepts/capture-and-replay).

## Building rules the hard way vs. the easy way

Most tests should use the SDK's `mockOutbound` helper — it compiles down to a correctly shaped rule without you writing YAML.

```ts
await session.mockOutbound({
  direction: 'outbound',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
  method: 'POST',
  response: hit.response,
});
```

is sugar for

```json
POST /v1/sessions/$ID/rules
{
  "version": 1,
  "rules": [
    {
      "name": "suite-mock-1",
      "priority": 100,
      "when": { "direction": "outbound", "hostSuffix": "stripe.com", "pathPrefix": "/v1/payment_intents", "method": "POST" },
      "then": { "action": "mock", "response": { "status": 200, "headers": {...}, "body": "{\"id\":\"pi_test\",...}" } }
    }
  ]
}
```

Write rules by hand (as YAML in `softprobe session rules apply --file rules/stripe.yaml`) when:

- you ship a shared rule pack across many tests (PII redaction, global auth bypass),
- you set a fleet-wide policy via configuration-as-code in CI,
- you need the exact wire shape for a contract test.

Everywhere else, prefer `mockOutbound`.

## SDKs merge, the runtime replaces

The runtime's `POST /v1/sessions/{id}/rules` endpoint **replaces the entire rules document** — whatever rules you send becomes the session's full rule set. This is simple but would be brittle for test authors if surfaced raw.

The SDKs compensate by **merging on the client side**: every `mockOutbound()` call appends to a local list and re-sends the complete list, so consecutive calls accumulate. The `clearRules()` method resets the SDK-side list and sends an empty document to the runtime.

| Channel | Merge behavior |
|---|---|
| SDK `mockOutbound()` | Appends; consecutive calls accumulate |
| SDK `clearRules()` | Resets the SDK-side list; sends `{ "version": 1, "rules": [] }` |
| CLI `softprobe session rules apply` | Replaces (no client-side merging) |
| Raw `POST …/rules` | Replaces |

If you mix SDK `mockOutbound` with `softprobe session rules apply` on the same session, the last writer wins. Pick one channel per session.

## Session revision and cache safety

Every rule change bumps `sessionRevision`. The proxy may cache **inject decisions** (not arbitrary upstream bytes) keyed on `(sessionId, sessionRevision, requestFingerprint)`. Any rule mutation invalidates the cache for that session — so a `clearRules()` followed by a `mockOutbound()` is guaranteed to be seen by the proxy on the next request.

## Common patterns

### Mock everything except one real call

```ts
await session.setPolicy({ externalHttp: 'strict' });
await session.mockOutbound({ hostSuffix: 'stripe.com', response: hit.response });
// Now allow one specific call through:
await session.mockOutbound({
  hostSuffix: 'auth.internal',
  then: { action: 'passthrough' },
  priority: 500,   // beats the strict default
});
```

### Simulate a partial outage

```ts
await session.mockOutbound({
  hostSuffix: 'db-replica.internal',
  then: { action: 'error', error: { status: 503, body: { error: 'down' } } },
  priority: 1000,
});
```

### Redact a field from captures

```yaml
# rules/redact.yaml — apply in capture mode
version: 1
rules:
  - id: strip-auth-headers
    priority: 10000
    when: { direction: outbound }
    then:
      action: capture_only
      captureOnly:
        redactHeaders: [authorization, x-api-key]
```

## What rules do *not* do

- **Transform live responses.** A rule either mocks (synth) or passes through (untouched). To modify a response in-flight, use a post-capture hook.
- **Run user code.** The runtime is a deterministic matcher. Custom logic (decryption, signature recomputation) lives in SDK hooks, not in rules.
- **Synthesize traffic.** Rules react to requests; they don't initiate them.

---

**Next:** [Capture your first session →](/guides/capture-your-first-session)
