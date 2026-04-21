/**
 * Example MockResponseHook used by `suites/fragment.suite.yaml`.
 *
 * When the captured case's `/fragment` response body is `{"dep":"ok"}`,
 * this hook rewrites `dep` to a value that is *impossible* for the live
 * upstream to produce. The harness test then sends `GET /hello` through
 * the proxy ingress and asserts the SUT's response body carries the
 * rewritten value — proving the hook actually executed and the mutated
 * response reached the mesh as a mock.
 */
import type { MockResponseHook } from '@softprobe/softprobe-js/hooks';

export const rewriteDep: MockResponseHook = ({ capturedResponse, env }) => {
  let body: Record<string, unknown>;
  try {
    body = JSON.parse(capturedResponse.body) as Record<string, unknown>;
  } catch {
    return {};
  }
  body.dep = env.FRAGMENT_DEP_VALUE ?? 'mutated-by-hook';
  return { body: JSON.stringify(body) };
};
