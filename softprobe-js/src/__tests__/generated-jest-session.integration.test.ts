import http from 'http';

import { startReplaySession } from './fixtures/captured.replay.session';

describe('generated jest session module', () => {
  it('imports and runs against a live control runtime', async () => {
    const requests: Array<{ path: string; body: unknown }> = [];
    const server = http.createServer((req, res) => {
      const chunks: Buffer[] = [];
      req.on('data', (chunk) => chunks.push(Buffer.from(chunk)));
      req.on('end', () => {
        const bodyText = Buffer.concat(chunks).toString('utf8');
        const body = bodyText ? JSON.parse(bodyText) : {};
        requests.push({ path: req.url ?? '', body });

        if (req.method === 'POST' && req.url === '/v1/sessions') {
          res.writeHead(200, { 'content-type': 'application/json' });
          res.end(JSON.stringify({ sessionId: 'sess_test', sessionRevision: 0 }));
          return;
        }
        if (req.method === 'POST' && req.url === '/v1/sessions/sess_test/load-case') {
          res.writeHead(200, { 'content-type': 'application/json' });
          res.end(JSON.stringify({ sessionId: 'sess_test', sessionRevision: 1 }));
          return;
        }
        if (req.method === 'POST' && req.url === '/v1/sessions/sess_test/rules') {
          res.writeHead(200, { 'content-type': 'application/json' });
          res.end(JSON.stringify({ sessionId: 'sess_test', sessionRevision: requests.length }));
          return;
        }
        if (req.method === 'POST' && req.url === '/v1/sessions/sess_test/close') {
          res.writeHead(200, { 'content-type': 'application/json' });
          res.end(JSON.stringify({ sessionId: 'sess_test', closed: true }));
          return;
        }

        res.writeHead(404, { 'content-type': 'application/json' });
        res.end(JSON.stringify({ error: 'unexpected request' }));
      });
    });

    await new Promise<void>((resolve) => server.listen(0, resolve));
    const address = server.address();
    if (address === null || typeof address === 'string') {
      server.close();
      throw new Error('failed to bind test server');
    }

    const previousBaseUrl = process.env.SOFTPROBE_RUNTIME_URL;
    process.env.SOFTPROBE_RUNTIME_URL = `http://127.0.0.1:${address.port}`;

    try {
      const session = await startReplaySession();
      expect(session.sessionId).toBe('sess_test');
      await session.close();
    } finally {
      if (previousBaseUrl === undefined) {
        delete process.env.SOFTPROBE_RUNTIME_URL;
      } else {
        process.env.SOFTPROBE_RUNTIME_URL = previousBaseUrl;
      }
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }

    expect(requests.map((request) => request.path)).toEqual([
      '/v1/sessions',
      '/v1/sessions/sess_test/load-case',
      '/v1/sessions/sess_test/rules',
      '/v1/sessions/sess_test/rules',
      '/v1/sessions/sess_test/close',
    ]);

    // First `mockOutbound` call: one rule for /fragment, materialized from the case.
    const firstRules = (requests[2].body as { rules: Array<Record<string, unknown>> }).rules;
    expect(firstRules).toHaveLength(1);
    expect(firstRules[0]).toMatchObject({
      when: { direction: 'outbound', method: 'GET', path: '/fragment' },
      then: {
        action: 'mock',
        response: {
          status: 200,
          body: '{"dep":"ok"}\n',
        },
      },
    });
    const firstResponse = (firstRules[0].then as { response: { headers: Record<string, string> } }).response;
    expect(firstResponse.headers['content-type']).toBe('application/json');

    // Second `mockOutbound` call: appends /hello.
    const secondRules = (requests[3].body as { rules: Array<Record<string, unknown>> }).rules;
    expect(secondRules).toHaveLength(2);
    expect(secondRules[1]).toMatchObject({
      when: { direction: 'outbound', method: 'GET', path: '/hello' },
      then: {
        action: 'mock',
        response: {
          status: 200,
          body: '{"dependency":{"dep":"ok"},"message":"hello"}\n',
        },
      },
    });
  });
});
