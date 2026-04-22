import { Softprobe, SoftprobeSession } from '../softprobe';

describe('SoftprobeSession runtime HTTP facade (PD6.5b)', () => {
  it('routes startSession, loadCase, mockOutbound, clearRules, and close through the control API', async () => {
    const calls: Array<{ url: string; body: unknown }> = [];
    const fetchImpl = async (input: string, init?: { method?: string; body?: string }) => {
      const url = String(input);
      const body = init?.body ? JSON.parse(init.body) : {};
      calls.push({ url, body });
      if (url.endsWith('/v1/sessions') && init?.method === 'POST') {
        return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 'sess_pd65', sessionRevision: 0 }) };
      }
      if (url.includes('/load-case')) {
        return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 'sess_pd65', sessionRevision: 1 }) };
      }
      if (url.includes('/rules')) {
        return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 'sess_pd65', sessionRevision: 2 }) };
      }
      if (url.endsWith('/close')) {
        return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 'sess_pd65', closed: true }) };
      }
      return { ok: false, status: 500, text: async () => 'unexpected url' };
    };

    const sp = new Softprobe({ baseUrl: 'http://runtime.test', fetchImpl });
    const session = await sp.startSession({ mode: 'replay' });
    expect(session).toBeInstanceOf(SoftprobeSession);
    expect(session.id).toBe('sess_pd65');

    await session.loadCase({ version: '1.0.0', caseId: 'c1', traces: [] }, 'inline');
    await session.mockOutbound({
      direction: 'outbound',
      method: 'GET',
      path: '/x',
      response: { status: 200, body: { ok: true } },
    });
    await session.clearRules();
    await session.close();

    expect(calls.map((c) => c.url)).toEqual([
      'http://runtime.test/v1/sessions',
      'http://runtime.test/v1/sessions/sess_pd65/load-case',
      'http://runtime.test/v1/sessions/sess_pd65/rules',
      'http://runtime.test/v1/sessions/sess_pd65/rules',
      'http://runtime.test/v1/sessions/sess_pd65/close',
    ]);

    expect(calls[0].body).toEqual({ mode: 'replay' });
    expect(calls[1].body).toEqual({ version: '1.0.0', caseId: 'c1', traces: [] });
    expect(calls[2].body).toEqual({
      version: 1,
      rules: [
        {
          when: { direction: 'outbound', method: 'GET', path: '/x' },
          then: { action: 'mock', response: { status: 200, body: { ok: true } } },
        },
      ],
    });
    expect(calls[3].body).toEqual({ version: 1, rules: [] });
    expect(calls[4].body).toEqual({});
  });
});
