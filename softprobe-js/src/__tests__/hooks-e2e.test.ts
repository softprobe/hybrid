/**
 * End-to-end verification that every hook kind actually runs.
 *
 * For each kind we:
 *  1. Load the example hook from `examples/hooks/hooks/<kind>.ts`.
 *  2. Feed it a realistic context.
 *  3. Assert the observable effect (payload change, issue list, etc.).
 *
 * The bottom of the file drives `runSuite()` against a stubbed runtime
 * and verifies that the MockResponseHook transformation appears on the
 * wire in the `mockOutbound` request — proving the hook actually runs
 * *through the adapter*, not just in isolation.
 */
import path from 'path';
import { readFileSync } from 'fs';

import {
  applyRequestHook,
  applyMockResponseHook,
  runBodyAssert,
  runHeadersAssert,
  HookExecutionError,
} from '../hook-runner';
import { runSuite } from '../suite';
import * as requestHooks from '../../examples/hooks/hooks/request';
import * as mockHooks from '../../examples/hooks/hooks/mock-response';
import * as assertBodyHooks from '../../examples/hooks/hooks/assert-body';
import * as assertHeadersHooks from '../../examples/hooks/hooks/assert-headers';
import type { RequestHook } from '../hooks';

const EXAMPLES = path.join(__dirname, '..', '..', 'examples', 'hooks');
const CASE_PATH = path.join(EXAMPLES, 'cases', 'checkout-happy.case.json');
const SUITE_PATH = path.join(EXAMPLES, 'suites', 'checkout.suite.yaml');

const loadCase = () => JSON.parse(readFileSync(CASE_PATH, 'utf8'));

describe('RequestHook (examples/hooks/hooks/request.ts)', () => {
  it('substituteCard swaps the masked PAN for a test card', () => {
    const caseDoc = loadCase();
    const out = applyRequestHook(
      requestHooks.substituteCard,
      'substituteCard',
      {
        method: 'POST',
        path: '/checkout',
        headers: { 'content-type': 'application/json' },
        body: '{"card":{"number":"**** **** **** 4242"}}',
      },
      { case: caseDoc, env: { TEST_CARD: '4000056655665556' } }
    );
    expect(JSON.parse(out.body!)).toEqual({
      card: { number: '4000056655665556', exp_month: 12, exp_year: 2030, cvc: '123' },
    });
    expect(out.method).toBe('POST');
    expect(out.path).toBe('/checkout');
  });

  it('omitted keys preserve the captured value', () => {
    const caseDoc = loadCase();
    const out = applyRequestHook(
      requestHooks.stampTestRunId,
      'stampTestRunId',
      { method: 'GET', path: '/ping', headers: { accept: 'text/plain' }, body: '' },
      { case: caseDoc, env: { TEST_RUN_ID: 'run_xyz' } }
    );
    expect(out.method).toBe('GET');
    expect(out.headers).toEqual({ accept: 'text/plain', 'x-test-run-id': 'run_xyz' });
  });

  it('wraps hook exceptions in HookExecutionError with hook name + kind', () => {
    const boom: RequestHook = () => {
      throw new Error('boom');
    };
    expect(() =>
      applyRequestHook(
        boom as never,
        'boom',
        { method: 'GET', path: '/x', headers: {}, body: '' },
        { case: {}, env: {} }
      )
    ).toThrow(HookExecutionError);
  });
});

describe('MockResponseHook (examples/hooks/hooks/mock-response.ts)', () => {
  it('unmaskCard rewrites the captured stripe token body', () => {
    const caseDoc = loadCase();
    const captured = {
      status: 200,
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        id: 'tok_auto_abc123',
        source: { card: { number: '**** **** **** 4242', brand: 'visa' } },
      }),
    };
    const out = applyMockResponseHook(
      mockHooks.unmaskCard,
      'unmaskCard',
      captured,
      null,
      'stripe-token',
      { case: caseDoc, env: { TEST_CARD: '4242424242424242' } }
    );
    expect(out.status).toBe(200);
    const body = JSON.parse(out.body!);
    expect(body.source.card.number).toBe('4242424242424242');
    expect(body.source.card.brand).toBe('visa');
  });

  it('freezeChargeId stabilizes non-deterministic ids', () => {
    const out = applyMockResponseHook(
      mockHooks.freezeChargeId,
      'freezeChargeId',
      { status: 200, headers: {}, body: '{"id":"ch_zxy_rand","amount":100}' },
      null,
      'charge',
      { case: {}, env: {} }
    );
    expect(JSON.parse(out.body!)).toEqual({ id: 'ch_frozen_for_test', amount: 100 });
  });
});

