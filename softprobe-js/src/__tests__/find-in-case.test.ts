import { mkdtemp, rm, writeFile } from 'fs/promises';
import os from 'os';
import path from 'path';

import { Softprobe, SoftprobeSession } from '../softprobe';

describe('SoftprobeSession.findInCase', () => {
  let tmpDir: string;

  beforeAll(async () => {
    tmpDir = await mkdtemp(path.join(os.tmpdir(), 'softprobe-find-in-case-'));
  });

  afterAll(async () => {
    await rm(tmpDir, { recursive: true, force: true });
  });

  function makeSpan(overrides: {
    traceId: string;
    spanId: string;
    direction: string;
    method: string;
    urlPath: string;
    host?: string;
    status?: number;
    body?: string;
    headers?: Record<string, string>;
    spanType?: string;
  }) {
    const attrs: Array<{ key: string; value: Record<string, unknown> }> = [
      { key: 'sp.span.type', value: { stringValue: overrides.spanType ?? 'inject' } },
      { key: 'sp.traffic.direction', value: { stringValue: overrides.direction } },
      { key: 'url.path', value: { stringValue: overrides.urlPath } },
      { key: 'http.request.method', value: { stringValue: overrides.method } },
    ];
    if (overrides.host !== undefined) {
      attrs.push({ key: 'url.host', value: { stringValue: overrides.host } });
    }
    if (overrides.status !== undefined) {
      attrs.push({ key: 'http.response.status_code', value: { intValue: overrides.status } });
    }
    if (overrides.body !== undefined) {
      attrs.push({ key: 'http.response.body', value: { stringValue: overrides.body } });
    }
    for (const [name, value] of Object.entries(overrides.headers ?? {})) {
      attrs.push({ key: `http.response.header.${name}`, value: { stringValue: value } });
    }
    return {
      traceId: overrides.traceId,
      spanId: overrides.spanId,
      name: `HTTP ${overrides.method}`,
      attributes: attrs,
    };
  }

  function makeCase(spans: Array<ReturnType<typeof makeSpan>>) {
    return {
      version: '1.0.0',
      caseId: 'test',
      traces: [
        {
          resourceSpans: [
            {
              resource: {
                attributes: [{ key: 'service.name', value: { stringValue: 'api' } }],
              },
              scopeSpans: [{ spans }],
            },
          ],
        },
      ],
    };
  }

  async function makeSession(caseDocument: unknown): Promise<SoftprobeSession> {
    const softprobe = new Softprobe({
      baseUrl: 'http://runtime.test',
      fetchImpl: async (input) => {
        const pathname = new URL(input).pathname;
        if (pathname === '/v1/sessions') {
          return jsonResponse({ sessionId: 'sess_find', sessionRevision: 0 });
        }
        if (pathname.endsWith('/load-case')) {
          return jsonResponse({ sessionId: 'sess_find', sessionRevision: 1 });
        }
        throw new Error(`unexpected request during findInCase test: ${pathname}`);
      },
    });
    const session = await softprobe.startSession({ mode: 'replay' });
    const caseFile = path.join(tmpDir, `${Math.random().toString(36).slice(2)}.case.json`);
    await writeFile(caseFile, JSON.stringify(caseDocument));
    await session.loadCaseFromFile(caseFile);
    return session;
  }

  it('returns the captured response when exactly one span matches', async () => {
    const session = await makeSession(
      makeCase([
        makeSpan({
          traceId: 'trace1',
          spanId: 'span1',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/fragment',
          status: 200,
          body: '{"dep":"ok"}',
          headers: { 'content-type': 'application/json' },
        }),
        makeSpan({
          traceId: 'trace1',
          spanId: 'span2',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/other',
          status: 404,
        }),
      ])
    );

    const hit = session.findInCase({
      direction: 'outbound',
      method: 'GET',
      path: '/fragment',
    });

    expect(hit.response.status).toBe(200);
    expect(hit.response.body).toBe('{"dep":"ok"}');
    expect(hit.response.headers).toEqual({ 'content-type': 'application/json' });
    expect(hit.span).toBeDefined();
    expect((hit.span as { spanId?: string }).spanId).toBe('span1');
  });

  it('throws with an informative error when zero spans match', async () => {
    const session = await makeSession(
      makeCase([
        makeSpan({
          traceId: 'trace1',
          spanId: 'span1',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/fragment',
          status: 200,
          body: '',
        }),
      ])
    );

    expect(() =>
      session.findInCase({
        direction: 'outbound',
        method: 'POST',
        path: '/fragment',
      })
    ).toThrow(/findInCase/);
    expect(() =>
      session.findInCase({
        direction: 'outbound',
        method: 'POST',
        path: '/fragment',
      })
    ).toThrow(/POST/);
    expect(() =>
      session.findInCase({
        direction: 'outbound',
        method: 'POST',
        path: '/fragment',
      })
    ).toThrow(/\/fragment/);
  });

  it('throws listing candidate span ids when more than one span matches', async () => {
    const session = await makeSession(
      makeCase([
        makeSpan({
          traceId: 'trace1',
          spanId: 'span-a',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/fragment',
          status: 500,
          body: 'err1',
        }),
        makeSpan({
          traceId: 'trace1',
          spanId: 'span-b',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/fragment',
          status: 200,
          body: 'ok',
        }),
      ])
    );

    expect(() =>
      session.findInCase({ direction: 'outbound', method: 'GET', path: '/fragment' })
    ).toThrow(/2 spans/);
    expect(() =>
      session.findInCase({ direction: 'outbound', method: 'GET', path: '/fragment' })
    ).toThrow(/span-a/);
    expect(() =>
      session.findInCase({ direction: 'outbound', method: 'GET', path: '/fragment' })
    ).toThrow(/span-b/);
  });

  it('matches pathPrefix and host predicates', async () => {
    const session = await makeSession(
      makeCase([
        makeSpan({
          traceId: 't1',
          spanId: 's1',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/v1/payment_intents/pi_123',
          host: 'api.stripe.com',
          status: 200,
          body: '{}',
        }),
      ])
    );

    const hit = session.findInCase({
      direction: 'outbound',
      method: 'GET',
      pathPrefix: '/v1/payment_intents',
      host: 'api.stripe.com',
    });
    expect(hit.response.status).toBe(200);
    expect(hit.response.body).toBe('{}');
  });

  it('defaults body and headers when the span omits response attributes', async () => {
    const session = await makeSession(
      makeCase([
        makeSpan({
          traceId: 't1',
          spanId: 's1',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/health',
          status: 204,
        }),
      ])
    );

    const hit = session.findInCase({ direction: 'outbound', path: '/health' });
    expect(hit.response.status).toBe(204);
    expect(hit.response.body).toBe('');
    expect(hit.response.headers).toEqual({});
  });

  it('also matches extract spans (not only inject)', async () => {
    const session = await makeSession(
      makeCase([
        makeSpan({
          traceId: 't1',
          spanId: 's-extract',
          spanType: 'extract',
          direction: 'outbound',
          method: 'GET',
          urlPath: '/late',
          status: 200,
          body: 'late',
        }),
      ])
    );

    const hit = session.findInCase({ direction: 'outbound', path: '/late' });
    expect(hit.response.body).toBe('late');
  });

  it('throws when no case has been loaded', async () => {
    const softprobe = new Softprobe({
      baseUrl: 'http://runtime.test',
      fetchImpl: async (input) => {
        if (new URL(input).pathname === '/v1/sessions') {
          return jsonResponse({ sessionId: 'sess_x', sessionRevision: 0 });
        }
        throw new Error('unexpected');
      },
    });
    const session = await softprobe.startSession({ mode: 'replay' });
    expect(() => session.findInCase({ path: '/anything' })).toThrow(/loadCaseFromFile/);
  });
});

function jsonResponse(body: unknown) {
  return {
    ok: true,
    status: 200,
    text: async () => JSON.stringify(body),
  };
}
