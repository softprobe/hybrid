/**
 * Example HeadersAssertHooks.
 *
 * Headers assertion hooks run after the SUT returns. They are the right
 * place to encode rules like "every endpoint must set Cache-Control" or
 * "responses must echo the correlation id we sent on the way in".
 */
import type { HeadersAssertHook, Issue } from '@softprobe/softprobe-js/hooks';

/** Every response must set a non-empty `x-request-id`. */
export const hasRequestId: HeadersAssertHook = ({ actual }) => {
  const rid = actual['x-request-id'] ?? actual['X-Request-Id'];
  if (rid && rid.length > 0) return [];
  return [{ path: 'headers.x-request-id', reason: 'missing or empty' }];
};

/**
 * Require `Content-Type: application/json; charset=utf-8` for any
 * response whose captured version was JSON. Lets you catch teams that
 * quietly start returning `text/plain`.
 */
export const jsonContentTypeWhenCaptured: HeadersAssertHook = ({ actual, captured }) => {
  const capturedCT = (captured['content-type'] ?? captured['Content-Type'] ?? '').toLowerCase();
  if (!capturedCT.includes('application/json')) return [];

  const actualCT = (actual['content-type'] ?? actual['Content-Type'] ?? '').toLowerCase();
  const issues: Issue[] = [];
  if (!actualCT.includes('application/json')) {
    issues.push({
      path: 'headers.content-type',
      expected: 'application/json',
      actual: actualCT || null,
      reason: 'captured was JSON; response is not',
    });
  }
  return issues;
};