describe('BodyAssertHook (examples/hooks/hooks/assert-body.ts)', () => {
  it('totalsMatchItems returns no issues on a consistent body', () => {
    const issues = runBodyAssert(
      assertBodyHooks.totalsMatchItems,
      'totalsMatchItems',
      { total: 15.5, items: [{ price: 10 }, { price: 5.5 }] },
      null,
      { case: {}, env: {} }
    );
    expect(issues).toEqual([]);
  });

  it('totalsMatchItems reports a precise Issue when total drifts', () => {
    const issues = runBodyAssert(
      assertBodyHooks.totalsMatchItems,
      'totalsMatchItems',
      { total: 99, items: [{ price: 10 }, { price: 5.5 }] },
      null,
      { case: {}, env: {} }
    );
    expect(issues).toEqual([
      { path: '$.total', expected: 15.5, actual: 99, reason: 'items sum mismatch' },
    ]);
  });

  it('noRawPAN flags a leaked card number', () => {
    const issues = runBodyAssert(
      assertBodyHooks.noRawPAN,
      'noRawPAN',
      { card: '4111111111111111' },
      null,
      { case: {}, env: {} }
    );
    expect(issues.length).toBe(1);
    expect(issues[0].reason).toMatch(/PAN/i);
  });
});

describe('HeadersAssertHook (examples/hooks/hooks/assert-headers.ts)', () => {
  it('hasRequestId passes when header present', () => {
    const issues = runHeadersAssert(
      assertHeadersHooks.hasRequestId,
      'hasRequestId',
      { 'x-request-id': 'req_abc' },
      {},
      { case: {}, env: {} }
    );
    expect(issues).toEqual([]);
  });

  it('hasRequestId fails when header missing', () => {
    const issues = runHeadersAssert(
      assertHeadersHooks.hasRequestId,
      'hasRequestId',
      {},
      {},
      { case: {}, env: {} }
    );
    expect(issues).toEqual([{ path: 'headers.x-request-id', reason: 'missing or empty' }]);
  });

  it('jsonContentTypeWhenCaptured reports drift from application/json to text/plain', () => {
    const issues = runHeadersAssert(
      assertHeadersHooks.jsonContentTypeWhenCaptured,
      'jsonContentTypeWhenCaptured',
      { 'content-type': 'text/plain' },
      { 'content-type': 'application/json; charset=utf-8' },
      { case: {}, env: {} }
    );
    expect(issues).toHaveLength(1);
    expect(issues[0].path).toBe('headers.content-type');
  });
});

/**
 * End-to-end: drive `runSuite()` against the example suite.yaml with a
 * stubbed runtime, and assert that the MockResponseHook actually runs —
 * i.e. the `mockOutbound` request body on the wire contains the unmasked
 * card number.
 *
 * `runSuite` MUST be called at module top level (it registers `describe`
 * / `it` synchronously), so we store captured calls in a module-scoped
 * array and assert against it inside the registered test body via
 * `onCase`.
 */
const wireCalls: Array<{ path: string; body?: string }> = [];
const fakeFetch = async (input: string, init?: { method?: string; body?: string }) => {
  const url = new URL(input);
  wireCalls.push({ path: url.pathname, body: init?.body });
  if (url.pathname === '/v1/sessions') {
    return jsonRes({ sessionId: 'sess_e2e', sessionRevision: 0 });
  }
  if (url.pathname.endsWith('/load-case')) {
    return jsonRes({ sessionId: 'sess_e2e', sessionRevision: 1 });
  }
  if (url.pathname.endsWith('/rules')) {
    return jsonRes({ sessionId: 'sess_e2e', sessionRevision: 2 });
  }
  if (url.pathname.endsWith('/close')) {
    return jsonRes({ sessionId: 'sess_e2e', closed: true });
  }
  throw new Error(`unexpected ${url.pathname}`);
};

process.env.TEST_CARD = '4242424242424242';

runSuite(SUITE_PATH, {
  baseUrl: 'http://runtime.test',
  fetchImpl: fakeFetch as never,
  hooks: {
    'mock-response.unmaskCard': mockHooks.unmaskCard,
  },
  onCase: async () => {
    // The adapter calls session.mockOutbound(...), which POSTs to
    // /v1/sessions/{id}/rules with the full rules array. Grab the most
    // recent rules call and verify the hook-transformed response.
    const ruleCalls = wireCalls.filter((c) => c.path.endsWith('/rules'));
    expect(ruleCalls.length).toBeGreaterThan(0);
    const last = ruleCalls[ruleCalls.length - 1];
    const payload = JSON.parse(last.body ?? '{}') as {
      rules: Array<{ when: Record<string, string>; then: { action: string; response: { status: number; body: string } } }>;
    };
    expect(payload.rules.length).toBeGreaterThan(0);
    const mockRule = payload.rules.find((r) => r.then?.action === 'mock');
    expect(mockRule).toBeDefined();
    const body = JSON.parse(mockRule!.then.response.body);
    expect(body.source.card.number).toBe('4242424242424242');
    expect(mockRule!.then.response.status).toBe(200);
  },
});

function jsonRes(payload: unknown) {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    headers: { get: (_n: string) => 'application/json' },
    text: async () => JSON.stringify(payload),
    json: async () => payload,
  };
}
