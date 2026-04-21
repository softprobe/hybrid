/**
 * Example BodyAssertHook.
 *
 * Runs after the SUT returns its `/hello` JSON. Returns an empty
 * `Issue[]` when the response has the expected shape, otherwise returns
 * structured violations so the Jest runner can report which field
 * failed and why — see `docs-site/guides/write-a-hook.md`.
 */
import type { BodyAssertHook, Issue } from '@softprobe/softprobe-js/hooks';

export const helloShape: BodyAssertHook = ({ actual }) => {
  const issues: Issue[] = [];
  if (!actual || typeof actual !== 'object') {
    return [{ path: '$', reason: 'expected object', actual: actual ?? null }];
  }
  const obj = actual as Record<string, unknown>;
  if (obj.message !== 'hello') {
    issues.push({ path: '$.message', expected: 'hello', actual: obj.message });
  }
  if (typeof obj.dep !== 'string' || obj.dep.length === 0) {
    issues.push({ path: '$.dep', reason: 'missing or empty', actual: obj.dep });
  }
  return issues;
};
