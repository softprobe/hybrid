# Write a hook

A **hook** is a small TypeScript function you write, give a stable name to, and reference by name from your `suite.yaml`. The Softprobe CLI (or an SDK adapter like `runSuite`) invokes the hook at the appropriate moment — rewriting a captured response, mutating a request, asserting a body — without you having to leave declarative YAML for the 90% case.

This page shows the four hook kinds, their contracts, and realistic examples for each.

## When do I need a hook?

| Problem | Can YAML solve it? | Use a hook? |
|---|---|---|
| Replay a captured response verbatim | Yes (`source: case`) | No |
| Ignore a volatile field like `$.timestamp` in comparison | Yes (`assertions.body.ignore`) | No |
| Stamp the current time into a replayed response | No | **Yes** (`mock-response` hook) |
| Substitute a masked credit card with a test value | No | **Yes** (`request` hook) |
| Assert `total == items_sum + shipping` | No | **Yes** (`assert-body` hook) |
| Compute an HMAC over the replayed body | No | **Yes** (`mock-response` hook) |

Rule of thumb: if the transformation needs a JavaScript function body, it's a hook. Everything else should stay in YAML.

## The four hook kinds

| Kind | Invoked | Returns | Typical use |
|---|---|---|---|
| `request` | Before the SUT is called | `{method, path, headers, body}` | Unmask PII, override auth, substitute test card |
| `mock-response` | Before a mock rule is registered | `{status, headers, body}` | Refresh timestamp, rotate token, recompute signature |
| `assert-body` | After the SUT's response arrives | `Issue[]` (empty = pass) | Custom invariants beyond field equality |
| `assert-headers` | After the SUT's response arrives | `Issue[]` | Header-specific invariants |

All four share a common contract: a pure function from an input object to an output object, with no side effects, no network calls, no randomness that isn't seeded.

## Contract reference

Types are exported from `@softprobe/softprobe-js/hooks`:

```ts
import type {
  RequestHook,
  MockResponseHook,
  BodyAssertHook,
  HeadersAssertHook,
  Issue,
} from '@softprobe/softprobe-js/hooks';
```

### `RequestHook`

```ts
type RequestHook = (ctx: {
  request:  { method: string; path: string; headers: Record<string, string>; body: string };
  case:     CaseDocument;            // full parsed case, read-only
  env:      Record<string, string>;  // process env
  caseFile: string;                  // path to the .case.json
}) => {
  method?: string;
  path?: string;
  headers?: Record<string, string>;
  body?: string;
};
```

Returning `{}` means "use the request as-is." Any returned key replaces the original.

### `MockResponseHook`

```ts
type MockResponseHook = (ctx: {
  capturedResponse: { status: number; headers: Record<string, string>; body: string };
  capturedSpan: unknown;              // raw OTLP span
  mockName: string;                   // the `name:` from suite.yaml
  case: CaseDocument;
  env: Record<string, string>;
  caseFile: string;
}) => {
  status?: number;
  headers?: Record<string, string>;
  body?: string;
};
```

Return the fields you want to change; others stay as captured.

### `BodyAssertHook`

```ts
type BodyAssertHook = (ctx: {
  actual:   unknown;    // parsed JSON (or string for non-JSON)
  captured: unknown;    // parsed JSON from the case ingress response
  case: CaseDocument;
  env: Record<string, string>;
}) => Issue[];

interface Issue {
  path: string;
  expected?: unknown;
  actual?: unknown;
  reason?: string;
}
```

Return an **empty array** for pass; any `Issue[]` means the case fails.

### `HeadersAssertHook`

```ts
type HeadersAssertHook = (ctx: {
  actual:   Record<string, string>;
  captured: Record<string, string>;
  case: CaseDocument;
  env: Record<string, string>;
}) => Issue[];
```

## Real examples

### Unmask a credit card before sending to the SUT

Captures from production have `card.number` masked. Replace it with a test card before the SUT's validator rejects it.

```ts
// hooks/checkout.ts
import type { RequestHook } from '@softprobe/softprobe-js/hooks';

export const unmaskCard: RequestHook = ({ request, env }) => {
  if (!request.body) return {};
  const body = JSON.parse(request.body);
  if (!body.card) return {};

  body.card.number = env.TEST_CARD ?? '4111111111111111';
  body.card.exp_month = 12;
  body.card.exp_year  = 2030;
  body.card.cvc       = '123';

  return { body: JSON.stringify(body) };
};
```

Referenced in `suite.yaml`:

```yaml
defaults:
  request:
    source: case.ingress
    transform: checkout.unmaskCard
```

### Stamp the current time into a mocked response

The captured response includes `"servedAt":"2024-11-14T10:22:01Z"`. Your SUT validates it's within the last 60 seconds.

```ts
// hooks/time.ts
import type { MockResponseHook } from '@softprobe/softprobe-js/hooks';

export const freshServedAt: MockResponseHook = ({ capturedResponse }) => {
  const body = JSON.parse(capturedResponse.body);
  body.servedAt = new Date().toISOString();
  return { body: JSON.stringify(body) };
};
```

```yaml
mocks:
  - name: auth-token
    match: { hostSuffix: auth.internal, pathPrefix: /token }
    source: case
    hook: time.freshServedAt
```

### Recompute an HMAC signature

