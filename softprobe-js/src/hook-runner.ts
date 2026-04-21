/**
 * Runtime execution helpers for Softprobe hooks.
 *
 * `src/hooks.ts` ships only the *types* that users import when authoring
 * hooks. This file is the missing half: the actual `apply…` /  `run…`
 * functions that invoke a hook, merge its partial return value back into a
 * captured value, and surface authoring errors with helpful context.
 *
 * Both the Jest `runSuite` adapter (src/suite.ts) and the future CLI
 * hook-runtime sidecar call through these helpers, so there is exactly one
 * implementation of "what does 'apply a hook' mean" across the SDK.
 *
 * Design invariants:
 *
 *  - Hooks are **pure functions** (see docs-site/guides/write-a-hook.md
 *    "Hook discipline"). The runner does not try to catch side effects;
 *    tests should guard this at authoring time.
 *  - A returned `{}` means "leave the input alone" — every hook return
 *    value is merged *over* the original, so forgetting to return a key
 *    keeps the captured value.
 *  - A hook that throws fails the case loudly. The runner wraps the
 *    exception in a `HookExecutionError` so the caller sees the hook name
 *    and kind in the stack trace, which is a huge time-saver when dozens
 *    of hooks run in one suite.
 */

import type {
  BodyAssertHook,
  BodyAssertHookContext,
  CaseDocument,
  HeadersAssertHook,
  HeadersAssertHookContext,
  HookRequest,
  HookResponse,
  Issue,
  MockResponseHook,
  MockResponseHookContext,
  RequestHook,
  RequestHookContext,
  AnyHook,
} from './hooks';
import { SoftprobeError } from './errors';

/**
 * Thrown when a hook's body raises. The original error is attached as
 * `cause` so `console.error(err)` in Node 16+ prints both frames.
 */
export class HookExecutionError extends SoftprobeError {
  constructor(
    public readonly hookName: string,
    public readonly hookKind: 'request' | 'mock-response' | 'assert-body' | 'assert-headers',
    public readonly cause: unknown
  ) {
    const detail = cause instanceof Error ? cause.message : String(cause);
    super(`hook ${hookName} (${hookKind}) threw: ${detail}`);
    this.name = 'HookExecutionError';
  }
}

/**
 * Base context shared by every hook kind. Exposed so adapter authors can
 * build a context once per case and reuse it across multiple hook calls.
 */
export interface BaseHookContext {
  case: CaseDocument;
  env: Record<string, string>;
  /** Filesystem path of the `.case.json`; optional, empty string for in-memory cases. */
  caseFile?: string;
}

/**
 * Apply a `RequestHook` to a captured / synthetic inbound request. The
 * hook's partial return value is shallow-merged over the input so any key
 * the hook does not return keeps its captured value.
 */
export function applyRequestHook(
  hook: RequestHook,
  hookName: string,
  request: HookRequest,
  ctx: BaseHookContext
): HookRequest {
  const full: RequestHookContext = {
    request,
    case: ctx.case,
    env: ctx.env,
    caseFile: ctx.caseFile ?? '',
  };
  let patch: ReturnType<RequestHook>;
  try {
    patch = hook(full);
  } catch (err) {
    throw new HookExecutionError(hookName, 'request', err);
  }
  return {
    method: patch.method ?? request.method,
    path: patch.path ?? request.path,
    headers: patch.headers ?? request.headers,
    body: patch.body ?? request.body,
  };
}

/**
 * Apply a `MockResponseHook` to a captured outbound response before it
 * becomes a mock rule. Same merge semantics as `applyRequestHook`.
 *
 * `mockName` flows through to the hook's `ctx.mockName` so authors can
 * switch behavior per mock when one file exports a hook used by many.
 */
export function applyMockResponseHook(
  hook: MockResponseHook,
  hookName: string,
  capturedResponse: HookResponse,
  capturedSpan: unknown,
  mockName: string,
  ctx: BaseHookContext
): HookResponse {
  const full: MockResponseHookContext = {
    capturedResponse,
    capturedSpan,
    mockName,
    case: ctx.case,
    env: ctx.env,
    caseFile: ctx.caseFile ?? '',
  };
  let patch: ReturnType<MockResponseHook>;
  try {
    patch = hook(full);
  } catch (err) {
    throw new HookExecutionError(hookName, 'mock-response', err);
  }
  return {
    status: patch.status ?? capturedResponse.status,
    headers: patch.headers ?? capturedResponse.headers,
    body: patch.body ?? capturedResponse.body,
  };
}

/**
 * Run a `BodyAssertHook`. Returns the raw `Issue[]` so the caller can
 * decide how to surface failures (throw for Jest, collect for reports,
 * etc.). An empty array means "no issues".
 */
export function runBodyAssert(
  hook: BodyAssertHook,
  hookName: string,
  actual: unknown,
  captured: unknown,
  ctx: BaseHookContext
): Issue[] {
  const full: BodyAssertHookContext = {
    actual,
    captured,
    case: ctx.case,
    env: ctx.env,
  };
  try {
    return hook(full) ?? [];
  } catch (err) {
    throw new HookExecutionError(hookName, 'assert-body', err);
  }
}

/** Run a `HeadersAssertHook`. Same contract as `runBodyAssert`. */
export function runHeadersAssert(
  hook: HeadersAssertHook,
  hookName: string,
  actual: Record<string, string>,
  captured: Record<string, string>,
  ctx: BaseHookContext
): Issue[] {
  const full: HeadersAssertHookContext = {
    actual,
    captured,
    case: ctx.case,
    env: ctx.env,
  };
  try {
    return hook(full) ?? [];
  } catch (err) {
    throw new HookExecutionError(hookName, 'assert-headers', err);
  }
}

/**
 * Convert `Issue[]` to a human-readable, single-line summary suitable for
 * inclusion in a Jest `expect(...).toEqual([])` failure message. Returning
 * an empty string when there are no issues keeps call sites terse.
 */
export function formatIssues(issues: Issue[]): string {
  if (issues.length === 0) return '';
  return issues
    .map((i) => {
      const parts: string[] = [i.path];
      if (i.expected !== undefined) parts.push(`expected=${JSON.stringify(i.expected)}`);
      if (i.actual !== undefined) parts.push(`actual=${JSON.stringify(i.actual)}`);
      if (i.reason) parts.push(`reason=${i.reason}`);
      return parts.join(' ');
    })
    .join('; ');
}

/**
 * Resolve a YAML hook reference like `"stripe.unmaskCard"` against the
 * registry passed to `runSuite({ hooks })`. The adapter parses
 * `hook: stripe.unmaskCard` in suite.yaml into a dotted string; we look
 * up the full string *and* the tail segment so authors can register hooks
 * either way:
 *
 *   { 'stripe.unmaskCard': fn }   // preferred — namespaced
 *   { unmaskCard:          fn }   // also accepted
 */
export function resolveHook(
  registry: Record<string, AnyHook> | undefined,
  ref: string
): AnyHook | undefined {
  if (!registry || !ref) return undefined;
  if (ref in registry) return registry[ref];
  const tail = ref.split('.').pop();
  if (tail && tail in registry) return registry[tail];
  return undefined;
}
