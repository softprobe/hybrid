# Quick start

Get a real capture-and-replay loop working in **about 10 minutes**. By the end you will have:

- a local Softprobe runtime + Envoy sidecar running,
- a captured `*.case.json` file for a sample request,
- a green Jest test that replays the capture without hitting the live upstream.

::: info Scope
This quick start is the **proxy instrumentation** path (Envoy + Softprobe WASM).  
If you are integrating the Node **language instrumentation** path, see [Proxy vs language instrumentation](/guides/proxy-vs-language-instrumentation) first.
:::

::: tip Prefer another language?
After you finish this walkthrough, see [Replay in pytest](/guides/replay-in-pytest), [JUnit](/guides/replay-in-junit), or [Go](/guides/replay-in-go). The capture half is identical; only the test file changes.
:::

## Prerequisites

| Tool | Version | Why |
|---|---|---|
| Docker | 24+ | Runs the runtime, proxy, sample app, and sample upstream |
| Docker Compose | v2 (bundled) | Orchestrates the five services |
| Node.js | 20+ | Runs the Jest example |

You do **not** need a Kubernetes cluster, Istio, or a hosted Softprobe account for this walkthrough.

## 1. Clone the starter repository

```bash
git clone https://github.com/softprobe/softprobe.git
cd softprobe
```

The starter contains a runnable sample in `e2e/`: one sample app, one sample upstream, an Envoy configured with the Softprobe WASM filter, and the runtime.

## 2. Start the stack

```bash
docker compose -f e2e/docker-compose.yaml up --build --wait
```

This brings up five services:

| Service | Port | Role |
|---|---|---|
| `softprobe-runtime` | `8080` | Control API + OTLP ingestion |
| `softprobe-proxy` | `8082` (ingress), `8084` (egress) | Envoy + Softprobe WASM |
| `app` | `8081` | Sample application under test |
| `upstream` | `8083` | Sample HTTP dependency the app calls |
| `test-runner` | — | Sanity health-check container |

When the command returns with no errors, all health checks passed. You can verify:

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8081/health
```

## 3. Capture a real request

Create a capture session, send one request through the proxy, and close the session. The runtime writes a case file.

```bash
# 1) Start a capture session, grab the sessionId
SESSION_ID=$(curl -s -X POST http://127.0.0.1:8080/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"mode":"capture"}' | jq -r .sessionId)

echo "Session: $SESSION_ID"

# 2) Drive the app THROUGH the ingress proxy, carrying the session header
curl -s -H "x-softprobe-session-id: $SESSION_ID" \
  http://127.0.0.1:8082/hello

# 3) Close the session — this flushes the captured traces to disk
curl -s -X POST "http://127.0.0.1:8080/v1/sessions/$SESSION_ID/close"

ls -la e2e/captured.case.json
```

You now have a case file on disk. Open it — it is plain JSON with an array of OTLP-shaped traces, one per HTTP hop (ingress `/hello`, egress `/fragment`).

::: info What just happened
The test client sent `GET /hello` through the **ingress** listener (`:8082`). Envoy forwarded it to the `app` container. The app then made an outbound `GET /fragment` call through the **egress** listener (`:8084`) to the `upstream` container. The Softprobe WASM filter reported every hop to the runtime over OTLP. On `close`, the runtime wrote those traces into `e2e/captured.case.json`.
:::

## 4. Install the TypeScript SDK

```bash
mkdir -p my-first-replay && cd my-first-replay
npm init -y
npm install --save-dev jest ts-jest @types/jest typescript
npm install --save-dev @softprobe/softprobe-js
```

Add a minimal `jest.config.js`:

```js
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
};
```

## 5. Write the replay test — two paths

Pick one. Both run against the same case file and pass the same assertion.

### Path A — Codegen (recommended)

Let the CLI generate a session helper from the case file. You commit the generator output alongside the case.

```bash
softprobe generate jest-session \
  --case ../softprobe/e2e/captured.case.json \
  --out test/generated/fragment.replay.session.ts
```

Then write a short test that imports the generated helper:

```ts
// fragment.replay.test.ts
import { startReplaySession } from './test/generated/fragment.replay.session';

