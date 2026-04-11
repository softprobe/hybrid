import { createSoftprobeRuntimeClient, SoftprobeRuntimeError } from '../runtime-client';

describe('SoftprobeRuntimeClient', () => {
  it('posts session create, load-case, and close requests to the control runtime', async () => {
    const calls: Array<{ input: string; init?: { method?: string; headers?: Record<string, string>; body?: string } }> = [];
    const client = createSoftprobeRuntimeClient({
      baseUrl: 'http://runtime.test',
      fetchImpl: async (input, init) => {
        calls.push({ input, init });
        if (String(input).endsWith('/close')) {
          return {
            ok: true,
            status: 200,
            text: async () => JSON.stringify({ sessionId: 'sess_123', closed: true }),
          };
        }
        return {
          ok: true,
          status: 200,
          text: async () => JSON.stringify({ sessionId: 'sess_123', sessionRevision: calls.length - 1 }),
        };
      },
    });

    const created = await client.sessions.create({ mode: 'replay' });
    const loaded = await client.sessions.loadCase('sess_123', { version: '1.0.0', caseId: 'checkout', traces: [] });
    const closed = await client.sessions.close('sess_123');

    expect(created).toEqual({ sessionId: 'sess_123', sessionRevision: 0 });
    expect(loaded).toEqual({ sessionId: 'sess_123', sessionRevision: 1 });
    expect(closed).toEqual({ sessionId: 'sess_123', closed: true });

    expect(calls).toHaveLength(3);
    expect(calls[0].input).toBe('http://runtime.test/v1/sessions');
    expect(calls[0].init?.method).toBe('POST');
    expect(calls[0].init?.headers).toEqual({ 'content-type': 'application/json' });
    expect(JSON.parse(calls[0].init?.body ?? '{}')).toEqual({ mode: 'replay' });

    expect(calls[1].input).toBe('http://runtime.test/v1/sessions/sess_123/load-case');
    expect(JSON.parse(calls[1].init?.body ?? '{}')).toEqual({ version: '1.0.0', caseId: 'checkout', traces: [] });

    expect(calls[2].input).toBe('http://runtime.test/v1/sessions/sess_123/close');
    expect(JSON.parse(calls[2].init?.body ?? '{}')).toEqual({});
  });

  it('surfaces a stable error type with status and body on non-2xx responses', async () => {
    const client = createSoftprobeRuntimeClient({
      baseUrl: 'http://runtime.test',
      fetchImpl: async () => ({
        ok: false,
        status: 404,
        text: async () => '{"error":"unknown session"}',
      }),
    });

    await expect(client.sessions.close('missing')).rejects.toMatchObject({
      name: 'SoftprobeRuntimeError',
      status: 404,
      body: '{"error":"unknown session"}',
    });
    await expect(client.sessions.close('missing')).rejects.toBeInstanceOf(SoftprobeRuntimeError);
  });
});
