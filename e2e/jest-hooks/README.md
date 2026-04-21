# Jest hooks + suite.yaml — end-to-end

This harness exercises `runSuite()` and the hook system **through the
real docker-compose stack**: `softprobe-runtime`, Envoy + Softprobe
WASM, the `app` SUT, and the `upstream` dependency. It is the smoke
test that proves hooks are not just wired up in TypeScript but also
actually flow through the mesh and change what the SUT sees.

## What it demonstrates

1. `suite.yaml` references a `MockResponseHook` by name (`hook: rewriteDep`).
2. `runSuite()` parses the YAML, resolves the hook from `options.hooks`,
   applies it to the captured `/fragment` response, and registers the
   transformed body as a mock rule on the real runtime container.
3. We fetch `GET /v1/sessions/{id}/state` from the runtime and decode
   the mock rule's `body` to prove the **hook-transformed** payload
   landed on the runtime — not the captured one.
4. We drive the SUT's `GET /hello` through the **ingress proxy on
   :8082** with the SDK-issued session id on `x-softprobe-session-id`.
   The ingress WASM translates that header into W3C `tracestate`,
   which the app propagates on its outbound `/fragment` call. The
   egress WASM reads the same session id back out of `tracestate`,
   keys `/v1/inject` by it, and the runtime returns the hook-mutated
   mock. We assert the SUT's response body reflects that mutation —
   closing the full loop:
   `suite.yaml → hooks registry → runtime → onCase → ingress → SUT →
    egress → /v1/inject → hook-mutated body → SUT response`.
5. A `BodyAssertHook` (`helloShape`) runs on the same response via the
   `onCase` handle, exercising the third hook kind end-to-end.

### Session id is opaque — proxy must not assume a format

The SDK owns the session-id namespace (today `sess_<base64url>`,
tomorrow anything else). The proxy must treat it as OPAQUE and must
never synthesize its own. A previous regression had the WASM filter
invent `sp-session-<trace-derived>` ids whenever no session id was
present on the request, which polluted the SDK namespace and caused
`/v1/inject` lookups to always miss. This is now pinned by unit tests
in `softprobe-proxy/src/otel.rs::session_id_tests` and by this
end-to-end assertion on the `/hello` response body.

## Prerequisites

Bring up the shared e2e stack from the repo root:

```bash
docker compose -f e2e/docker-compose.yaml up -d --wait
```

and verify the runtime:

```bash
softprobe doctor --runtime-url http://127.0.0.1:8080
```

## Run it

```bash
cd e2e/jest-hooks
npm install
npm test
```

## Layout

```
jest-hooks/
├── README.md
├── package.json
├── jest.config.js
├── tsconfig.json
├── fragment.hooks.test.ts     # drives runSuite() + assertions
├── hooks/
│   ├── rewrite-dep.ts         # MockResponseHook example
│   └── assert-hello.ts        # BodyAssertHook example
└── suites/
    └── fragment.suite.yaml    # references both hooks by name
```

## The canonical user-facing snippet

```ts
import path from 'path';
import { runSuite } from '@softprobe/softprobe-js/suite';
import { rewriteDep } from './hooks/rewrite-dep';
import { helloShape } from './hooks/assert-hello';

runSuite(path.join(__dirname, 'suites/fragment.suite.yaml'), {
  baseUrl: process.env.SOFTPROBE_RUNTIME_URL,
  hooks: { rewriteDep, helloShape },
  onCase: async ({ session, assertBody }) => {
    // Go through the ingress proxy so the WASM filter can translate
    // `x-softprobe-session-id` into `tracestate` for the egress hop.
    const response = await fetch(`${process.env.INGRESS_URL}/hello`, {
      headers: { 'x-softprobe-session-id': session.id },
    });
    const body = await response.json();
    expect(body).toEqual({ message: 'hello', dep: 'mutated-by-hook' });
    assertBody('helloShape', body, { direction: 'inbound', path: '/hello' });
  },
});
```

See `docs-site/guides/run-a-suite-at-scale.md` for the full user guide.
