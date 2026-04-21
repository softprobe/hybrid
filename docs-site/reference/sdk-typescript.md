# TypeScript SDK reference

The `@softprobe/softprobe-js` package. Published to npm. Written in TypeScript, ships with full `.d.ts` types.

```bash
npm install --save-dev @softprobe/softprobe-js
```

## Import

```ts
import { Softprobe, SoftprobeSession } from '@softprobe/softprobe-js';
import type { CapturedHit, CapturedResponse, CaseSpanPredicate, MockRuleSpec, Policy } from '@softprobe/softprobe-js';
```

Hook types live under a subpath:

```ts
import type { RequestHook, MockResponseHook, BodyAssertHook, HeadersAssertHook } from '@softprobe/softprobe-js/hooks';
```

## `Softprobe`

The client / factory.

### `new Softprobe(opts?)`

```ts
const softprobe = new Softprobe({
  baseUrl: 'http://127.0.0.1:8080',
  timeoutMs: 5000,
  fetch: customFetch,  // optional; defaults to global fetch
});
```

| Option | Type | Default | Purpose |
|---|---|---|---|
| `baseUrl` | string | `process.env.SOFTPROBE_RUNTIME_URL` or `http://127.0.0.1:8080` | Runtime base URL |
| `timeoutMs` | number | `5000` | HTTP timeout for control-plane calls |
| `fetch` | `typeof fetch` | global | Override HTTP client (e.g. for proxies, keep-alive tuning) |

### `.startSession(body)`

`async (body: { mode: 'capture' | 'replay' | 'generate' }) => Promise<SoftprobeSession>`

Creates a new session on the runtime. Returns a handle bound to the new `sessionId`.

```ts
const session = await softprobe.startSession({ mode: 'replay' });
```

### `.attach(sessionId)`

`(sessionId: string) => SoftprobeSession`

Binds a handle to an existing session without an extra HTTP call. Useful when a session was created in a different process (e.g. CLI captured a session id, test attaches to it).

```ts
const session = softprobe.attach(process.env.SOFTPROBE_SESSION_ID!);
```

## `SoftprobeSession`

Handle to one live session.

### Properties

| Property | Type | Description |
|---|---|---|
| `id` | `string` | The `sessionId` (UUID-ish string) |
| `baseUrl` | `string` | Read-only runtime URL |

### `.loadCaseFromFile(path)`

`async (path: string) => Promise<void>`

Reads the file, ships it to the runtime via `POST /v1/sessions/{id}/load-case`, and keeps a parsed copy locally for `findInCase`. Bumps `sessionRevision`.

```ts
await session.loadCaseFromFile('cases/checkout.case.json');
```

Throws if the file isn't valid JSON or doesn't conform to [`case.schema.json`](/reference/case-schema).

### `.loadCase(doc)`

`async (doc: CaseDocument) => Promise<void>`

As above, but accepts an in-memory case document (e.g. one you built programmatically or fetched over HTTP).

### `.findInCase(predicate)`

`(predicate: CaseSpanPredicate) => CapturedHit`

**Synchronous, in-memory, zero-network** lookup against the loaded case. Returns `CapturedHit`:

```ts
interface CapturedHit {
  response: CapturedResponse;  // materialized HTTP response
  span: unknown;               // raw OTLP span, for advanced assertions
}

interface CapturedResponse {
  status: number;
  headers: Record<string, string>;
  body: string;
}

interface CaseSpanPredicate {
  direction?: 'inbound' | 'outbound';
  method?: string;
  host?: string;
  hostSuffix?: string;
  path?: string;
  pathPrefix?: string;
  service?: string;
}
```

Throws if **zero** or **more than one** spans match. Use `.findAllInCase(...)` if you expect many.

```ts
const hit = session.findInCase({
  direction: 'outbound',
  method: 'POST',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
});
```

### `.findAllInCase(predicate)`

`(predicate: CaseSpanPredicate) => CapturedHit[]`

Non-throwing variant; returns an array (possibly empty).

### `.mockOutbound(spec)`

`async (spec: MockRuleSpec) => Promise<void>`

Registers a concrete mock rule on the runtime. Merges with any rules already applied via this handle.

