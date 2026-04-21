/**
 * Example MockResponseHooks.
 *
 * A MockResponseHook transforms a captured outbound response *once* — at
 * the moment the proxy is about to serve it back as a mock. Use this to
 * reverse redactions that only the test needs, freeze unstable fields,
 * or swap in a test-mode identifier. The hook runs exactly once per
 * session, not on every call.
 */
import type { MockResponseHook } from '@softprobe/softprobe-js/hooks';

/**
 * Reverse the production redaction on Stripe card numbers so downstream
 * mocks see the valid test number. `$TEST_CARD` overrides the default.
 */
export const unmaskCard: MockResponseHook = ({ capturedResponse, env }) => {
  let body: unknown;
  try {
    body = JSON.parse(capturedResponse.body);
  } catch {
    return {};
  }

  if (!body || typeof body !== 'object') return {};
  const src = (body as { source?: Record<string, unknown> }).source;
  if (!src || typeof src !== 'object') return {};
  const card = (src as { card?: Record<string, unknown> }).card;
  if (!card || typeof card !== 'object') return {};

  card.number = env.TEST_CARD ?? '4111111111111111';
  return { body: JSON.stringify(body) };
};

/**
 * Freeze an auto-generated Stripe charge id so golden-file assertions do
 * not drift across reruns.
 */
export const freezeChargeId: MockResponseHook = ({ capturedResponse }) => {
  let body: unknown;
  try {
    body = JSON.parse(capturedResponse.body);
  } catch {
    return {};
  }
  if (!body || typeof body !== 'object') return {};
  (body as { id?: string }).id = 'ch_frozen_for_test';
  return { body: JSON.stringify(body) };
};
