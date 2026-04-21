/**
 * Public error surface for `@softprobe/softprobe-js`.
 *
 * Everything in this file is documented in docs-site/reference/sdk-typescript.md
 * and imported directly by user code. The class hierarchy is intentionally
 * flat so `instanceof` checks in tests are obvious:
 *
 *   SoftprobeError (base, extends Error)
 *     ├── RuntimeError          (status, body, url)
 *     │     └── SoftprobeUnknownSessionError  (runtime emits 404 + code=unknown_session)
 *     ├── SoftprobeRuntimeUnreachableError    (transport-level failure)
 *     ├── CaseLookupError       (matches[])
 *     └── CaseLoadError         (path, cause)
 *
 * Historical names (`SoftprobeRuntimeError`, `SoftprobeCaseLoadError`,
 * `SoftprobeCaseLookupAmbiguityError`) are preserved as re-exports that
 * point at the same constructors so existing consumers keep compiling.
 * New code should prefer the documented short names.
 */

export class SoftprobeError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'SoftprobeError';
  }
}

/**
 * Raised when the runtime responded with a non-2xx status. `status` and
 * `body` come straight from the HTTP response; `url` is the target URL we
 * requested, which makes diagnostics actionable without the caller having to
 * remember the endpoint.
 */
export class RuntimeError extends SoftprobeError {
  constructor(
    public readonly status: number,
    public readonly body: string,
    public readonly url: string = ''
  ) {
    super(`softprobe runtime request failed: status ${status}: ${body.trim()}`);
    this.name = 'RuntimeError';
  }
}

/**
 * Transport-level failure (DNS, TCP, TLS, timeout). Kept as a sibling of
 * RuntimeError rather than a subclass because it has no `status` / `body`.
 */
export class SoftprobeRuntimeUnreachableError extends SoftprobeError {
  constructor(message: string) {
    super(message);
    this.name = 'SoftprobeRuntimeUnreachableError';
  }
}

/**
 * Specialisation of RuntimeError emitted when the runtime rejected the
 * request because the session id is unknown (typically: you closed it,
 * restarted the runtime, or typo'd the id).
 */
export class SoftprobeUnknownSessionError extends RuntimeError {
  constructor(status: number, body: string, url: string = '') {
    super(status, body, url);
    this.name = 'SoftprobeUnknownSessionError';
  }
}

/**
 * `findInCase` saw zero or multiple matching spans. `matches` is always
 * populated so the caller can inspect candidate span ids, the raw attributes,
 * or fall through to `findAllInCase`-style logic.
 */
export class CaseLookupError extends SoftprobeError {
  constructor(
    message: string,
    public readonly matches: readonly unknown[]
  ) {
    super(message);
    this.name = 'CaseLookupError';
  }
}

/**
 * `loadCaseFromFile` failed — usually because the file is missing, malformed
 * JSON, or rejected by the runtime. `path` is the fs path we tried to read.
 */
export class CaseLoadError extends SoftprobeError {
  constructor(
    message: string,
    public readonly path: string = '',
    public readonly cause?: unknown
  ) {
    super(message);
    this.name = 'CaseLoadError';
  }
}

/*
 * ---- Legacy aliases ----
 *
 * These identifiers shipped in earlier releases and remain exported so the
 * 2.x SDK doesn't break semver for downstream users. New code should prefer
 * the short names above.
 */
export const SoftprobeRuntimeError = RuntimeError;
export type SoftprobeRuntimeError = RuntimeError;

export const SoftprobeCaseLoadError = CaseLoadError;
export type SoftprobeCaseLoadError = CaseLoadError;

export const SoftprobeCaseLookupAmbiguityError = CaseLookupError;
export type SoftprobeCaseLookupAmbiguityError = CaseLookupError;