describe('fragment replay', () => {
  let sessionId = '';
  let close = async () => {};

  beforeAll(async () => {
    const session = await startReplaySession();
    sessionId = session.sessionId;
    close = session.close;
  });

  afterAll(() => close());

  it('replays /fragment through the mesh', async () => {
    const res = await fetch('http://127.0.0.1:8082/hello', {
      headers: { 'x-softprobe-session-id': sessionId },
    });

    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ message: 'hello', dep: 'ok' });
  });
});
```

The generated helper contains one `findInCase` + `mockOutbound` pair per outbound hop in the case. Regenerate any time the capture changes. This is the [default happy path](/guides/generate-jest-session) — start here unless Path B's ad-hoc control is required.

### Path B — Ad-hoc `findInCase` + `mockOutbound`

Write the session setup by hand. Use this when you need to **mutate** a captured response, express **ordered responses**, or use **predicate-based** matching beyond `(direction, method, path)` triples.

Create `fragment.replay.test.ts`:

```ts
import path from 'path';
import { Softprobe } from '@softprobe/softprobe-js';

const softprobe = new Softprobe({ baseUrl: 'http://127.0.0.1:8080' });

describe('fragment replay', () => {
  let sessionId = '';
  let close = async () => {};

  beforeAll(async () => {
    const session = await softprobe.startSession({ mode: 'replay' });
    sessionId = session.id;
    close = () => session.close();

    await session.loadCaseFromFile(
      path.resolve(__dirname, '../softprobe/e2e/captured.case.json')
    );

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
  });

  afterAll(() => close());

  it('replays /fragment through the mesh', async () => {
    const res = await fetch('http://127.0.0.1:8082/hello', {
      headers: { 'x-softprobe-session-id': sessionId },
    });

    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ message: 'hello', dep: 'ok' });
  });
});
```

::: tip What Path B does
1. **`startSession({ mode: 'replay' })`** asks the runtime for a fresh session.
2. **`loadCaseFromFile`** uploads the case file to the runtime and parses it in the SDK.
3. **`findInCase`** is an in-memory lookup — it throws if zero or multiple spans match, so ambiguity surfaces at *authoring* time, not test time.
4. **`mockOutbound`** registers a concrete mock rule on the runtime, using the captured response we found.
5. The test hits the SUT with the session header. The sidecar intercepts the `/fragment` call and returns the mock instead of calling the live upstream.
:::

## 6. Run it

```bash
npx jest
```

Expected output:

```
 PASS  ./fragment.replay.test.ts
  fragment replay
    ✓ replays /fragment through the mesh (27 ms)
```

## 7. Prove that replay actually bypassed the upstream

Stop the upstream container and rerun the test — it should still pass, because `/fragment` is now served from the case:

```bash
docker compose -f ../softprobe/e2e/docker-compose.yaml stop upstream
npx jest
```

Start it back up when you're done:

```bash
docker compose -f ../softprobe/e2e/docker-compose.yaml start upstream
```

## You're done

You have a working capture-replay loop in under 10 minutes. From here:

| I want to… | Read |
|---|---|
| Understand what each moving part does | [Architecture](/concepts/architecture) |
| Capture a real production session and commit it | [Capture your first session](/guides/capture-your-first-session) |
| Rewrite masked PII before replay | [Write a hook](/guides/write-a-hook) |
| Run thousands of cases in CI | [Run a suite at scale](/guides/run-a-suite-at-scale) |
| Do the same thing in Python / Java / Go | [Replay in pytest](/guides/replay-in-pytest), [JUnit](/guides/replay-in-junit), [Go](/guides/replay-in-go) |

## Troubleshooting

**`curl: (7) Failed to connect to 127.0.0.1 port 8080`** — the stack isn't up yet. Re-run the `docker compose … --wait` command and watch for health-check failures.

**`findInCase threw: 0 matches`** — the capture step didn't include the span you're looking for. Open `e2e/captured.case.json` and search for `/fragment`; if it's missing, re-capture while the upstream is running.

**Test hangs on `fetch(…/hello)`** — the SUT may not be routing egress through the proxy. Check that the `app` container has `EGRESS_PROXY_URL=http://softprobe-proxy:8084` set.

More at [Troubleshooting](/guides/troubleshooting).
