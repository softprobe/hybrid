# Debug a strict-policy miss

Running tests under `externalHttp: strict` is the fastest way to catch "I forgot to mock Stripe" bugs — the proxy refuses to let any un-matched outbound call through. When the policy fires, **your test fails with a confusing HTTP error** because the error originated at the proxy, not in your test or your app. This guide teaches you how to recognize the symptoms, correlate with runtime logs, and fix the missing rule.

## What the SUT sees when strict blocks an outbound

When a request matches no rule and `policy.externalHttp = strict` (or `policy.defaultOnMiss = error`), the proxy synthesizes an error response **as if it came from the upstream**. Defaults per [`docs/design.md` §8.1](https://github.com/softprobe/softprobe/blob/main/docs/design.md#81-what-can-a-rule-say):

| Field | Default |
|---|---|
| `status` | `599` (non-standard, deliberately distinct from 5xx from real upstreams) |
| `headers.content-type` | `application/json` |
| `headers.x-softprobe-strict-miss` | `1` — machine-detectable marker |
| `body` | `{"error":"strict policy blocked unmatched outbound","sessionId":"sess_…","requestFingerprint":"…"}` |

Your app's HTTP client raises whatever exception it normally raises for a 5xx — `got 599` in Node, `httpx.HTTPStatusError` in Python, `IOException` in Java, etc.

The `x-softprobe-strict-miss: 1` header is the fastest way to distinguish a strict miss from a genuine upstream 5xx. Check for it before anything else.

## Symptom → diagnosis decision tree

```text
Test fails with an HTTP error inside the SUT
 │
 ├─ Response status is exactly 599?         ─► Strict miss (90% likely)
 │
 ├─ Response header x-softprobe-strict-miss: 1? ─► Confirmed strict miss
 │
 ├─ Status is 5xx but not 599?               ─► Real upstream failure OR proxy timeout (see Troubleshooting)
 │
 └─ Error is ECONNREFUSED?                   ─► Proxy isn't running; not a policy issue
```

## Step 1 — Confirm it's a strict miss

Add a logging hook to your test to capture the response on failure:

```ts
try {
  await fetch(url, { headers: { 'x-softprobe-session-id': sessionId } });
} catch (e) {
  const res = e.response;
  console.error('status:', res?.status);
  console.error('strict-miss:', res?.headers.get('x-softprobe-strict-miss'));
  console.error('body:', await res?.text());
  throw e;
}
```

If the status is `599` or the header is `1`, skip to [Step 2](#step-2--identify-the-missing-rule).

## Step 2 — Identify the missing rule

The strict-miss body includes a `requestFingerprint` — a stable hash of `(method, host, path, body-hash)`. Combined with your **runtime logs**, you can pinpoint the exact request that wasn't matched.

### Tail the runtime log

```bash
docker logs -f softprobe-runtime 2>&1 | grep -E "(inject|strict)"
```

Typical output on a miss:

```text
{"level":"info","msg":"inject miss","sessionId":"sess_abc","method":"POST","host":"api.stripe.com","path":"/v1/customers","policy":"strict","action":"error","status":599}
```

Every strict-miss creates one line. Copy the `method`, `host`, and `path` — they're your rule's `when` fields.

### Or inspect session stats

```bash
curl -s $SOFTPROBE_RUNTIME_URL/v1/sessions/$SOFTPROBE_SESSION_ID/stats | jq
```

```json
{
  "injectedSpans": 3,
  "strictMisses": 1,
  "lastStrictMiss": {
    "method": "POST",
    "host": "api.stripe.com",
    "path": "/v1/customers"
  }
}
```

## Step 3 — Fix it

You have three choices, in order of preference.

### Option A (recommended): add the missing `mockOutbound`

Usually the capture includes the call you need; you just forgot to mock it. Add the rule and re-run:

```ts
const hit = session.findInCase({
  direction: 'outbound',
  method: 'POST',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/customers',
});

await session.mockOutbound({
  direction: 'outbound',
  method: 'POST',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/customers',
  response: hit.response,
});
```

If `findInCase` also throws "0 matches", the capture is incomplete — re-capture with that endpoint hit during the session.

### Option B (temporary): relax the policy

If you need to ship green tests now and want to fix capture later:

```ts
await session.setPolicy({
  externalHttp: 'strict',
  externalAllowlist: ['api.stripe.com'],   // explicitly allowed through
});
```

Or drop strict entirely for one test:

```ts
beforeAll(async () => {
  // no setPolicy call — default policy (externalHttp: 'allow') is in effect
});
```

This is a **fix-by-delay** — add a TODO and come back.

### Option C: add a passthrough rule with higher priority

If the call should always go through (e.g. a local DB, an intra-cluster service), add a `passthrough` rule that beats the policy's catch-all:

```ts
await session.mockOutbound({
  direction: 'outbound',
  hostSuffix: '.svc.cluster.local',
  then: { action: 'passthrough' },
  priority: 500,
});
```

Because policy is a priority-0 synthesized rule, any explicit rule with `priority >= 1` shadows it.

## Step 4 — Verify the fix

Re-run the test. Check the session stats:

```bash
curl -s $SOFTPROBE_RUNTIME_URL/v1/sessions/$SOFTPROBE_SESSION_ID/stats | jq '.strictMisses'
```

Should be `0`.

## Common root causes

| Cause | Tell | Fix |
|---|---|---|
| Forgot to mock a newly-added upstream call | 1 strict-miss with a recognizable host | Option A |
| Test logic triggers a retry on error, doubling miss counts | `strictMisses: 2` but only 1 expected call | Option A + check retry policy in SUT |
| Capture was too short — didn't include a conditional branch | `findInCase` also throws "0 matches" | Re-capture with the branch exercised |
| Propagation broken — the egress proxy never sees the session id | Strict miss on every outbound, host is the real upstream | See [My egress mocks aren't hit](/guides/troubleshooting#my-egress-mocks-arent-hit) |
| Rule's `when` is typo'd or too specific | Miss despite seeing the correct `mockOutbound` call in code | Log the compiled rule with `session.dumpRules()` (TS/Python) and compare |

## Session-wide strict-miss protection

If you want strict-miss failures to surface as a **test-suite assertion** rather than a mid-test HTTP error, add an after-all check:

```ts
afterAll(async () => {
  const stats = await session.getStats();
  expect(stats.strictMisses).toBe(0);
  await session.close();
});
```

This catches misses even if the test's logic swallows the error.

## Related troubleshooting entries

- [`403 Forbidden` on outbound under strict policy](/guides/troubleshooting#_403-forbidden-on-outbound-under-strict-policy)
- [Test passes locally but fails in CI with `ECONNREFUSED`](/guides/troubleshooting#test-passes-locally-but-fails-in-ci-with-econnrefused)
- [My egress mocks aren't hit](/guides/troubleshooting#my-egress-mocks-arent-hit)

## See also

- [Rules and policy → policy defaults](/concepts/rules-and-policy#policy-defaults-for-the-unmatched)
- [Rule schema → `action: error`](/reference/rule-schema#then-action-error) — the shape of the synthesized miss response.
