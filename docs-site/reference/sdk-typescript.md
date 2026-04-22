# TypeScript SDK reference

::: tip Ships in this build (PD3 complete)
The short error names (`RuntimeError`, `CaseLookupError`, …), `@softprobe/softprobe-js/hooks`, `@softprobe/softprobe-js/suite` + `runSuite`, and `setLogger` / `SOFTPROBE_LOG` are **shipped** — see [Phase PD3](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd3--typescript-sdk-reference-reality-alignment) in `tasks.md`. Canonical source in this monorepo:

- [`softprobe-js/src/errors.ts`](https://github.com/softprobe/hybrid/blob/main/softprobe-js/src/errors.ts) — class hierarchy and legacy aliases (`SoftprobeRuntimeError`, …)
- [`softprobe-js/src/hooks.ts`](https://github.com/softprobe/hybrid/blob/main/softprobe-js/src/hooks.ts) — hook types for `suite.yaml`
- [`softprobe-js/src/suite.ts`](https://github.com/softprobe/hybrid/blob/main/softprobe-js/src/suite.ts) — Jest `runSuite` runner
- [`softprobe-js/src/hook-runner.ts`](https://github.com/softprobe/hybrid/blob/main/softprobe-js/src/hook-runner.ts) — `HookExecutionError` when a hook throws

If an import of `/hooks` or `/suite` fails, ensure your resolver honors `package.json#exports` (Node 16+, current Jest/Vite/esbuild do).
:::

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

::: warning Throws on ambiguity
`findInCase` **throws** `CaseLookupError` if **zero** or **more than one** spans match the predicate — ambiguity is caught at authoring time, never at request time. The thrown error includes the matched span ids so you can narrow the predicate. Use [`findAllInCase`](#findallincase-predicate) when you genuinely expect multiple matches.
:::

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

Registers a concrete mock rule on the runtime.

::: info Merge on the client, replace on the wire
The runtime's `POST /v1/sessions/{id}/rules` **replaces** the entire rules document. The SDK compensates by keeping a local merged list: every call to `mockOutbound` appends to that list and sends the complete list to the runtime. Two calls in the same test process accumulate. Use [`clearRules()`](#clearrules) to reset the SDK-side list.
:::

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

All SDK errors extend `SoftprobeError`.

### Error catalog

| Condition | Error class | Typical cause | Recovery |
|---|---|---|---|
| **Runtime unreachable** | `SoftprobeRuntimeUnreachableError` | DNS / TCP / TLS / timeout before an HTTP status exists | Start the runtime; check `baseUrl`; `softprobe doctor` |
| **Unknown session** | `SoftprobeUnknownSessionError` (`instanceof RuntimeError`, `status: 404`) | Session already closed, wrong id | Start a fresh session |
| **Strict miss** (proxy returns error to app) | Not an SDK error — surfaces as an HTTP error inside the SUT, e.g. `Error: Request failed with status 599` | Missing `mockOutbound` or wrong predicate | Add the rule; see [Debug strict miss](/guides/troubleshooting#_403-forbidden-on-outbound-under-strict-policy) |
| **Invalid rule payload** | `RuntimeError` with `status: 400`, body describing the schema violation | Rule body doesn't validate against [rule-schema](/reference/rule-schema) | Fix the spec; SDK validates many fields client-side |
| **`findInCase` zero matches** | `CaseLookupError` with `.matches.length === 0` | Predicate too narrow; capture didn't include that hop | Relax predicate; re-capture |
| **`findInCase` multiple matches** | `CaseLookupError` with `.matches.length > 1` | Predicate too broad; capture has >1 matching hop | Narrow predicate with `path` / `pathPrefix` / `bodyJsonPath`; or use `findAllInCase` |

### Example

```ts
import {
  SoftprobeError,
  RuntimeError,
  SoftprobeRuntimeUnreachableError,
  SoftprobeUnknownSessionError,
  CaseLookupError,
  CaseLoadError,
} from '@softprobe/softprobe-js';

try {
  const hit = session.findInCase({ direction: 'outbound', hostSuffix: 'stripe.com' });
} catch (e) {
  if (e instanceof CaseLookupError) {
    console.error(
      `findInCase: ${e.matches.length} matches:`,
      e.matches.map((m) => m.spanId),
    );
  } else if (e instanceof SoftprobeRuntimeUnreachableError) {
    console.error(`runtime unreachable: ${e.message}`);
  } else if (e instanceof SoftprobeUnknownSessionError) {
    console.error(`unknown session at ${e.url}: ${e.body}`);
  } else if (e instanceof RuntimeError) {
    console.error(`runtime ${e.status} at ${e.url}: ${e.body}`);
  } else if (e instanceof CaseLoadError) {
    console.error(`case load failed: ${e.path}: ${e.message}`);
  }
  throw e;
}
```

### Class hierarchy

| Class | Extends | When thrown |
|---|---|---|
| `SoftprobeError` | `Error` | Base class; catch this to catch everything |
| `SoftprobeRuntimeUnreachableError` | `SoftprobeError` | Transport failure before HTTP (no `status` / `body`) |
| `RuntimeError` | `SoftprobeError` | Runtime returned non-2xx. Fields: `status`, `body`, `url` |
| `SoftprobeUnknownSessionError` | `RuntimeError` | Runtime returned 404 for unknown session |
| `CaseLookupError` | `SoftprobeError` | `findInCase` saw 0 or >1 matches. Field: `matches: Span[]` |
| `CaseLoadError` | `SoftprobeError` | `loadCaseFromFile` failed to parse / validate. Field: `path` |
| `HookExecutionError` | `SoftprobeError` | A suite hook threw; fields include hook name and kind (`hook-runner.ts`) |

Legacy names `SoftprobeRuntimeError`, `SoftprobeCaseLoadError`, and `SoftprobeCaseLookupAmbiguityError` are aliases of `RuntimeError`, `CaseLoadError`, and `CaseLookupError` respectively (`errors.ts`).

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

The npm package uses **2.x** semver (`package.json` / exported `VERSION` in `src/version.ts`); the Go runtime and `softprobe` CLI use the **0.5.x** platform line until release tagging catches up. Pair a given npm release with a runtime built from the same commit or release notes.

| `@softprobe/softprobe-js` (npm) | `softprobe-runtime` / CLI | Spec |
|---|---|---|
| `2.0.x` (e.g. `2.0.10`) | `0.5.x` expected; dev builds may show `0.0.0-dev` | http-control-api v1 |
| `0.4.x` (legacy) | `0.4.x` – `0.5.x` | v1 |

`softprobe doctor` reports drift.

## See also

- [Replay in Jest](/guides/replay-in-jest) — tutorial
- [TypeScript SDK on npm](https://www.npmjs.com/package/@softprobe/softprobe-js)
- [HTTP control API](/reference/http-control-api) — wire-level spec the SDK calls
