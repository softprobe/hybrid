/**
 * `@softprobe/softprobe-js/suite` — Jest adapter for `suite.yaml`.
 *
 * The CLI (`softprobe suite run`) is the canonical suite executor; this
 * module lets Jest tests load the **same** YAML and register one
 * `describe` / `it` per case, sharing hook modules between both runners.
 *
 * See docs-site/reference/sdk-typescript.md#runSuite and
 * docs-site/guides/run-a-suite-at-scale.md.
 */
import { readFileSync } from 'fs';
import path from 'path';

import { parse as parseYAML } from 'yaml';

import { findSpans, responseFromSpan, type CaseSpanPredicate, type CapturedHit } from './core/case/find-span';
import {
  applyMockResponseHook,
  applyRequestHook,
  runBodyAssert,
  runHeadersAssert,
  resolveHook,
  formatIssues,
  type BaseHookContext,
} from './hook-runner';
import type {
  AnyHook,
  BodyAssertHook,
  HeadersAssertHook,
  HookRequest,
  HookResponse,
  Issue,
  MockResponseHook,
  RequestHook,
} from './hooks';
import { Softprobe, SoftprobeSession } from './softprobe';

export interface RunSuiteOptions {
  /**
   * Resolved hook functions keyed by the string you'd write in `suite.yaml`
   * after `hook:`. Pass `{ ...import * as hooks }` to register every export.
   */
  hooks?: Record<string, AnyHook>;
  /** Runtime base URL override; falls back to `SOFTPROBE_RUNTIME_URL`. */
  baseUrl?: string;
  /**
   * Custom `fetch` — primarily for tests that want to drive `runSuite`
   * without a live runtime. Signature matches the global `fetch`.
   */
  fetchImpl?: (input: string, init?: { method?: string; headers?: Record<string, string>; body?: string }) => Promise<{
    ok: boolean;
    status: number;
    statusText?: string;
    headers: { get(name: string): string | null };
    text(): Promise<string>;
    json(): Promise<unknown>;
  }>;
  /** Override `$APP_URL` — available to hooks via `ctx.env.APP_URL`. */
  appUrl?: string;
  /** Substring filter applied to `caseId` / `path`; only matching cases run. */
  filter?: string;
  /** Mark every `it` as `.concurrent`. Jest decides actual parallelism. */
  parallel?: number;
  /**
   * Test body executed inside each `it`. When supplied, the adapter passes
   * a `SuiteCaseHandle` so the author can exercise the SUT, run
   * RequestHooks on captured requests, and invoke assertion hooks on live
   * responses. When omitted the adapter only wires MockResponseHooks —
   * which is the most common setup.
   */
  onCase?: (ctx: SuiteCaseHandle) => Promise<void> | void;
}

export interface SuiteMock {
  name: string;
  match: CaseSpanPredicate;
  /** Fully-qualified hook reference, e.g. `stripe.unmaskCard`. */
  hook?: string;
}

export interface SuiteCaseRef {
  path: string;
  name?: string;
  overrides?: Record<string, unknown>;
}

export interface SuiteDocument {
  name: string;
  version?: number;
  cases: SuiteCaseRef[];
  mocks?: SuiteMock[];
  defaults?: Record<string, unknown>;
}

/**
 * Handle passed to `onCase`. Encapsulates everything the test body can do
 * once the case is loaded and mocks are wired: exercise the SUT, ask the
 * session for captured hits, invoke assertion hooks, etc.
 */
export interface SuiteCaseHandle {
  session: SoftprobeSession;
  caseDocument: unknown;
  casePath: string;
  env: Record<string, string>;
  /**
   * Apply a `RequestHook` by its registry reference. Useful when the test
   * body needs to transform a captured inbound request before replaying
   * it against the SUT.
   */
  applyRequest(ref: string, request: HookRequest): HookRequest;
  /** Run a `BodyAssertHook` and throw if it returns a non-empty issue list. */
  assertBody(ref: string, actual: unknown, capturedSpec: CaseSpanPredicate): void;
  /** Run a `HeadersAssertHook` and throw if it returns issues. */
  assertHeaders(ref: string, actual: Record<string, string>, capturedSpec: CaseSpanPredicate): void;
}

