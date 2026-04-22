import path from 'path';

import { Softprobe } from '@softprobe/softprobe-js';

const runtimeUrl = process.env.SOFTPROBE_RUNTIME_URL ?? 'http://127.0.0.1:8080';
const appUrl = process.env.APP_URL ?? 'http://127.0.0.1:8081';
const apiToken = process.env.SOFTPROBE_API_KEY ?? undefined;
const softprobe = new Softprobe({ baseUrl: runtimeUrl, apiToken });

async function isReachable(url: string): Promise<boolean> {
  try {
    const r = await fetch(url, { signal: AbortSignal.timeout(2000) });
    return r.ok;
  } catch {
    return false;
  }
}

describe('fragment replay', () => {
  let sessionId = '';
  let close = async (): Promise<void> => {};

  beforeAll(async () => {
    if (!(await isReachable(`${runtimeUrl}/health`))) {
      return; // tests will be skipped via sessionId guard
    }
    const session = await softprobe.startSession({ mode: 'replay' });
    sessionId = session.id;
    close = () => session.close();

    await session.loadCaseFromFile(
      path.join(__dirname, '../../spec/examples/cases/fragment-happy-path.case.json')
    );

    // Pure in-memory lookup against the loaded case (`docs/design.md` §3.2.3).
    // Returns the captured response so the test author can inspect or mutate
    // it before registering the mock rule.
    const hit = session.findInCase({
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
    });

    await session.mockOutbound({
      id: 'fragment-replay',
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
    if (!sessionId) {
      console.log(`Skipping: runtime unreachable at ${runtimeUrl}`);
      return;
    }
    if (!(await isReachable(`${appUrl}/health`))) {
      console.log(`Skipping: app unreachable at ${appUrl}`);
      return;
    }
    const response = await fetch(`${appUrl}/hello`, {
      headers: {
        'x-softprobe-session-id': sessionId,
      },
    });

    expect(response.status).toBe(200);
    // The SUT composes its response from the mocked /fragment dependency;
    // `dep` coming through proves MockOutbound replaced the live upstream.
    expect(await response.json()).toEqual({ message: 'hello', dep: 'ok' });
  });
});