```ts
interface MockRuleSpec {
  id?: string;
  priority?: number;       // default: 100
  consume?: 'once' | 'many';
  latencyMs?: number;
  direction?: 'inbound' | 'outbound';
  method?: string;
  host?: string;
  hostSuffix?: string;
  path?: string;
  pathPrefix?: string;
  response: CapturedResponse | {
    status: number;
    body?: unknown;                 // SDK serializes objects with JSON.stringify
    headers?: Record<string, string>;
  };
}
```

```ts
await session.mockOutbound({
  direction: 'outbound',
  method: 'POST',
  hostSuffix: 'stripe.com',
  pathPrefix: '/v1/payment_intents',
  response: hit.response,
});
```

### `.clearRules()`

`async () => Promise<void>`

Removes all session-local rules. Keeps the session and loaded case intact. Bumps `sessionRevision`.

### `.setPolicy(policy)`

`async (policy: Policy) => Promise<void>`

```ts
interface Policy {
  externalHttp?: 'strict' | 'allow';
  externalAllowlist?: string[];
  defaultOnMiss?: 'error' | 'passthrough' | 'mock';
}
```

```ts
await session.setPolicy({
  externalHttp: 'strict',
  externalAllowlist: ['localhost', '.internal'],
});
```

### `.setAuthFixtures(fixtures)`

`async (fixtures: Fixtures) => Promise<void>`

Register non-HTTP fixtures (tokens, cookies, metadata) the runtime surfaces to matchers or codegen.

### `.close()`

`async () => Promise<void>`

Deletes the session from the runtime. For capture mode, flushes buffered traces to disk.

Always call in `afterAll` / `finally`.

## `runSuite`

Adapter for running a `suite.yaml` from inside Jest (or any Node test runner that supports top-level `describe` / `it` calls).

```ts
import { runSuite } from '@softprobe/softprobe-js/suite';
import * as hooks from './hooks/checkout';

runSuite('suites/checkout.suite.yaml', { hooks });
```

| Option | Type | Purpose |
|---|---|---|
| `hooks` | `Record<string, Function>` | Resolved hook functions. Names must match `suite.yaml` references. |
| `baseUrl` | string | Runtime base URL override |
| `appUrl` | string | Override `$APP_URL` |
| `filter` | string | Substring to filter cases |
| `parallel` | number | Passed to Jest `concurrent` |

Registers one `describe` per suite, one `it` per case. Standard Jest assertion errors feed into Jest's reporters.

## Hooks

See [Write a hook](/guides/write-a-hook) for authoring. Type definitions:

```ts
import type {
  RequestHook,
  MockResponseHook,
  BodyAssertHook,
  HeadersAssertHook,
  Issue,
} from '@softprobe/softprobe-js/hooks';
```

## Errors

All SDK errors extend `SoftprobeError`:

```ts
import { SoftprobeError, RuntimeError, CaseLookupError } from '@softprobe/softprobe-js';

try {
  const hit = session.findInCase({ ... });
} catch (e) {
  if (e instanceof CaseLookupError) {
    // e.message = "findInCase: 0 matches for {...}"
    // e.matches contains all found spans
  }
  if (e instanceof RuntimeError) {
    // e.status, e.body, e.url
  }
  throw e;
}
```

| Class | When thrown |
|---|---|
| `RuntimeError` | Runtime returned non-2xx |
| `CaseLookupError` | `findInCase` saw 0 or >1 matches |
| `CaseLoadError` | `loadCaseFromFile` failed to parse / validate |
| `SoftprobeError` | Base class; parent of all the above |

## Logging

The SDK is silent by default. To enable debug logs:

```ts
import { setLogger } from '@softprobe/softprobe-js';

setLogger({
  debug: (...args) => console.debug('[softprobe]', ...args),
  warn:  (...args) => console.warn('[softprobe]', ...args),
});
```

Or via env: `SOFTPROBE_LOG=debug npm test`.

## Version compatibility

| SDK version | Runtime versions | Spec version |
|---|---|---|
| `0.5.x` | `0.5.x` | v1 |
| `0.4.x` | `0.4.x` – `0.5.x` | v1 |

`softprobe doctor` reports drift.

## See also

- [Replay in Jest](/guides/replay-in-jest) — tutorial
- [TypeScript SDK on npm](https://www.npmjs.com/package/@softprobe/softprobe-js)
- [HTTP control API](/reference/http-control-api) — wire-level spec the SDK calls
