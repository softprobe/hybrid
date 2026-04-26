# Replay in a Jest test

This guide turns a captured `*.case.json` into a passing Jest test in about **5 minutes**. You will write about 20 lines of TypeScript and end with a green check.

::: info Instrumentation mode
This replay authoring flow is the same for proxy and Node language instrumentation.  
If you are deciding between those models, see [Proxy vs language instrumentation](/guides/proxy-vs-language-instrumentation).
:::

**Prerequisites:**
- A running Softprobe stack ([Installation](/installation)).
- A captured case file from [Capture your first session](/guides/capture-your-first-session), or use the checked-in `spec/examples/cases/fragment-happy-path.case.json` for this walkthrough.

## 1. Install the SDK

```bash
npm install --save-dev @softprobe/softprobe-js
```

If Jest isn't set up yet:

```bash
npm install --save-dev jest ts-jest @types/jest typescript
```

Add a minimal `jest.config.js`:

```js
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
};
```

## 2. The minimum working test

Create `checkout.replay.test.ts`:

```ts
import path from 'path';
import { Softprobe } from '@softprobe/softprobe-js';

const softprobe = new Softprobe();  // reads SOFTPROBE_RUNTIME_URL; defaults to https://runtime.softprobe.dev

describe('checkout replay', () => {
  let sessionId = '';
  let close: () => Promise<void> = async () => {};

  beforeAll(async () => {
    const session = await softprobe.startSession({ mode: 'replay' });
    sessionId = session.id;
    close = () => session.close();

    await session.loadCaseFromFile(
      path.resolve(__dirname, '../cases/checkout-happy-path.case.json'),
    );

    // Replay the Stripe payment dependency from the case.
    const stripeHit = session.findInCase({
      direction: 'outbound',
      method: 'POST',
      hostSuffix: 'stripe.com',
      pathPrefix: '/v1/payment_intents',
    });

    await session.mockOutbound({
      direction: 'outbound',
      method: 'POST',
      hostSuffix: 'stripe.com',
      pathPrefix: '/v1/payment_intents',
      response: stripeHit.response,
    });
  });

  afterAll(async () => {
    await close();
  });

  it('charges the captured card successfully', async () => {
    const res = await fetch('http://127.0.0.1:8082/checkout', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        'x-softprobe-session-id': sessionId,
      },
      body: JSON.stringify({ amount: 1000, currency: 'usd' }),
    });

    expect(res.status).toBe(200);
    expect(await res.json()).toMatchObject({
      status: 'paid',
      paymentId: expect.stringMatching(/^pi_/),
    });
  });
});
```

## 3. Run it

```bash
npx jest
```

Expected:

```
 PASS  ./checkout.replay.test.ts
  checkout replay
    ✓ charges the captured card successfully (38 ms)
```

## Anatomy of the test

### `startSession({ mode: 'replay' })`

Posts to `/v1/sessions` and returns a `SoftprobeSession` handle. The session is valid until you call `close()`.

### `loadCaseFromFile(path)`

Reads and parses the JSON, ships it to the runtime (so embedded rules apply), and keeps a parsed copy locally for `findInCase`.

### `findInCase(predicate)`

**Pure, synchronous, in-memory lookup** against the loaded case. Returns `{ response, span }`:

- `response`: `{ status, headers, body }` — the materialized captured HTTP response, ready to use.
- `span`: the raw OTLP span, for advanced assertions.

It throws if **zero** or **more than one** spans match. That strictness is intentional: ambiguity is caught at test-authoring time, not as a mysterious runtime miss six months later.

| Predicate key | Matches |
|---|---|
| `method` | HTTP method (case-insensitive) |
| `path` | exact path |
| `pathPrefix` | path starts with |
| `host` | exact host |
| `hostSuffix` | host ends with |
| `direction` | `'inbound'` \| `'outbound'` |
| `service` | OTEL `service.name` attribute |

### `mockOutbound(spec)`

Registers a concrete mock rule on the runtime. Once registered, the proxy will return exactly `spec.response` for any request matching the predicate keys of `spec`. Subsequent calls accumulate; use `clearRules()` between test groups to start fresh.

### `close()`

Deletes the session from the runtime. Always call this in `afterAll` — leaked sessions accumulate in memory.

## Mutating a captured response before replay

One of the main reasons Softprobe resolves replay in the SDK is that you can change the response before mocking it. Common cases:

### Refreshing a timestamp

```ts
const hit = session.findInCase({ direction: 'outbound', path: '/v1/time' });
const body = JSON.parse(hit.response.body);
body.servedAt = new Date().toISOString();

await session.mockOutbound({
  direction: 'outbound',
  path: '/v1/time',
  response: { ...hit.response, body: JSON.stringify(body) },
});
```