/**
 * Jest test registrar. MUST be called at module top level so `describe` /
 * `it` are hoisted by Jest's global registration phase — calling it from
 * inside a `beforeAll` is a common foot-gun and registers zero tests.
 */
export function runSuite(suitePath: string, options: RunSuiteOptions = {}): void {
  const suite = loadSuite(suitePath);
  const suiteDir = path.dirname(path.resolve(suitePath));
  const { filter, parallel, baseUrl, hooks = {}, onCase, fetchImpl } = options;

  const cases = filter
    ? suite.cases.filter((c) => c.path.includes(filter) || (c.name ?? '').includes(filter))
    : suite.cases;

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const g = globalThis as any;
  if (typeof g.describe !== 'function' || typeof g.it !== 'function') {
    throw new Error(
      'runSuite requires Jest globals (describe/it). Call it from a *.test.ts file.'
    );
  }

  g.describe(suite.name, () => {
    const itFn = parallel && parallel > 1 && typeof g.it.concurrent === 'function'
      ? g.it.concurrent
      : g.it;

    for (const caseRef of cases) {
      const label = caseRef.name ?? path.basename(caseRef.path);
      itFn(label, async () => {
        const softprobe = new Softprobe({
          baseUrl,
          ...(fetchImpl ? { fetchImpl } : {}),
        });
        const session = await softprobe.startSession({ mode: 'replay' });
        try {
          const resolvedCasePath = path.isAbsolute(caseRef.path)
            ? caseRef.path
            : path.join(suiteDir, caseRef.path);
          await session.loadCaseFromFile(resolvedCasePath);

          // We need the parsed case for hook contexts + span lookups. The
          // session kept a reference, but it is private; re-read from disk
          // so the document on the hook ctx matches what the CLI would
          // load.
          const caseDocument = JSON.parse(readFileSync(resolvedCasePath, 'utf8'));
          const env = { ...process.env, APP_URL: options.appUrl ?? process.env.APP_URL ?? '' };
          const ctx: BaseHookContext = {
            case: caseDocument as Readonly<Record<string, unknown>>,
            env: env as Record<string, string>,
            caseFile: resolvedCasePath,
          };

          // Wire every suite-level mock: find the captured hit, apply the
          // MockResponseHook if referenced, then post the resulting mock
          // rule to the runtime. This is the piece that makes hooks
          // *actually run* end-to-end.
          for (const mock of suite.mocks ?? []) {
            const hits = findSpans(caseDocument, mock.match);
            if (hits.length === 0) {
              throw new Error(
                `runSuite: mock '${mock.name}' matches zero spans in ${resolvedCasePath}. ` +
                  'Check match predicate (direction/host/path).'
              );
            }
            const hit: CapturedHit = {
              response: responseFromSpan(hits[0].span),
              span: hits[0].span,
            };
            let response: HookResponse = {
              status: hit.response.status,
              headers: hit.response.headers,
              body: hit.response.body,
            };
            if (mock.hook) {
              const hookFn = resolveHook(hooks, mock.hook);
              if (!hookFn) {
                throw new Error(
                  `runSuite: mock '${mock.name}' references hook '${mock.hook}' but it is not in options.hooks`
                );
              }
              response = applyMockResponseHook(
                hookFn as MockResponseHook,
                mock.hook,
                response,
                hit.span,
                mock.name,
                ctx
              );
            }
            await session.mockOutbound({
              ...mock.match,
              direction: mock.match.direction as 'inbound' | 'outbound' | undefined,
              response,
            });
          }

          if (onCase) {
            const handle: SuiteCaseHandle = {
              session,
              caseDocument,
              casePath: resolvedCasePath,
              env: env as Record<string, string>,
              applyRequest(ref, request) {
                const hookFn = resolveHook(hooks, ref);
                if (!hookFn) {
                  throw new Error(`runSuite.applyRequest: hook '${ref}' not registered`);
                }
                return applyRequestHook(hookFn as RequestHook, ref, request, ctx);
              },
              assertBody(ref, actual, capturedSpec) {
                const hookFn = resolveHook(hooks, ref);
                if (!hookFn) throw new Error(`runSuite.assertBody: hook '${ref}' not registered`);
                const captured = pickCapturedBody(caseDocument, capturedSpec);
                const issues = runBodyAssert(hookFn as BodyAssertHook, ref, actual, captured, ctx);
                if (issues.length > 0) throw new AssertionIssuesError(ref, issues);
              },
              assertHeaders(ref, actual, capturedSpec) {
                const hookFn = resolveHook(hooks, ref);
                if (!hookFn) throw new Error(`runSuite.assertHeaders: hook '${ref}' not registered`);
                const capturedHeaders = pickCapturedHeaders(caseDocument, capturedSpec);
                const issues = runHeadersAssert(
                  hookFn as HeadersAssertHook,
                  ref,
                  actual,
                  capturedHeaders,
                  ctx
                );
                if (issues.length > 0) throw new AssertionIssuesError(ref, issues);
              },
            };
            await onCase(handle);
          }
        } finally {
          await session.close();
        }
      });
    }
  });
}

