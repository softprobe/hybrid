# Mock an external dependency

Capture gives you real upstream bytes. Sometimes you want to **override** those bytes — simulate a failure mode, test against a value the upstream doesn't naturally produce, or mock a dependency that was never captured in the first place. This guide covers all three.

## The three flavors of mocking

| You have… | You want… | Use |
|---|---|---|
| A captured response, happy path | Replay it verbatim | `findInCase(...) → mockOutbound(..., response: hit.response)` |
| A captured response, modified | Replay a *variant* of the capture | `findInCase(...) → mutate → mockOutbound(...)` |
| No capture at all | Hand-written mock | `mockOutbound(..., response: { status, body })` without `findInCase` |

## Happy-path replay (recap)

The default case from [Replay in Jest](/guides/replay-in-jest):

```ts
const hit = session.findInCase({
  direction: 'outbound',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
});

await session.mockOutbound({
  direction: 'outbound',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
  response: hit.response,
});
```

## Simulate a failure

You captured a successful Stripe call. Now you want to test "what if Stripe returns 429 Too Many Requests?" No need to re-capture — just hand-write the response:

```ts
await session.mockOutbound({
  direction: 'outbound',
  method: 'POST',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
  response: {
    status: 429,
    headers: { 'retry-after': '2', 'content-type': 'application/json' },
    body: JSON.stringify({
      error: { type: 'rate_limit_error', message: 'Too many requests' },
    }),
  },
});
```

Your test now asserts the app's backoff or error-handling behavior — a scenario that is nearly impossible to trigger against the real Stripe sandbox.

## Simulate a slow upstream

Add a synthetic latency to the rule:

```ts
await session.mockOutbound({
  direction: 'outbound',
  hostSuffix: 'stripe.com',
  response: hit.response,
  latencyMs: 2500,   // proxy delays 2.5s before returning the mock
});
```

Useful for testing timeout and circuit-breaker logic without real-world flakes.

## Mock an upstream you never captured

Not every upstream needs to have been captured to be mocked. `mockOutbound` with no `findInCase` works:

```ts
// No capture for this endpoint exists. Just hand-roll a plausible response.
await session.mockOutbound({
  direction: 'outbound',
  method: 'GET',
  hostSuffix: 'geoip.example',
  path: '/lookup',
  response: {
    status: 200,
    body: JSON.stringify({ country: 'US', region: 'CA', city: 'San Francisco' }),
  },
});
```

The SDK sends a rule to the runtime; the proxy matches it on the next outbound request and returns that payload.

## Override part of a captured response

```ts
const hit = session.findInCase({
  direction: 'outbound',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
});

const body = JSON.parse(hit.response.body);
body.amount = 9900;                // replay the capture, but charge less
body.receipt_email = 'test@example.com';

await session.mockOutbound({
  direction: 'outbound',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
  response: { ...hit.response, body: JSON.stringify(body) },
});
```

## Mocking only some dependencies while others pass through

A common integration setup: mock the paid external APIs, let the internal services stay real.

```ts
await session.setPolicy({
  externalHttp: 'allow',   // unmocked outbound goes through
});

// Mock only Stripe and Sendgrid
await session.mockOutbound({ hostSuffix: 'stripe.com',   response: stripeHit.response });
await session.mockOutbound({ hostSuffix: 'sendgrid.net', response: sendgridHit.response });
```

Or, invert the defaults — block everything unknown and allowlist a specific host:

```ts
await session.setPolicy({ externalHttp: 'strict' });

// Block everything by policy, but let one host through:
await session.mockOutbound({
  hostSuffix: 'internal.svc.cluster.local',
  then: { action: 'passthrough' },
  priority: 500,
});
```

## Chain of calls: same upstream, different responses

Suppose the app calls `/v1/order/$ID` three times in a sequence and you want each call to return different data:

```ts
// Call 1: pending
await session.mockOutbound({
  direction: 'outbound',
  pathPrefix: '/v1/order/',
  consume: 'once',
  priority: 300,
  response: { status: 200, body: JSON.stringify({ status: 'pending' }) },
});

// Call 2: processing
await session.mockOutbound({
  direction: 'outbound',
  pathPrefix: '/v1/order/',
  consume: 'once',
  priority: 200,
  response: { status: 200, body: JSON.stringify({ status: 'processing' }) },
});

// Call 3: final
await session.mockOutbound({
  direction: 'outbound',
  pathPrefix: '/v1/order/',
  consume: 'many',
  priority: 100,
  response: { status: 200, body: JSON.stringify({ status: 'completed' }) },
});
```

`consume: 'once'` removes the rule after first match; `priority` orders the fallbacks. The last rule (`consume: 'many'`) is the final state.

## Mocking inbound traffic

Sometimes you want to test a webhook endpoint **on your app** against a specific inbound request the app returns data for. That's the same `mockOutbound` API with `direction: 'inbound'`:

```ts
await session.mockOutbound({
  direction: 'inbound',
  method: 'POST',
  path: '/webhooks/stripe',
  response: {
    status: 200,
    body: JSON.stringify({ received: true, eventId: 'evt_test_123' }),
  },
});
```

Called from the proxy's ingress leg; returns the canned response without the app ever running for that request. Usually you don't need this (driving the real app handler is the point), but it's useful for testing ingress middleware in isolation.

## Clearing rules between tests

`mockOutbound` calls within a session **accumulate**. Between test cases:

```ts
beforeEach(() => session.clearRules());
```

This drops all session-local mocks but keeps the session and loaded case intact — fast reset.

## Observe-only: `capture_only` rules

Sometimes you want a request to go through to the real upstream **and** be recorded — for example, to capture a new endpoint during an otherwise-replayed test. That's the `capture_only` action:

```yaml
# rules/audit-partner.yaml
version: 1
rules:
  - id: audit-partner-calls
    priority: 500
    when:
      direction: outbound
      host: partner.example.com
    then:
      action: capture_only
```

```bash
softprobe session rules apply --session $SESSION_ID --file rules/audit-partner.yaml
```

`capture_only` **does not** synthesize a response — the proxy forwards to the real upstream as usual. It only ensures the hop shows up in `POST /v1/traces` and lands in the case file at close.

`mockOutbound` does not emit `capture_only` rules; apply them via `softprobe session rules apply` or embed them in `case.rules[]`. See the [rule schema reference](/reference/rule-schema) for the full action inventory.

## Common mistakes

**Predicate too broad.** Writing `{ hostSuffix: 'stripe.com' }` mocks every Stripe endpoint — the one for payments, the one for webhooks, the one for customers. Narrow to `{ pathPrefix: '/v1/payment_intents' }` unless you really mean all of them.

**Predicate too strict.** Writing `{ path: '/v1/payment_intents/pi_abc123' }` only matches that exact URL. If the ID varies between captures, use `pathPrefix: '/v1/payment_intents'`.

**Headers mismatch in replay.** The proxy returns your `response.headers` verbatim. If you omit `content-type`, downstream parsers may choke. When in doubt, echo `hit.response.headers`.

**Body type mismatch.** `mockOutbound` accepts either a string or a JSON-serializable object for `response.body`. If you pass an object, the SDK serializes with `JSON.stringify` — make sure the `content-type` header matches.

## Next

- [Rules and policy](/concepts/rules-and-policy) — the mental model beneath these calls.
- [Write a hook](/guides/write-a-hook) — reusable transforms (PII masking, date rewriting) that apply to many rules.
- [Run a suite at scale](/guides/run-a-suite-at-scale) — the same patterns, driven from YAML.
