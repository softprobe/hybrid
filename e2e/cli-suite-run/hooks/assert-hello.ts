/**
 * BodyAssertHook driven by `softprobe suite run` through the Node
 * sidecar. Returns an `Issue[]` (empty = pass). Identical signature to
 * the Jest adapter so the file is portable between drivers.
 */
export function helloShape({ actual, env }: any) {
  if (!actual || typeof actual !== 'object') {
    return [{ path: '$', reason: 'expected object', actual }];
  }
  const issues: Array<Record<string, unknown>> = [];
  if (actual.message !== 'hello') {
    issues.push({ path: '$.message', expected: 'hello', actual: actual.message });
  }
  // The partner `rewriteDep` MockResponseHook writes `FRAGMENT_DEP_VALUE`
  // (or "mutated-by-cli") into the registered mock. If we observe the
  // live upstream's "ok" here, the hook → runtime → proxy → SUT path
  // broke somewhere — proxy session-id leak, missing --hooks, etc.
  const expected = (env && env.FRAGMENT_DEP_VALUE) || 'mutated-by-cli';
  if (actual.dep !== expected) {
    issues.push({ path: '$.dep', expected, actual: actual.dep });
  }
  return issues;
}

/**
 * Paired with the `fragment-down` case in `suites/fragment.suite.yaml`.
 * That case uses `overrides:` to swap in a `source: inline` mock that
 * returns 503 + `{"dep":"unavailable"}` for the outbound `/fragment`
 * hop, so the SUT's `/hello` surfaces `dep="unavailable"` without any
 * MockResponseHook running. This assert proves the inline-mock path
 * round-trips through the runtime + proxy the same way the hook-driven
 * path does.
 */
export function helloUnavailable({ actual }: any) {
  if (!actual || typeof actual !== 'object') {
    return [{ path: '$', reason: 'expected object', actual }];
  }
  const issues: Array<Record<string, unknown>> = [];
  if (actual.message !== 'hello') {
    issues.push({ path: '$.message', expected: 'hello', actual: actual.message });
  }
  if (actual.dep !== 'unavailable') {
    issues.push({ path: '$.dep', expected: 'unavailable', actual: actual.dep });
  }
  return issues;
}
