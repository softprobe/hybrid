/**
 * `@softprobe/softprobe-js/hooks` — public type surface for suite hooks.
 *
 * Hooks are pure functions that mutate requests / responses or express
 * custom assertions during a suite run. The runtime does not execute them
 * directly; the Node-side suite runner (PD3.1c) does. These types are
 * imported by hook authors so their modules compile against the same
 * contract that `softprobe suite run` enforces.
 *
 * Reference: docs-site/guides/write-a-hook.md.
 */

/**
 * CaseDocument is an opaque, read-only structural type representing the
 * parsed `.case.json`. We intentionally leave it as `unknown` so hook
 * authors never rely on private fields — the documented way to traverse a
 * case from a hook is via spans returned from the SDK, not by reaching into
 * `ctx.case` internals.
 */
export type CaseDocument = Readonly<Record<string, unknown>>;

/** Minimal shape of a captured HTTP response recorded in a case span. */
export interface HookResponse {
  status: number;
  headers: Record<string, string>;
  body: string;
}

/** Request context passed to RequestHook. */
export interface HookRequest {
  method: string;
  path: string;
  headers: Record<string, string>;
  body: string;
}

export interface RequestHookContext {
  request: HookRequest;
  case: CaseDocument;
  env: Record<string, string>;
  caseFile: string;
}

/**
 * Request transformer. Returning `{}` leaves the request untouched; any
 * returned key replaces the original field.
 */
export type RequestHook = (ctx: RequestHookContext) => {
  method?: string;
  path?: string;
  headers?: Record<string, string>;
  body?: string;
};

export interface MockResponseHookContext {
  capturedResponse: HookResponse;
  capturedSpan: unknown;
  mockName: string;
  case: CaseDocument;
  env: Record<string, string>;
  caseFile: string;
}

/**
 * Mock response transformer. Return the fields you want to change; others
 * stay as captured.
 */
export type MockResponseHook = (ctx: MockResponseHookContext) => {
  status?: number;
  headers?: Record<string, string>;
  body?: string;
};

export interface Issue {
  path: string;
  expected?: unknown;
  actual?: unknown;
  reason?: string;
}

export interface BodyAssertHookContext {
  actual: unknown;
  captured: unknown;
  case: CaseDocument;
  env: Record<string, string>;
}

/**
 * Custom body assertion. Return an empty array to pass; any `Issue[]`
 * fails the case with the listed problems.
 */
export type BodyAssertHook = (ctx: BodyAssertHookContext) => Issue[];

export interface HeadersAssertHookContext {
  actual: Record<string, string>;
  captured: Record<string, string>;
  case: CaseDocument;
  env: Record<string, string>;
}

/** Custom headers assertion. Same return-value contract as BodyAssertHook. */
export type HeadersAssertHook = (ctx: HeadersAssertHookContext) => Issue[];

/**
 * Discriminated union over every hook shape. Useful for suite-runner
 * implementations that load a registry of hooks without knowing which kind
 * each export is.
 */
export type AnyHook = RequestHook | MockResponseHook | BodyAssertHook | HeadersAssertHook;
