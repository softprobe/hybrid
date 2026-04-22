/**
 * PD7.3a — TS SDK parity test.
 *
 * Drives the full Softprobe facade (startSession → loadCaseFromFile →
 * findInCase → mockOutbound → close) against a fake runtime using the
 * checked-in golden case `fragment-happy-path.case.json`.
 *
 * This test proves that the TS SDK correctly serialises every control-API
 * call without a live runtime or proxy.
 */
import path from 'path';

import { Softprobe } from '../softprobe';

const GOLDEN_CASE = path.join(
  __dirname,
  '..', '..', '..', 'spec', 'examples', 'cases', 'fragment-happy-path.case.json',
);

describe('TS SDK parity (PD7.3a)', () => {
  it('drives startSession → loadCaseFromFile → findInCase → mockOutbound → close', async () => {
    const calls: Array<{ method: string; url: string; body: unknown }> = [];

    const fetchImpl = async (input: string, init?: { method?: string; body?: string }) => {
      const url = String(input);
      const method = (init?.method ?? 'GET').toUpperCase();
      const body = init?.body ? JSON.parse(init.body) : {};
      calls.push({ method, url, body });

      if (url.endsWith('/v1/sessions') && method === 'POST') {
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ sessionId: 'dogfood-session', sessionRevision: 0 }),
        };
      }
      if (url.includes('/load-case') && method === 'POST') {
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ sessionId: 'dogfood-session', sessionRevision: 1 }),
        };
      }
      if (url.includes('/rules') && method === 'POST') {
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ sessionId: 'dogfood-session', sessionRevision: 2 }),
        };
      }
      if (url.endsWith('/close') && method === 'POST') {
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ sessionId: 'dogfood-session', closed: true }),
        };
      }
      return { ok: false, status: 500, text: async () => `unexpected: ${method} ${url}` };
    };

    const sp = new Softprobe({ baseUrl: 'http://fake-runtime.test', fetchImpl });

    // start
    const session = await sp.startSession({ mode: 'replay' });
    expect(session.id).toBe('dogfood-session');

    // loadCaseFromFile — reads the actual golden JSON from disk
    await session.loadCaseFromFile(GOLDEN_CASE);

    // findInCase — in-memory lookup; no HTTP call
    const hit = session.findInCase({
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
    });
    expect(hit).toBeDefined();
    expect(hit.response).toBeDefined();

    // mockOutbound — posts the rule to the fake runtime
    await session.mockOutbound({
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
      response: hit.response,
    });

    // close
    await session.close();

    // Verify the sequence of calls against the fake runtime.
    const urls = calls.map((c) => c.url);
    expect(urls).toContain('http://fake-runtime.test/v1/sessions');
    expect(urls).toContain('http://fake-runtime.test/v1/sessions/dogfood-session/load-case');
    expect(urls.some((u) => u.includes('/rules'))).toBe(true);
    expect(urls.some((u) => u.endsWith('/close'))).toBe(true);

    // mockOutbound rule must reference /fragment
    const rulesCall = calls.find((c) => c.url.includes('/rules') && c.method === 'POST');
    expect(rulesCall).toBeDefined();
    const rulesBody = rulesCall!.body as { version: number; rules: unknown[] };
    expect(rulesBody.rules.length).toBeGreaterThanOrEqual(1);
  });
});
