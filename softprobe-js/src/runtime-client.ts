import { fetch as undiciFetch } from 'undici';

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
}

export class SoftprobeRuntimeError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: string
  ) {
    super(`softprobe runtime request failed: status ${status}: ${body.trim()}`);
    this.name = 'SoftprobeRuntimeError';
  }
}

export class SoftprobeRuntimeUnreachableError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'SoftprobeRuntimeUnreachableError';
  }
}

export class SoftprobeUnknownSessionError extends SoftprobeRuntimeError {
  constructor(status: number, body: string) {
    super(status, body);
    this.name = 'SoftprobeUnknownSessionError';
  }
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

  constructor(private readonly baseUrl: string, fetchImpl: FetchLike = undiciFetch as unknown as FetchLike) {
    this.fetchImpl = fetchImpl;
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
    let response: ResponseLike;
    try {
      response = await this.fetchImpl(this.url(path), {
        method: 'POST',
        headers: {
          'content-type': 'application/json',
        },
        body: JSON.stringify(body),
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      throw new SoftprobeRuntimeUnreachableError(`softprobe runtime is unreachable: ${message}`);
    }

    const responseText = await response.text();
    if (!response.ok) {
      throw classifyRuntimeError(response.status, responseText);
    }

    return JSON.parse(responseText) as T;
  }

  private url(path: string): string {
    return new URL(path, this.baseUrl).toString();
  }
}

export function createSoftprobeRuntimeClient(options: SoftprobeRuntimeClientOptions): SoftprobeRuntimeClient {
  return new SoftprobeRuntimeClient(options.baseUrl, options.fetchImpl);
}

function classifyRuntimeError(status: number, body: string): SoftprobeRuntimeError {
  try {
    const parsed = JSON.parse(body) as { error?: { code?: string } };
    if (parsed.error?.code === 'unknown_session') {
      return new SoftprobeUnknownSessionError(status, body);
    }
  } catch {
    // Fall back to the generic runtime error when the body is not JSON.
  }

  return new SoftprobeRuntimeError(status, body);
}
