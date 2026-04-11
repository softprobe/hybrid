import http from 'http';

import { createSoftprobeRuntimeClient } from '../runtime-client';

function listen(server: http.Server): Promise<{ url: string; close: () => Promise<void> }> {
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        reject(new Error('expected TCP server address'));
        return;
      }

      resolve({
        url: `http://127.0.0.1:${address.port}`,
        close: () =>
          new Promise((closeResolve, closeReject) => {
            server.close((error) => {
              if (error) {
                closeReject(error);
                return;
              }
              closeResolve();
            });
          }),
      });
    });
  });
}

describe('Jest reference flow', () => {
  it('creates a session and forwards x-softprobe-session-id to the SUT', async () => {
    const seenHeaders: string[] = [];

    const runtimeServer = http.createServer((req, res) => {
      if (req.method !== 'POST' || req.url !== '/v1/sessions') {
        res.statusCode = 404;
        res.end('not found');
        return;
      }

      res.setHeader('content-type', 'application/json');
      res.end(JSON.stringify({ sessionId: 'sess_jest_001', sessionRevision: 0 }));
    });

    const sutServer = http.createServer((req, res) => {
      const header = req.headers['x-softprobe-session-id'];
      seenHeaders.push(Array.isArray(header) ? header[0] : header ?? '');
      res.end('ok');
    });

    const runtime = await listen(runtimeServer);
    const sut = await listen(sutServer);

    try {
      const client = createSoftprobeRuntimeClient({ baseUrl: runtime.url });
      const session = await client.sessions.create({ mode: 'replay' });

      const response = await fetch(`${sut.url}/checkout`, {
        headers: {
          'x-softprobe-session-id': session.sessionId,
        },
      });

      expect(response.status).toBe(200);
      expect(await response.text()).toBe('ok');
      expect(seenHeaders).toEqual([session.sessionId]);
    } finally {
      await runtime.close();
      await sut.close();
    }
  });
});
