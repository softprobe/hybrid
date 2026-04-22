# Proxy vs language instrumentation

This guide explains when to use each instrumentation model and shows a real workflow for both.

## Choose your model

| Question | Proxy instrumentation (recommended) | Language instrumentation (Node compatibility) |
|---|---|---|
| Where traffic is intercepted | Envoy + Softprobe WASM sidecar | In-process Node hooks/interceptors |
| Works across languages | Yes | No (Node only) |
| Requires app code changes | Usually no | Yes (`@softprobe/softprobe-js/init` at bootstrap) |
| Session/rule APIs | `SoftprobeSession` via runtime | Same `SoftprobeSession` via runtime |
| Capture artifact | `*.case.json` | `*.case.json` |

If you can run a sidecar/mesh, use proxy instrumentation. Use language instrumentation when proxy adoption is temporarily blocked and you still need replay in Node tests.

## Shared invariants (both models)

- Replay authorship stays in the SDK: `startSession`, `loadCaseFromFile`, `findInCase`, `mockOutbound`, `close`.
- Runtime is the source of truth for session state and rules.
- Case files are `*.case.json`.
- Tests should always clean up sessions with `close()` in teardown.

## Proxy instrumentation workflow (canonical)

Use this when your SUT is reached through Envoy:

1. Start replay session in test code.
2. Load a case and register outbound mocks.
3. Call the SUT through proxy ingress, with `x-softprobe-session-id`.
4. Assert response and close the session.

```ts
import path from 'path';
import { Softprobe } from '@softprobe/softprobe-js';

const softprobe = new Softprobe({ baseUrl: 'http://127.0.0.1:8080' });

const session = await softprobe.startSession({ mode: 'replay' });
await session.loadCaseFromFile(path.resolve('cases/fragment-happy-path.case.json'));

const hit = session.findInCase({
  direction: 'outbound',
  method: 'GET',
  path: '/fragment',
});

await session.mockOutbound({
  direction: 'outbound',
  method: 'GET',
  path: '/fragment',
  response: hit.response,
});

const res = await fetch('http://127.0.0.1:8082/hello', {
  headers: { 'x-softprobe-session-id': session.id },
});
```

## Language instrumentation workflow (Node)

Use this when you cannot run proxy interception yet but need Node replay/capture behavior in-process.

### 1) Bootstrap instrumentation first

Load Softprobe before your app framework and before OTel auto-instrumentation:

```ts
import '@softprobe/softprobe-js/init';
import { applyLegacyFrameworkPatches } from '@softprobe/softprobe-js/legacy';
import { NodeSDK } from '@opentelemetry/sdk-node';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';

// Optional migration path for require-based Express/Fastify auto-patching.
applyLegacyFrameworkPatches();

const sdk = new NodeSDK({ instrumentations: [getNodeAutoInstrumentations()] });
sdk.start();
```

### 2) Keep replay authoring identical to proxy mode

Your test code still uses runtime sessions and SDK APIs:

```ts
const session = await softprobe.startSession({ mode: 'replay' });
await session.loadCaseFromFile('cases/checkout.case.json');
const hit = session.findInCase({ direction: 'outbound', method: 'POST', pathPrefix: '/v1/payment_intents' });
await session.mockOutbound({ direction: 'outbound', method: 'POST', pathPrefix: '/v1/payment_intents', response: hit.response });
```

### 3) Important guardrail

Default `@softprobe/softprobe-js/init` does **not** accept `SOFTPROBE_MODE` or `SOFTPROBE_DATA_DIR`.  
Behavior is controlled through runtime sessions (same control-plane semantics as proxy mode), not init env toggles.

## Migration recommendation

- Start new deployments with proxy instrumentation.
- If a Node service is already using in-process instrumentation, keep test authoring in `SoftprobeSession` APIs so later migration to proxy mode is mostly a traffic-routing change, not a test rewrite.

## Next

| I want to… | Read |
|---|---|
| Run the canonical sidecar setup | [Quick start](/quickstart) |
| Write a replay test in Jest | [Replay in a Jest test](/guides/replay-in-jest) |
| Understand control/data-plane boundaries | [Architecture](/concepts/architecture) |