If your service validates a signature against the replayed body:

```ts
// hooks/webhook.ts
import crypto from 'crypto';
import type { MockResponseHook } from '@softprobe/softprobe-js/hooks';

export const recomputeSignature: MockResponseHook = ({ capturedResponse, env }) => {
  const body = capturedResponse.body;
  const secret = env.WEBHOOK_SECRET;
  if (!secret) throw new Error('WEBHOOK_SECRET is required');

  const sig = crypto.createHmac('sha256', secret).update(body).digest('hex');
  return {
    headers: { ...capturedResponse.headers, 'x-signature': sig },
  };
};
```

### Custom body assertion: totals match items

The business rule says `total == sum(items.price) + shipping`. Captured data has varied totals per case, so a field-equality check isn't enough.

```ts
// hooks/checkout.ts
import type { BodyAssertHook, Issue } from '@softprobe/softprobe-js/hooks';

export const assertTotalsMatchItems: BodyAssertHook = ({ actual, captured }) => {
  const issues: Issue[] = [];
  const a = actual as { total: number; shipping: number };
  const ingress = JSON.parse((captured as any).ingressBody);
  const expected = ingress.items.reduce((s: number, i: any) => s + i.price, 0) + a.shipping;

  if (Math.abs(a.total - expected) > 0.01) {
    issues.push({
      path: '$.total',
      expected,
      actual: a.total,
      reason: 'items+shipping mismatch',
    });
  }
  return issues;
};
```

```yaml
assertions:
  body:
    mode: json-subset
    custom: checkout.assertTotalsMatchItems
```

### Assert a rate-limit header exists on 429 responses

```ts
// hooks/rate-limit.ts
import type { HeadersAssertHook } from '@softprobe/softprobe-js/hooks';

export const requireRetryAfter: HeadersAssertHook = ({ actual }) => {
  if (actual['content-type']?.includes('json') && !actual['retry-after']) {
    return [{ path: 'headers.retry-after', reason: 'missing on rate-limited response' }];
  }
  return [];
};
```

## How hooks are loaded

### From the CLI

```bash
softprobe suite run suites/checkout.suite.yaml \
  --hooks hooks/checkout.ts,hooks/time.ts
```

Under the hood the CLI spawns a Node sidecar, compiles the TypeScript with `esbuild`, and exposes every exported function under `<fileBasename>.<exportName>`. You reference them as `checkout.unmaskCard` in YAML.

### From `runSuite` in Jest

```ts
import { runSuite } from '@softprobe/softprobe-js/suite';
import * as checkoutHooks from '../hooks/checkout';
import * as timeHooks from '../hooks/time';

runSuite('suites/checkout.suite.yaml', {
  hooks: { ...checkoutHooks, ...timeHooks },
});
```

Jest loads the hooks in-process — no Node sidecar needed.

### From `run_suite` in pytest, JUnit, Go

Pytest, JUnit, and Go adapters load the **same YAML** but expect hooks in their native language. If you write hooks in Python for pytest and in TypeScript for the CLI, they coexist by language — the YAML stays unchanged, but the hook *names* resolve to different modules at runtime.

See [`softprobe-python` docs](/reference/sdk-python) for the Python hook API (same shape, `snake_case`).

## Hook discipline

**Pure functions, please.** Hooks that make network calls or write files are bugs waiting to happen. If you need side effects, express them as a separate step before the suite runs.

**Fail loud.** Throwing from a hook fails the case with a clear error. That's good — silent fallbacks hide bugs.

**Keep hooks small.** If a hook is more than 30 lines of logic, split it or reconsider whether the suite YAML should grow.

**Version your hooks alongside the suite.** Treat `hooks/*.ts` as part of the test fixture, not as production code.

**Don't catch and ignore errors.** Let exceptions propagate. The CLI prints the stack trace into the JUnit failure and the HTML report.

## Debugging

### Print from a hook

```ts
export const unmaskCard: RequestHook = ({ request, env }) => {
  console.error('[unmaskCard] env.TEST_CARD =', env.TEST_CARD);
  // ...
};
```

`console.error` flows to the CLI's stderr and appears in CI logs. `console.log` is reserved for the JSON protocol between the CLI and the sidecar — don't use it.

### Run a single case

```bash
softprobe suite run suites/checkout.suite.yaml \
  --filter 'happy' \
  --hooks hooks/checkout.ts \
  --verbose
```

`--verbose` prints the before/after payload at each hook invocation.

### Step through a hook in Jest

The easiest way to set a breakpoint in a hook is to call it from a Jest test:

```ts
import { unmaskCard } from '../hooks/checkout';

it('unmasks the card', () => {
  const before = { method: 'POST', path: '/checkout', headers: {}, body: '{"card":{"number":"****1111"}}' };
  const after = unmaskCard({ request: before, case: {} as any, env: { TEST_CARD: '4242424242424242' }, caseFile: '' });
  expect(after.body).toContain('4242');
});
```

Now you can breakpoint inside the hook from your IDE without needing the CLI or sidecar at all.

## Next

- [Run a suite at scale](/guides/run-a-suite-at-scale) — how hooks are referenced from the suite file.
- [Suite YAML reference](/reference/suite-yaml) — every field, every transformation key.
- [CI integration](/guides/ci-integration) — shipping hooks as part of the CI container.
