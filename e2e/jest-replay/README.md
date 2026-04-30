# Jest replay quickstart

## Prerequisites

This example expects the Softprobe environment to already be running:

- `softprobe-runtime` on `http://127.0.0.1:8080`
- the app under test on `http://127.0.0.1:8081`
- the live e2e stack started with `docker compose -f e2e/docker-compose.yaml up -d --wait` from the repo root

If you have not started the stack yet, run that compose command first, then run the Jest command below.

Before running the example, verify the runtime contract with:

```bash
softprobe doctor --runtime-url http://127.0.0.1:8080
```

## Quick Start

This is the canonical first-stage Jest flow for Softprobe:

1. Run `softprobe doctor` against the runtime.
2. Create a replay session with `Softprobe`.
3. Load a captured case with `loadCaseFromFile`.
4. Pick the captured response with `findInCase`, then register it as a dependency mock with `mockOutbound` (mutate the response freely in between if needed).
5. Run your app under test with `x-softprobe-session-id`.
6. Close the session in `afterAll`.

This mirrors the SDK-side materialization flow in `docs/design.md` §5.3:
the runtime owns session state, while the Jest test materializes one captured
dependency response into an explicit mock rule before driving the SUT.

Copy the test below into your Jest suite and run `npm test`.

```ts
import path from 'path';
import { Softprobe } from '@softprobe/softprobe-js';

const runtimeUrl = process.env.SOFTPROBE_RUNTIME_URL ?? 'http://127.0.0.1:8080';
const appUrl = process.env.APP_URL ?? 'http://127.0.0.1:8081';
const softprobe = new Softprobe({ baseUrl: runtimeUrl });

const generatedDir = path.dirname(__filename);

describe('fragment replay', () => {
  let sessionId: string;
  let close: () => Promise<void>;

  beforeAll(async () => {
    const session = await softprobe.startSession({ mode: 'replay' });
    sessionId = session.id;
    close = () => session.close();

    await session.loadCaseFromFile(path.join(generatedDir, '../../spec/examples/cases/fragment-happy-path.case.json'));

    // Pure in-memory lookup against the loaded case — see docs/design.md §5.3.
    const hit = session.findInCase({
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
    });

    await session.mockOutbound({
      name: 'fragment-replay',
      priority: 100,
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
      response: hit.response,
    });
  });

  afterAll(async () => {
    await close();
  });

  it('replays the fragment dependency through the mesh', async () => {
    const response = await fetch(`${appUrl}/hello`, {
      headers: {
        'x-softprobe-session-id': sessionId,
      },
    });

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ message: 'hello', dep: 'ok' });
  });
});
```

The SDK methods above compile to the JSON control API. They do not emit raw `fetch` calls to `/v1/sessions`.

## Run it

From [`e2e/jest-replay/`](.):

```bash
npm test
```

If you want to run the test file directly, the equivalent command is:

```bash
SOFTPROBE_RUNTIME_URL=http://127.0.0.1:8080 \
APP_URL=http://127.0.0.1:8081 \
npx jest --config jest.config.js --runInBand fragment.replay.test.ts
```
