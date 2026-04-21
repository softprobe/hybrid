/**
 * Example BodyAssertHooks.
 *
 * BodyAssertHooks let you replace a strict field-by-field golden compare
 * with domain-level invariants. Return an empty array for "no issues",
 * or one `Issue` per violation. The Softprobe CLI and the Jest suite
 * adapter both fail the case if any hook returns non-empty `Issue[]`.
 */
import type { BodyAssertHook, Issue } from '@softprobe/softprobe-js/hooks';

/**
 * Assert that `actual.total` equals the sum of `actual.items[].price`.
 * Works against both live responses and the captured one.
 */
export const totalsMatchItems: BodyAssertHook = ({ actual }) => {
  const issues: Issue[] = [];
  if (!actual || typeof actual !== 'object') {
    return [
      { path: '$', reason: 'expected object, got ' + typeof actual, actual: actual ?? null },
    ];
  }

  const obj = actual as { total?: number; items?: Array<{ price?: number }> };
  if (!Array.isArray(obj.items)) {
    issues.push({ path: '$.items', reason: 'missing or not an array' });
    return issues;
  }

  const sum = obj.items.reduce((acc, it) => acc + (typeof it.price === 'number' ? it.price : 0), 0);
  if (typeof obj.total !== 'number') {
    issues.push({ path: '$.total', reason: 'missing or not a number', actual: obj.total });
  } else if (Math.abs(obj.total - sum) > 0.001) {
    issues.push({ path: '$.total', expected: sum, actual: obj.total, reason: 'items sum mismatch' });
  }

  return issues;
};

/**
 * Reject any response that embeds a raw credit-card number. A safety net
 * for suites that deliberately skip the global scrub to test a card flow.
 */
export const noRawPAN: BodyAssertHook = ({ actual }) => {
  const serialized = JSON.stringify(actual ?? {});
  const matches = serialized.match(/\b\d{13,19}\b/g) ?? [];
  return matches
    .filter((n) => luhnValid(n))
    .map<Issue>((n) => ({ path: '$', reason: 'raw PAN detected', actual: n }));
};

function luhnValid(digits: string): boolean {
  let sum = 0;
  let shouldDouble = false;
  for (let i = digits.length - 1; i >= 0; i--) {
    let d = parseInt(digits.charAt(i), 10);
    if (shouldDouble) {
      d *= 2;
      if (d > 9) d -= 9;
    }
    sum += d;
    shouldDouble = !shouldDouble;
  }
  return sum % 10 === 0 && digits.length >= 13;
}