### Rotating an auth token

```ts
const hit = session.findInCase({
  direction: 'outbound',
  hostSuffix: 'auth.internal',
  pathPrefix: '/token',
});
const body = JSON.parse(hit.response.body);
body.token = `test_${Date.now()}`;

await session.mockOutbound({
  hostSuffix: 'auth.internal',
  pathPrefix: '/token',
  response: { ...hit.response, body: JSON.stringify(body) },
});
```

### Substituting a masked credit card

```ts
const hit = session.findInCase({ direction: 'outbound', hostSuffix: 'stripe.com' });
const body = JSON.parse(hit.response.body);
body.source.card.number = process.env.TEST_CARD ?? '4111111111111111';

await session.mockOutbound({
  hostSuffix: 'stripe.com',
  response: { ...hit.response, body: JSON.stringify(body) },
});
```

## Running multiple test cases from one file

Re-use the session and just swap rules between tests:

```ts
describe('checkout scenarios', () => {
  let session: SoftprobeSession;

  beforeAll(async () => {
    session = await softprobe.startSession({ mode: 'replay' });
    await session.loadCaseFromFile(path.resolve(__dirname, '../cases/checkout.case.json'));
  });

  afterAll(() => session.close());

  beforeEach(() => session.clearRules());

  it('happy path', async () => {
    const hit = session.findInCase({ hostSuffix: 'stripe.com' });
    await session.mockOutbound({ hostSuffix: 'stripe.com', response: hit.response });

    const res = await fetch('http://127.0.0.1:8082/checkout', {
      method: 'POST',
      headers: { 'x-softprobe-session-id': session.id },
      body: JSON.stringify({ amount: 1000 }),
    });
    expect(res.status).toBe(200);
  });

  it('stripe returns 402 card-declined', async () => {
    const hit = session.findInCase({ hostSuffix: 'stripe.com' });
    await session.mockOutbound({
      hostSuffix: 'stripe.com',
      response: { status: 402, body: JSON.stringify({ error: { code: 'card_declined' } }) },
    });

    const res = await fetch('http://127.0.0.1:8082/checkout', {
      method: 'POST',
      headers: { 'x-softprobe-session-id': session.id },
      body: JSON.stringify({ amount: 1000 }),
    });
    expect(res.status).toBe(402);
  });
});
```

## Running tests in parallel

Jest's `test.concurrent` and worker-per-file isolation need **one session per test file** so state doesn't collide. Put `startSession` in `beforeAll`, not `beforeEach`, and never share a `sessionId` across Jest workers.

The hosted runtime handles hundreds of concurrent sessions comfortably; each
session is isolated by tenant and session id.

## Strict policy (fail on unexpected outbound)

Belt-and-braces for "did I forget to mock something?":

```ts
beforeAll(async () => {
  session = await softprobe.startSession({ mode: 'replay' });
  await session.setPolicy({ externalHttp: 'strict' });
  // …loadCase + mocks…
});
```

With `strict`, any outbound hop without a matching mock rule fails with a 5xx from the proxy. Your test will surface it as either an explicit error or a status-code mismatch.

## Common errors

### `findInCase threw: 0 matches for {method: POST, ...}`

Your predicate didn't match anything in the case. Either:

- the capture didn't include that hop (re-check the capture),
- you typed the host/path wrong (use `softprobe inspect case <file>`),
- the predicate is too strict (try `pathPrefix` instead of `path`).

### `findInCase threw: 3 matches`

The case has multiple candidate spans. Narrow the predicate: add `method`, `host`, or `pathPrefix`. If you genuinely want all of them, iterate on `session.findAllInCase(...)` instead (returns an array).

### `fetch failed: ECONNREFUSED`

Your test is hitting an address the proxy doesn't listen on. In the reference stack, ingress is `127.0.0.1:8082`. For hosted deployments, use the proxy address in your cluster.

### `Session not found (404)`

Someone closed the session already (another `afterAll`), the session expired, or
the token belongs to a different tenant. Start a fresh session.

### Test hangs at `await session.close()`

Check hosted runtime reachability and authentication:

```bash
softprobe doctor --verbose
```

If `doctor` is green, inspect app/proxy logs for a request that never returned.

More at [Troubleshooting](/guides/troubleshooting).

## Next

| I want to… | Read |
|---|---|
| Do the same in Python | [Replay in pytest](/guides/replay-in-pytest) |
| Replay thousands of cases from YAML | [Run a suite at scale](/guides/run-a-suite-at-scale) |
| Unmask a credit card before replay | [Write a hook](/guides/write-a-hook) |
| Mock an additional upstream on top of a capture | [Mock an external dependency](/guides/mock-external-dependency) |
