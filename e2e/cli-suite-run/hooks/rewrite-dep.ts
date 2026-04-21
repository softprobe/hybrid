/**
 * MockResponseHook used by `suites/fragment.suite.yaml` and driven
 * through the CLI's Node sidecar by `softprobe suite run --hooks …`.
 *
 * The sidecar invokes this function with
 * `{ capturedResponse, capturedSpan, mockName, ctx, env }` — identical
 * shape to the TypeScript SDK's Jest `runSuite()` adapter, so the same
 * file can be imported from either driver without changes. See
 * `docs-site/guides/run-a-suite-at-scale.md#hooks--when-declarative-isnt-enough`.
 */
export function rewriteDep({ capturedResponse, env }: any) {
  let body: Record<string, unknown>;
  try {
    body = JSON.parse(capturedResponse.body);
  } catch {
    return {};
  }
  body.dep = (env && env.FRAGMENT_DEP_VALUE) || 'mutated-by-cli';
  return { body: JSON.stringify(body) };
}
