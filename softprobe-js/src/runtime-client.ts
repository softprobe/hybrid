import { fetch as undiciFetch } from 'undici';

import {
  RuntimeError,
  SoftprobeError,
  SoftprobeRuntimeUnreachableError,
  SoftprobeUnknownSessionError,
} from './errors';

export {
  RuntimeError,
  SoftprobeError,
  SoftprobeRuntimeUnreachableError,
  SoftprobeUnknownSessionError,
} from './errors';

// Legacy re-export kept for consumers that imported SoftprobeRuntimeError
// directly from './runtime-client'; it is the same constructor as
// RuntimeError.
export { SoftprobeRuntimeError } from './errors';

export interface SessionCreateInput {
  mode: string;
}

export interface SessionCreateResponse {
  sessionId: string;
  sessionRevision: number;
}

export interface SessionCloseResponse {
  sessionId: string;
  closed: boolean;
}

export interface SoftprobeRuntimeClientOptions {
  baseUrl: string;
  fetchImpl?: FetchLike;
  /**
   * Bearer token sent as `Authorization: Bearer <token>` on every control-plane
   * and OTLP request. When omitted, falls back to the `SOFTPROBE_API_TOKEN`
   * environment variable. When both are empty / whitespace, no Authorization
   * header is sent — matching the runtime's "auth disabled by default" story
   * (see `softprobe-runtime/internal/controlapi.withOptionalBearerAuth`).
   */
  apiToken?: string;
}

interface ResponseLike {
  ok: boolean;
  status: number;
  text(): Promise<string>;
}

interface RequestInitLike {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
}

type FetchLike = (input: string, init?: RequestInitLike) => Promise<ResponseLike>;

interface SessionsClient {
  create(input: SessionCreateInput): Promise<SessionCreateResponse>;
  loadCase(sessionId: string, caseDocument: unknown): Promise<SessionCreateResponse>;
  setPolicy(sessionId: string, policyDocument: unknown): Promise<SessionCreateResponse>;
  setAuthFixtures(sessionId: string, fixturesDocument: unknown): Promise<SessionCreateResponse>;
  close(sessionId: string): Promise<SessionCloseResponse>;
}

/**
 * Thin HTTP client for the Softprobe control runtime.
 * This client only speaks the JSON control API and deliberately omits CLI verbs.
 */
export class SoftprobeRuntimeClient {
  readonly sessions: SessionsClient;

  private readonly fetchImpl: FetchLike;
  private readonly apiToken?: string;

  constructor(
    private readonly baseUrl: string,
    fetchImpl: FetchLike = undiciFetch as unknown as FetchLike,
    apiToken?: string
  ) {
    this.fetchImpl = fetchImpl;
    this.apiToken = apiToken;
    this.sessions = {
      create: (input) => this.createSession(input),
      loadCase: (sessionId, caseDocument) => this.loadCase(sessionId, caseDocument),
      setPolicy: (sessionId, policyDocument) => this.setPolicy(sessionId, policyDocument),
      setAuthFixtures: (sessionId, fixturesDocument) => this.setAuthFixtures(sessionId, fixturesDocument),
      close: (sessionId) => this.closeSession(sessionId),
    };
  }

  async createSession(input: SessionCreateInput): Promise<SessionCreateResponse> {
    return this.postJson<SessionCreateResponse>('/v1/sessions', input);
  }

  async loadCase(sessionId: string, caseDocument: unknown): Promise<SessionCreateResponse> {
    return this.postJson<SessionCreateResponse>(`/v1/sessions/${sessionId}/load-case`, caseDocument);
  }

  async closeSession(sessionId: string): Promise<SessionCloseResponse> {
    return this.postJson<SessionCloseResponse>(`/v1/sessions/${sessionId}/close`, {});
  }

  async setPolicy(sessionId: string, policyDocument: unknown): Promise<SessionCreateResponse> {
    return this.postJson<SessionCreateResponse>(`/v1/sessions/${sessionId}/policy`, policyDocument);
  }

  async setAuthFixtures(sessionId: string, fixturesDocument: unknown): Promise<SessionCreateResponse> {
    return this.postJson<SessionCreateResponse>(`/v1/sessions/${sessionId}/fixtures/auth`, fixturesDocument);
  }

  async updateRules(sessionId: string, rulesDocument: unknown): Promise<SessionCreateResponse> {
    return this.postJson<SessionCreateResponse>(`/v1/sessions/${sessionId}/rules`, rulesDocument);
  }

  private async postJson<T>(path: string, body: unknown): Promise<T> {
    const headers: Record<string, string> = {
      'content-type': 'application/json',
    };
    const token = resolveBearerToken(this.apiToken);
    if (token) {
      headers.authorization = `Bearer ${token}`;
    }

    const url = this.url(path);
    let response: ResponseLike;
    try {
      response = await this.fetchImpl(url, {
        method: 'POST',
        headers,
        body: JSON.stringify(body),
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      throw new SoftprobeRuntimeUnreachableError(`softprobe runtime is unreachable: ${message}`);
    }

    const responseText = await response.text();
    if (!response.ok) {
      throw classifyRuntimeError(response.status, responseText, url);
    }

    return JSON.parse(responseText) as T;
  }

  private url(path: string): string {
    return new URL(path, this.baseUrl).toString();
  }
}

export function createSoftprobeRuntimeClient(options: SoftprobeRuntimeClientOptions): SoftprobeRuntimeClient {
  return new SoftprobeRuntimeClient(options.baseUrl, options.fetchImpl, options.apiToken);
}

function resolveBearerToken(explicit?: string): string | undefined {
  const candidate = explicit ?? process.env.SOFTPROBE_API_TOKEN;
  if (candidate === undefined) {
    return undefined;
  }
  const trimmed = candidate.trim();
  return trimmed === '' ? undefined : trimmed;
}

function classifyRuntimeError(status: number, body: string, url: string = ''): RuntimeError {
  try {
    const parsed = JSON.parse(body) as { error?: { code?: string } };
    if (parsed.error?.code === 'unknown_session') {
      return new SoftprobeUnknownSessionError(status, body, url);
    }
  } catch {
    // Fall back to the generic runtime error when the body is not JSON.
  }

  return new RuntimeError(status, body, url);
}
