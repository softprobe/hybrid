/**
 * Example RequestHooks.
 *
 * RequestHooks mutate a captured inbound request before it is replayed
 * against the system under test. Common uses: swap a masked credit card
 * for a valid test card, inject a fresh idempotency key, or re-sign a
 * request with a test HMAC secret.
 *
 * Return only the fields you want to change; omitted keys keep the
 * captured value.
 */
import type { RequestHook } from '@softprobe/softprobe-js/hooks';

/**
 * Replace a masked card number inside a JSON body with a valid test card
 * (controlled by `$TEST_CARD`). This is the classic use case for a
 * RequestHook: production captures mask PAN data, and your test-mode
 * payment processor needs something that passes Luhn.
 */
export const substituteCard: RequestHook = ({ request, env }) => {
  if (!request.body) return {};

  let parsed: unknown;
  try {
    parsed = JSON.parse(request.body);
  } catch {
    return {};
  }
  if (
    !parsed ||
    typeof parsed !== 'object' ||
    !('card' in parsed) ||
    typeof (parsed as { card: unknown }).card !== 'object'
  ) {
    return {};
  }

  const next = parsed as { card: Record<string, unknown> };
  next.card.number = env.TEST_CARD ?? '4111111111111111';
  next.card.exp_month = 12;
  next.card.exp_year = 2030;
  next.card.cvc = '123';

  return { body: JSON.stringify(next) };
};

/**
 * Append `x-test-run-id` to every inbound request so downstream logs can
 * disambiguate parallel replays.
 */
export const stampTestRunId: RequestHook = ({ request, env }) => {
  const runId = env.TEST_RUN_ID ?? 'local-dev';
  return {
    headers: { ...request.headers, 'x-test-run-id': runId },
  };
};
