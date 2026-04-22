# Quick start

Get a real capture-and-replay loop working in **about 5 minutes**. By the end you will have:

- a Softprobe session running against the hosted runtime,
- a captured `*.case.json` file for a sample request,
- a green Jest test that replays the capture without hitting the live upstream.

::: tip Self-hosted option
Prefer to run the runtime yourself? Jump to [Quick start: local (Docker)](#quick-start-local-docker) at the bottom of this page.
:::

::: info Scope
This quick start uses the **hosted runtime** path — no containers required.
If you are integrating the Node **language instrumentation** path instead of the proxy, see [Proxy vs language instrumentation](/guides/proxy-vs-language-instrumentation) first.
:::

## Prerequisites

| Tool | Version | Why |
|---|---|---|
| Node.js | 20+ | Runs the Jest example |
| `softprobe` CLI | latest | Creates sessions, drives capture |

Install the CLI:

```bash
# macOS / Linux
curl -fsSL https://softprobe.dev/install/cli.sh | sh

# npm (cross-platform)
npm install -g @softprobe/cli
```

## 1. Get an API key

1. Sign up at [app.softprobe.dev](https://app.softprobe.dev) (Google or GitHub — no credit card).
2. Copy your API key from the dashboard. It looks like `sk_live_…`.

## 2. Configure the CLI

```bash
export SOFTPROBE_RUNTIME_URL=https://runtime.softprobe.dev
export SOFTPROBE_API_KEY=sk_live_...
export SOFTPROBE_API_TOKEN=$SOFTPROBE_API_KEY   # SDKs and CLI read SOFTPROBE_API_TOKEN

softprobe doctor
# ✓ runtime reachable at https://runtime.softprobe.dev
# ✓ authenticated as <your-org>
# ✓ spec version matches CLI (http-control-api@v1)
```

All three lines in your shell profile. `doctor` passes green → you're ready.

## 3. Start a capture session

```bash
SESSION_ID=$(softprobe session start --mode capture --json | jq -r .sessionId)
echo "Session: $SESSION_ID"
```

## 4. Drive a real request

Send a request to your app (or any HTTP service) through the Softprobe sidecar, carrying the session header so the proxy knows which session to attribute the spans to. For the purposes of this quick start, we'll use the sample app from the e2e harness:

```bash
# If you have the compose stack running locally:
curl -H "x-softprobe-session-id: $SESSION_ID" http://127.0.0.1:8082/hello

# Or against any service you're already running that has the WASM sidecar.
```

::: tip No proxy yet?
If you haven't set up the WASM sidecar, you can still use the hosted runtime for
**API-first testing** — write rules directly with `session rules apply` and issue
SDK `mockOutbound` calls. Full proxy capture is the richest path, but it's not
required to get value from replay.
:::

## 5. Close the session

```bash
softprobe session close $SESSION_ID
```

On close, the hosted runtime reads all captured OTLP spans, assembles them into a case file, and stores it in your tenant's GCS bucket. Fetch it locally:

```bash
softprobe cases get $SESSION_ID --out my-app.case.json
```

Or, if you're using the sample compose stack, the case is also written to `e2e/captured.case.json` on disk.

## 6. Install the TypeScript SDK

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

## 7. Write the replay test — two paths

Pick one. Both run against the same case file and pass the same assertion.

### Path A — Codegen (recommended)

Let the CLI generate a session helper from the case file. You commit the generator output alongside the case.

```bash
softprobe generate jest-session \
  --case my-app.case.json \
  --out test/generated/my-app.replay.session.ts
```

Then write a short test that imports the generated helper:

```ts
// my-app.replay.test.ts
import { startReplaySession } from './test/generated/my-app.replay.session';

describe('my-app replay', () => {
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

The generated helper contains one `findInCase` + `mockOutbound` pair per outbound hop in the case. Regenerate any time the capture changes.

### Path B — Ad-hoc `findInCase` + `mockOutbound`

Write the session setup by hand when you need to mutate a captured response, express ordered responses, or use predicate-based matching beyond `(direction, method, path)` triples.

```ts
import path from 'path';
import { Softprobe } from '@softprobe/softprobe-js';

const softprobe = new Softprobe();  // reads SOFTPROBE_RUNTIME_URL and SOFTPROBE_API_TOKEN from env

describe('my-app replay', () => {
  let sessionId = '';
  let close = async () => {};

  beforeAll(async () => {
    const session = await softprobe.startSession({ mode: 'replay' });
    sessionId = session.id;
    close = () => session.close();

    await session.loadCaseFromFile(path.resolve(__dirname, 'my-app.case.json'));

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

## 8. Run it

```bash
SOFTPROBE_RUNTIME_URL=https://runtime.softprobe.dev \
SOFTPROBE_API_TOKEN=sk_live_... \
npx jest
```

Expected output:

```
 PASS  ./my-app.replay.test.ts
  my-app replay
    ✓ replays /fragment through the mesh (27 ms)
```

## You're done

You have a working hosted capture-replay loop. From here:

| I want to… | Read |
|---|---|
| Understand what each moving part does | [Architecture](/concepts/architecture) |
| Set up the Envoy proxy sidecar | [Installation: Proxy](/installation#proxy) |
| Rewrite masked PII before replay | [Write a hook](/guides/write-a-hook) |
| Run thousands of cases in CI | [Run a suite at scale](/guides/run-a-suite-at-scale) |
| Do the same thing in Python / Java / Go | [Replay in pytest](/guides/replay-in-pytest), [JUnit](/guides/replay-in-junit), [Go](/guides/replay-in-go) |
| Use self-hosted runtime instead | [Local quick start](#quick-start-local-docker) below |

---

## Quick start: local (Docker)

If you prefer to run the runtime yourself — no internet dependency, no account required.

### Prerequisites

| Tool | Version | Why |
|---|---|---|
| Docker | 24+ | Runs the runtime, proxy, sample app, and sample upstream |
| Docker Compose | v2 (bundled) | Orchestrates the five services |
| Node.js | 20+ | Runs the Jest example |

### 1. Clone the starter repository

```bash
git clone https://github.com/softprobe/softprobe.git
cd softprobe
```

### 2. Start the stack

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

When the command returns with no errors, all health checks passed. Verify:

```bash
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8081/health
```

### 3. Capture a real request

```bash
SESSION_ID=$(curl -s -X POST http://127.0.0.1:8080/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"mode":"capture"}' | jq -r .sessionId)

echo "Session: $SESSION_ID"

curl -s -H "x-softprobe-session-id: $SESSION_ID" http://127.0.0.1:8082/hello

curl -s -X POST "http://127.0.0.1:8080/v1/sessions/$SESSION_ID/close"

ls -la e2e/captured.case.json
```

### 4–8. Write and run the replay test

Follow steps 4–8 from the hosted path above, using:
- `SOFTPROBE_RUNTIME_URL=http://127.0.0.1:8080` (no API key needed)
- `path.resolve(__dirname, '../softprobe/e2e/captured.case.json')` for the case path

## Troubleshooting

**`softprobe doctor` fails with `401 Unauthorized`** — your `SOFTPROBE_API_TOKEN` doesn't match the key in the dashboard. Re-copy the key and set all three export lines from step 2.

**`curl: (7) Failed to connect to 127.0.0.1 port 8080`** — the local stack isn't up yet. Re-run the `docker compose … --wait` command and watch for health-check failures.

**`findInCase threw: 0 matches`** — the capture step didn't include the span you're looking for. Open the case file and search for the path you expected; if it's missing, re-capture while the upstream is running.

**Test hangs on `fetch(…/hello)`** — the SUT may not be routing egress through the proxy. Check that the `app` container has `EGRESS_PROXY_URL` set.

More at [Troubleshooting](/guides/troubleshooting).
