import { mkdtemp, rm, writeFile } from 'fs/promises';
import os from 'os';
import path from 'path';

import { Softprobe } from '../softprobe';

describe('Softprobe', () => {
  it('starts sessions, loads cases from file, and accumulates rules before posting them', async () => {
    const calls: Array<{ input: string; init?: { method?: string; headers?: Record<string, string>; body?: string } }> = [];
    const softprobe = new Softprobe({
      baseUrl: 'http://runtime.test',
      fetchImpl: async (input, init) => {
        calls.push({ input, init });
        const pathname = new URL(input).pathname;

        if (pathname === '/v1/sessions') {
          return jsonResponse({ sessionId: 'sess_123', sessionRevision: 0 });
        }
        if (pathname.endsWith('/load-case')) {
          return jsonResponse({ sessionId: 'sess_123', sessionRevision: 1 });
        }
        if (pathname.endsWith('/rules')) {
          return jsonResponse({ sessionId: 'sess_123', sessionRevision: calls.length });
        }
        if (pathname.endsWith('/close')) {
          return jsonResponse({ sessionId: 'sess_123', closed: true });
        }

        throw new Error(`unexpected request: ${pathname}`);
      },
    });

    const session = await softprobe.startSession({ mode: 'replay' });
    expect(session.id).toBe('sess_123');
    expect(softprobe.attach(session.id).id).toBe(session.id);

    const tmpDir = await mkdtemp(path.join(os.tmpdir(), 'softprobe-sdk-'));
    const casePath = path.join(tmpDir, 'fragment.case.json');
    const caseDocument = {
      version: '1.0.0',
      caseId: 'fragment',
      traces: [
        {
          resourceSpans: [
            {
              resource: {
                attributes: [{ key: 'service.name', value: { stringValue: 'api' } }],
              },
              scopeSpans: [
                {
                  spans: [
                    {
                      traceId: 'abc',
                      spanId: 'span1',
                      name: 'HTTP GET',
                      attributes: [
                        { key: 'sp.span.type', value: { stringValue: 'inject' } },
                        { key: 'sp.traffic.direction', value: { stringValue: 'outbound' } },
                        { key: 'http.request.method', value: { stringValue: 'GET' } },
                        { key: 'url.path', value: { stringValue: '/fragment' } },
                        { key: 'http.response.status_code', value: { intValue: 200 } },
                        { key: 'http.response.body', value: { stringValue: '{"dep":"ok"}' } },
                      ],
                    },
                  ],
                },
              ],
            },
          ],
        },
      ],
    };
    await writeFile(casePath, JSON.stringify(caseDocument));

    await session.loadCaseFromFile(casePath);

    await session.mockOutbound({
      id: 'mock-fragment',
      priority: 10,
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
      response: { status: 200, body: { ok: true } },
    });
    await session.mockOutbound({
      id: 'mock-fragment-prefix',
      priority: 20,
      direction: 'outbound',
      pathPrefix: '/fragment',
      response: { status: 201, body: { ok: true } },
    });

    const fragment = session.findInCase({
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
    });
    expect(fragment.response).toEqual({
      status: 200,
      headers: {},
      body: '{"dep":"ok"}',
    });

    await session.mockOutbound({
      id: 'mock-from-case',
      priority: 30,
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
      response: fragment.response,
    });

    await session.clearRules();
    await session.close();

    expect(calls).toHaveLength(7);
    expect(calls[0].input).toBe('http://runtime.test/v1/sessions');
    expect(JSON.parse(calls[0].init?.body ?? '{}')).toEqual({ mode: 'replay' });

    expect(calls[1].input).toBe(`http://runtime.test/v1/sessions/sess_123/load-case`);
    expect(JSON.parse(calls[1].init?.body ?? '{}')).toEqual(caseDocument);

    expect(calls[2].input).toBe(`http://runtime.test/v1/sessions/sess_123/rules`);
    expect(JSON.parse(calls[2].init?.body ?? '{}')).toEqual({
      version: 1,
      rules: [
        {
          id: 'mock-fragment',
          priority: 10,
          when: {
            direction: 'outbound',
            method: 'GET',
            path: '/fragment',
          },
          then: {
            action: 'mock',
            response: { status: 200, body: { ok: true } },
          },
        },
      ],
    });

    expect(JSON.parse(calls[3].init?.body ?? '{}')).toEqual({
      version: 1,
      rules: [
        {
          id: 'mock-fragment',
          priority: 10,
          when: {
            direction: 'outbound',
            method: 'GET',
            path: '/fragment',
          },
          then: {
            action: 'mock',
            response: { status: 200, body: { ok: true } },
          },
        },
        {
          id: 'mock-fragment-prefix',
          priority: 20,
          when: {
            direction: 'outbound',
            pathPrefix: '/fragment',
          },
          then: {
            action: 'mock',
            response: { status: 201, body: { ok: true } },
          },
        },
      ],
    });

    expect(JSON.parse(calls[4].init?.body ?? '{}')).toEqual({
      version: 1,
      rules: [
        {
          id: 'mock-fragment',
          priority: 10,
          when: {
            direction: 'outbound',
            method: 'GET',
            path: '/fragment',
          },
          then: {
            action: 'mock',
            response: { status: 200, body: { ok: true } },
          },
        },
        {
          id: 'mock-fragment-prefix',
          priority: 20,
          when: {
            direction: 'outbound',
            pathPrefix: '/fragment',
          },
          then: {
            action: 'mock',
            response: { status: 201, body: { ok: true } },
          },
        },
        {
          id: 'mock-from-case',
          priority: 30,
          when: {
            direction: 'outbound',
            method: 'GET',
            path: '/fragment',
          },
          then: {
            action: 'mock',
            response: {
              status: 200,
              headers: {},
              body: '{"dep":"ok"}',
            },
          },
        },
      ],
    });
    expect(JSON.parse(calls[5].init?.body ?? '{}')).toEqual({
      version: 1,
      rules: [],
    });
    expect(calls[6].input).toBe(`http://runtime.test/v1/sessions/sess_123/close`);
    expect(JSON.parse(calls[6].init?.body ?? '{}')).toEqual({});

    await rm(tmpDir, { recursive: true, force: true });
  });
});

function jsonResponse(body: unknown) {
  return {
    ok: true,
    status: 200,
    text: async () => JSON.stringify(body),
  };
}
