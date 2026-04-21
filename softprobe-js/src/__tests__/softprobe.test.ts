import { mkdtemp, rm, writeFile } from 'fs/promises';
import os from 'os';
import path from 'path';

import {
  Softprobe,
  SoftprobeCaseLoadError,
  SoftprobeCaseLookupAmbiguityError,
  SoftprobeRuntimeUnreachableError,
  SoftprobeUnknownSessionError,
} from '../softprobe';

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

  it('supports loadCase, findAllInCase, setPolicy, and setAuthFixtures', async () => {
    const calls: Array<{ input: string; init?: { method?: string; headers?: Record<string, string>; body?: string } }> = [];
    const softprobe = new Softprobe({
      baseUrl: 'http://runtime.test',
      fetchImpl: async (input, init) => {
        calls.push({ input, init });
        const pathname = new URL(input).pathname;

        if (pathname === '/v1/sessions') {
          return jsonResponse({ sessionId: 'sess_456', sessionRevision: 0 });
        }
        if (pathname.endsWith('/load-case') || pathname.endsWith('/policy') || pathname.endsWith('/fixtures/auth')) {
          return jsonResponse({ sessionId: 'sess_456', sessionRevision: calls.length - 1 });
        }

        throw new Error(`unexpected request: ${pathname}`);
      },
    });

    const session = await softprobe.startSession({ mode: 'replay' });
    const caseDocument = {
      version: '1.0.0',
      caseId: 'fragment',
      traces: [
        {
          resourceSpans: [
            {
              scopeSpans: [
                {
                  spans: [
                    {
                      spanId: 'span1',
                      attributes: [
                        { key: 'sp.span.type', value: { stringValue: 'inject' } },
                        { key: 'sp.traffic.direction', value: { stringValue: 'outbound' } },
                        { key: 'url.path', value: { stringValue: '/fragment' } },
                        { key: 'http.response.status_code', value: { intValue: 200 } },
                        { key: 'http.response.body', value: { stringValue: '{"dep":"one"}' } },
                      ],
                    },
                    {
                      spanId: 'span2',
                      attributes: [
                        { key: 'sp.span.type', value: { stringValue: 'extract' } },
                        { key: 'sp.traffic.direction', value: { stringValue: 'outbound' } },
                        { key: 'url.path', value: { stringValue: '/fragment' } },
                        { key: 'http.response.status_code', value: { intValue: 200 } },
                        { key: 'http.response.body', value: { stringValue: '{"dep":"two"}' } },
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

    await session.loadCase(caseDocument);
    const hits = session.findAllInCase({ direction: 'outbound', path: '/fragment' });
    expect(hits).toHaveLength(2);
    expect(hits.map((hit) => hit.response.body)).toEqual(['{"dep":"one"}', '{"dep":"two"}']);

    await session.setPolicy({ externalHttp: 'strict' });
    await session.setAuthFixtures({ tokens: ['t1'] });

    expect(calls).toHaveLength(4);
    expect(calls[1].input).toBe('http://runtime.test/v1/sessions/sess_456/load-case');
    expect(JSON.parse(calls[1].init?.body ?? '{}')).toEqual(caseDocument);
    expect(calls[2].input).toBe('http://runtime.test/v1/sessions/sess_456/policy');
    expect(JSON.parse(calls[2].init?.body ?? '{}')).toEqual({ externalHttp: 'strict' });
    expect(calls[3].input).toBe('http://runtime.test/v1/sessions/sess_456/fixtures/auth');
    expect(JSON.parse(calls[3].init?.body ?? '{}')).toEqual({ tokens: ['t1'] });
  });

  it('surfaces typed errors for runtime unreachable, unknown session, case-load failure, and case lookup ambiguity', async () => {
    const unreachable = new Softprobe({
      baseUrl: 'http://runtime.test',
      fetchImpl: async () => {
        throw new Error('connect ECONNREFUSED');
      },
    });
    await expect(unreachable.startSession({ mode: 'replay' })).rejects.toBeInstanceOf(SoftprobeRuntimeUnreachableError);

    const missingSession = new Softprobe({
      baseUrl: 'http://runtime.test',
      fetchImpl: async (input) => {
        const pathname = new URL(input).pathname;
        if (pathname === '/v1/sessions') {
          return jsonResponse({ sessionId: 'sess_missing', sessionRevision: 0 });
        }
        return {
          ok: false,
          status: 404,
          text: async () => JSON.stringify({ error: { code: 'unknown_session', message: 'unknown session' } }),
        };
      },
    });
    const missing = await missingSession.startSession({ mode: 'replay' });
    await expect(missing.close()).rejects.toBeInstanceOf(SoftprobeUnknownSessionError);

    const tmpDir = await mkdtemp(path.join(os.tmpdir(), 'softprobe-sdk-errors-'));
    const invalidCasePath = path.join(tmpDir, 'invalid.case.json');
    await writeFile(invalidCasePath, '{"version":', 'utf8');

    const loader = new Softprobe({
      baseUrl: 'http://runtime.test',
      fetchImpl: async (input) => {
        const pathname = new URL(input).pathname;
        if (pathname === '/v1/sessions') {
          return jsonResponse({ sessionId: 'sess_case', sessionRevision: 0 });
        }
        return jsonResponse({ sessionId: 'sess_case', sessionRevision: 1 });
      },
    });
    const loadingSession = await loader.startSession({ mode: 'replay' });
    await expect(loadingSession.loadCaseFromFile(invalidCasePath)).rejects.toBeInstanceOf(SoftprobeCaseLoadError);

    const ambiguousSession = await loader.startSession({ mode: 'replay' });
    await ambiguousSession.loadCase({
      version: '1.0.0',
      caseId: 'ambiguous',
      traces: [
        {
          resourceSpans: [
            {
              scopeSpans: [
                {
                  spans: [
                    {
                      spanId: 'span-a',
                      attributes: [
                        { key: 'sp.span.type', value: { stringValue: 'inject' } },
                        { key: 'sp.traffic.direction', value: { stringValue: 'outbound' } },
                        { key: 'url.path', value: { stringValue: '/fragment' } },
                        { key: 'http.response.status_code', value: { intValue: 200 } },
                      ],
                    },
                    {
                      spanId: 'span-b',
                      attributes: [
                        { key: 'sp.span.type', value: { stringValue: 'extract' } },
                        { key: 'sp.traffic.direction', value: { stringValue: 'outbound' } },
                        { key: 'url.path', value: { stringValue: '/fragment' } },
                        { key: 'http.response.status_code', value: { intValue: 200 } },
                      ],
                    },
                  ],
                },
              ],
            },
          ],
        },
      ],
    });
    expect(() => ambiguousSession.findInCase({ direction: 'outbound', path: '/fragment' })).toThrow(
      SoftprobeCaseLookupAmbiguityError
    );

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
