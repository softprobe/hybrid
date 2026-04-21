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

    // `.name` now follows the canonical short class name (`RuntimeError`);
    // the legacy SoftprobeRuntimeError identifier still points at the same
    // constructor, so downstream `instanceof` checks keep working.
    await expect(client.sessions.close('missing')).rejects.toMatchObject({
      name: 'RuntimeError',
      status: 404,
      body: '{"error":"unknown session"}',
    });
    await expect(client.sessions.close('missing')).rejects.toBeInstanceOf(SoftprobeRuntimeError);
  });

  describe('bearer token authentication', () => {
    const envBackup = process.env.SOFTPROBE_API_TOKEN;
    afterEach(() => {
      if (envBackup === undefined) {
        delete process.env.SOFTPROBE_API_TOKEN;
      } else {
        process.env.SOFTPROBE_API_TOKEN = envBackup;
      }
    });

    it('attaches Authorization: Bearer from the apiToken option', async () => {
      const calls: Array<{ headers?: Record<string, string> }> = [];
      const client = createSoftprobeRuntimeClient({
        baseUrl: 'http://runtime.test',
        apiToken: 'sp_explicit_token',
        fetchImpl: async (_input, init) => {
          calls.push({ headers: init?.headers });
          return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 's', sessionRevision: 0 }) };
        },
      });

      await client.sessions.create({ mode: 'replay' });
      expect(calls[0].headers?.authorization).toBe('Bearer sp_explicit_token');
    });

    it('falls back to SOFTPROBE_API_TOKEN from the environment', async () => {
      process.env.SOFTPROBE_API_TOKEN = 'sp_env_token';
      const calls: Array<{ headers?: Record<string, string> }> = [];
      const client = createSoftprobeRuntimeClient({
        baseUrl: 'http://runtime.test',
        fetchImpl: async (_input, init) => {
          calls.push({ headers: init?.headers });
          return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 's', sessionRevision: 0 }) };
        },
      });

      await client.sessions.create({ mode: 'replay' });
      expect(calls[0].headers?.authorization).toBe('Bearer sp_env_token');
    });

    it('apiToken option overrides the SOFTPROBE_API_TOKEN env var', async () => {
      process.env.SOFTPROBE_API_TOKEN = 'sp_env_token';
      const calls: Array<{ headers?: Record<string, string> }> = [];
      const client = createSoftprobeRuntimeClient({
        baseUrl: 'http://runtime.test',
        apiToken: 'sp_explicit_wins',
        fetchImpl: async (_input, init) => {
          calls.push({ headers: init?.headers });
          return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 's', sessionRevision: 0 }) };
        },
      });

      await client.sessions.create({ mode: 'replay' });
      expect(calls[0].headers?.authorization).toBe('Bearer sp_explicit_wins');
    });

    it('sends no Authorization header when no token is configured', async () => {
      delete process.env.SOFTPROBE_API_TOKEN;
      const calls: Array<{ headers?: Record<string, string> }> = [];
      const client = createSoftprobeRuntimeClient({
        baseUrl: 'http://runtime.test',
        fetchImpl: async (_input, init) => {
          calls.push({ headers: init?.headers });
          return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 's', sessionRevision: 0 }) };
        },
      });

      await client.sessions.create({ mode: 'replay' });
      expect(calls[0].headers?.authorization).toBeUndefined();
    });

    it('treats an empty apiToken or env var as no token', async () => {
      process.env.SOFTPROBE_API_TOKEN = '   ';
      const calls: Array<{ headers?: Record<string, string> }> = [];
      const client = createSoftprobeRuntimeClient({
        baseUrl: 'http://runtime.test',
        apiToken: '',
        fetchImpl: async (_input, init) => {
          calls.push({ headers: init?.headers });
          return { ok: true, status: 200, text: async () => JSON.stringify({ sessionId: 's', sessionRevision: 0 }) };
        },
      });

      await client.sessions.create({ mode: 'replay' });
      expect(calls[0].headers?.authorization).toBeUndefined();
    });
  });
});
