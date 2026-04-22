/**
 * End-to-end verification that hooks referenced from a `suite.yaml`
 * flow all the way through to a real `softprobe-runtime` AND the real
 * Envoy + Softprobe WASM proxy running inside `e2e/docker-compose.yaml`.
 *
 * The test drives traffic through the **ingress proxy on :8082** (not the
 * app directly). That is the realistic path: the SDK issues a session
 * id, hands it to the caller via the `x-softprobe-session-id` header,
 * and the proxy translates it into W3C `tracestate` so the egress WASM
 * on the app's outbound hop can key `/v1/inject` lookups by the same
 * SDK-issued id.
 *
 * What we prove here (real stack, no mocks):
 *
 *   1. `runSuite()` loads `suite.yaml`, resolves the `hook:` reference
 *      against `options.hooks`, and applies the `MockResponseHook` to
 *      the captured `/fragment` response.
 *   2. The hook-transformed mock rule is registered on the runtime —
 *      asserted by reading `GET /v1/sessions/{id}/state`.
 *   3. When we hit the ingress proxy with the SDK session id on the
 *      `x-softprobe-session-id` header, the egress WASM matches the
 *      session on `/v1/inject` (the proxy must preserve the id's
 *      format — see `softprobe-proxy/src/otel.rs` session-id tests).
 *      The SUT therefore observes the **hook-mutated** dep, not the
 *      live upstream's `{"dep":"ok"}`. That closes the end-to-end loop.
 *   4. A `BodyAssertHook` (`helloShape`) runs on the live SUT response —
 *      exercising the full `onCase` handle that the docs describe.
 */
import path from 'path';

import { runSuite } from '@softprobe/softprobe-js/suite';

import { rewriteDep } from './hooks/rewrite-dep';
import { helloShape } from './hooks/assert-hello';

const runtimeUrl = process.env.SOFTPROBE_RUNTIME_URL ?? 'http://127.0.0.1:8080';
// Route through the ingress proxy so the WASM filter injects the SDK
// session id into tracestate. Tests that hit the app directly (:8081)
// bypass the proxy and defeat the point.
const ingressUrl = process.env.INGRESS_URL ?? 'http://127.0.0.1:8082';
const apiToken = process.env.SOFTPROBE_API_KEY ?? undefined;
const authHeaders = apiToken ? { Authorization: `Bearer ${apiToken}` } : undefined;

process.env.FRAGMENT_DEP_VALUE = 'mutated-by-hook';

async function checkReachable(url: string): Promise<boolean> {
  try {
    const r = await fetch(url, { signal: AbortSignal.timeout(2000) });
    return r.ok;
  } catch {
    return false;
  }
}

runSuite(path.join(__dirname, 'suites/fragment.suite.yaml'), {
  baseUrl: runtimeUrl,
  apiToken,
  // Skip when the runtime or the ingress proxy (Envoy, compose-only) aren't up.
  skipIf: async () =>
    !(await checkReachable(`${runtimeUrl}/health`)) ||
    !(await checkReachable(`${ingressUrl}/health`)),
  hooks: {
    rewriteDep,
    helloShape,
  },
  onCase: async ({ session, assertBody }) => {
    // (1) Runtime state: prove the hook-transformed body landed on the
    // runtime under the exact session id the SDK issued. The SDK uses a
    // `sess_<base64url>` format today, but the proxy must NOT rely on
    // that prefix — see `softprobe-proxy/src/otel.rs` session-id tests.
    expect(session.id).not.toMatch(/^sp-session-/);
    const stateRes = await fetch(`${runtimeUrl}/v1/sessions/${session.id}/state`, {
      headers: authHeaders,
    });
    expect(stateRes.status).toBe(200);
    const state = (await stateRes.json()) as {
      rules: {
        rules: Array<{
          when: Record<string, string>;
          then: { action: string; response: { status: number; body: string } };
        }>;
      };
    };
    const mockRule = state.rules.rules.find((r) => r.then?.action === 'mock');
    expect(mockRule).toBeDefined();
    expect(mockRule!.when.path).toBe('/fragment');
    expect(mockRule!.when.direction).toBe('outbound');
    const mockedBody = JSON.parse(mockRule!.then.response.body);
    expect(mockedBody).toEqual({ dep: 'mutated-by-hook' });

    // (2) Drive /hello through the ingress proxy → app → egress proxy →
    // /v1/inject (runtime). Before the proxy fix the egress would
    // generate a synthetic `sp-session-...` id and `/v1/inject` would
    // always miss, so the live upstream answered with `{"dep":"ok"}`.
    // After the fix the SDK-issued session id survives verbatim and
    // the hook-mutated body reaches the SUT.
    const response = await fetch(`${ingressUrl}/hello`, {
      headers: { 'x-softprobe-session-id': session.id },
    });
    expect(response.status).toBe(200);
    const body = (await response.json()) as { message: string; dep: string };
    expect(body.message).toBe('hello');
    expect(body.dep).toBe('mutated-by-hook');

    // (3) BodyAssertHook over the live SUT response — hook runs, returns
    // `Issue[]`, the adapter throws if non-empty.
    assertBody('helloShape', body, { direction: 'inbound', path: '/hello' });
  },
});
