# Hooks — end-to-end examples

This folder is a working mini-project that shows all four hook kinds
supported by `@softprobe/softprobe-js`, each with:

1. A real hook implementation (TypeScript).
2. A `suite.yaml` that references the hook by name.
3. A Jest test that drives the suite end-to-end with `runSuite()`.
4. A small captured case used as input.

The tests under `__tests__/` are executed as part of the SDK's own Jest
run, so the examples stay in sync with the shipping SDK by construction —
if a hook example stops compiling or producing the expected output, CI
turns red.

## The four hook kinds

| Kind | When it runs | What it returns | Example file |
|---|---|---|---|
| `RequestHook` | Before replaying a captured inbound request at the SUT | A partial `{method,path,headers,body}` patch | `hooks/request.ts` |
| `MockResponseHook` | Before the proxy serves a captured outbound response back as a mock | A partial `{status,headers,body}` patch | `hooks/mock-response.ts` |
| `BodyAssertHook` | After the SUT returns, to assert on the response body | `Issue[]` (empty = pass) | `hooks/assert-body.ts` |
| `HeadersAssertHook` | After the SUT returns, to assert on response headers | `Issue[]` (empty = pass) | `hooks/assert-headers.ts` |

## Minimal `suite.yaml`

```yaml
name: checkout-nightly
version: 1

cases:
  - path: cases/checkout-happy.case.json

mocks:
  # The Jest runSuite adapter turns this into
  #   session.mockOutbound({ direction:"outbound", hostSuffix:"stripe.com",
  #                          pathPrefix:"/v1/tokens",
  #                          response: unmaskCard(capturedResponse) })
  - name: stripe-token
    match: { direction: outbound, hostSuffix: stripe.com, pathPrefix: /v1/tokens }
    hook: mock-response.unmaskCard
```

## Wiring hooks in a Jest test

```ts
import { runSuite } from '@softprobe/softprobe-js/suite';
import * as requestHooks    from '../hooks/request';
import * as mockHooks       from '../hooks/mock-response';
import * as assertBodyHooks from '../hooks/assert-body';

runSuite('../suites/checkout.suite.yaml', {
  hooks: {
    ...requestHooks,
    ...mockHooks,
    ...assertBodyHooks,
  },
  onCase: async ({ session, applyRequest, assertBody }) => {
    // Transform a captured inbound request, fire it at the SUT, then
    // hand the SUT response to a BodyAssertHook.
    const transformed = applyRequest('request.substituteCard', {
      method: 'POST',
      path: '/checkout',
      headers: { 'content-type': 'application/json' },
      body: '{"card":{"number":"4000000000000002"}}',
    });
    const response = await fetch('http://localhost:3000' + transformed.path, {
      method: transformed.method,
      headers: transformed.headers,
      body: transformed.body,
    });
    assertBody('assert-body.totalsMatchItems', await response.json(), {
      direction: 'inbound',
      path: '/checkout',
    });
  },
});
```

## Hook discipline

1. **Pure functions.** No network calls, no filesystem writes. Side-effectful
   hooks are bugs waiting to happen — use a setup step instead.
2. **Return partials.** Any key you omit keeps its captured value. Returning
   `{}` means "use the request/response as-is".
3. **Throw loudly.** A thrown exception fails the case with the hook name in
   the stack trace — silent fallbacks hide bugs.
4. **Keep hooks small.** If a hook is longer than ~30 lines, split it or
   rethink whether the suite YAML should grow.

See also:

- [`docs-site/guides/write-a-hook.md`](../../../docs-site/guides/write-a-hook.md) — authoring guide
- [`docs-site/reference/sdk-typescript.md#runSuite`](../../../docs-site/reference/sdk-typescript.md) — API reference