/**
 * Parse a `suite.yaml` into a typed document. Exported so the CLI schema
 * validator and the Jest adapter share the exact same surface.
 */
export function loadSuite(suitePath: string): SuiteDocument {
  const raw = readFileSync(suitePath, 'utf8');
  const parsed = parseYAML(raw) as Record<string, unknown> | null;
  if (!parsed || typeof parsed !== 'object') {
    throw new Error(`suite.yaml ${suitePath}: expected top-level mapping`);
  }
  const name = typeof parsed.name === 'string' ? parsed.name : '';
  if (!name) {
    throw new Error(`suite.yaml ${suitePath}: missing required 'name'`);
  }
  const version = typeof parsed.version === 'number' ? parsed.version : 1;
  const cases = normalizeCases(parsed.cases);
  const mocks = normalizeMocks(parsed.mocks);
  return {
    name,
    version,
    cases,
    mocks,
    defaults: (parsed.defaults as Record<string, unknown>) ?? undefined,
  };
}

function normalizeCases(raw: unknown): SuiteCaseRef[] {
  if (!raw) return [];
  if (Array.isArray(raw)) {
    return raw.map((entry) => {
      if (typeof entry === 'string') return { path: entry };
      if (entry && typeof entry === 'object' && typeof (entry as { path?: unknown }).path === 'string') {
        const obj = entry as { path: string; name?: string; overrides?: Record<string, unknown> };
        return { path: obj.path, name: obj.name, overrides: obj.overrides };
      }
      throw new Error('suite.yaml: each case must be a string or {path,...} object');
    });
  }
  if (typeof raw === 'string') {
    // Glob form stored verbatim; the CLI expands it, the Jest adapter
    // does not.
    return [{ path: raw }];
  }
  throw new Error('suite.yaml: cases must be an array or glob string');
}

function normalizeMocks(raw: unknown): SuiteMock[] | undefined {
  if (!raw) return undefined;
  if (!Array.isArray(raw)) {
    throw new Error('suite.yaml: mocks must be an array');
  }
  return raw.map((entry, i) => {
    if (!entry || typeof entry !== 'object') {
      throw new Error(`suite.yaml: mocks[${i}] must be an object`);
    }
    const m = entry as { name?: unknown; match?: unknown; hook?: unknown };
    if (typeof m.name !== 'string' || !m.name) {
      throw new Error(`suite.yaml: mocks[${i}] missing required 'name'`);
    }
    if (!m.match || typeof m.match !== 'object') {
      throw new Error(`suite.yaml: mocks[${i}] missing required 'match' object`);
    }
    return {
      name: m.name,
      match: m.match as CaseSpanPredicate,
      hook: typeof m.hook === 'string' ? m.hook : undefined,
    };
  });
}

class AssertionIssuesError extends Error {
  constructor(hookRef: string, public readonly issues: Issue[]) {
    super(`hook ${hookRef} reported issues: ${formatIssues(issues)}`);
    this.name = 'AssertionIssuesError';
  }
}

function pickCapturedBody(caseDocument: unknown, spec: CaseSpanPredicate): unknown {
  const hits = findSpans(caseDocument, spec);
  if (hits.length === 0) return undefined;
  const body = responseFromSpan(hits[0].span).body;
  try {
    return JSON.parse(body);
  } catch {
    return body;
  }
}

function pickCapturedHeaders(caseDocument: unknown, spec: CaseSpanPredicate): Record<string, string> {
  const hits = findSpans(caseDocument, spec);
  if (hits.length === 0) return {};
  return responseFromSpan(hits[0].span).headers;
}
