import { readFile } from 'fs/promises';

import {
  CapturedHit,
  CaseSpanPredicate,
  findSpans,
  formatPredicate,
  responseFromSpan,
} from './core/case/find-span';
import { createSoftprobeRuntimeClient, SoftprobeRuntimeClient, SoftprobeRuntimeClientOptions } from './runtime-client';

export interface SoftprobeOptions extends Omit<SoftprobeRuntimeClientOptions, 'baseUrl'> {
  baseUrl?: string;
}

export interface SoftprobeSessionStartInput {
  mode: 'capture' | 'replay' | 'generate';
}

export interface SoftprobeResponseSpec {
  status: number;
  body?: unknown;
  headers?: Record<string, string>;
}

export interface SoftprobeRuleSpec {
  id?: string;
  priority?: number;
  direction?: 'inbound' | 'outbound';
  service?: string;
  host?: string;
  hostSuffix?: string;
  method?: string;
  path?: string;
  pathPrefix?: string;
}

export interface SoftprobeMockRuleSpec extends SoftprobeRuleSpec {
  response: SoftprobeResponseSpec;
}

export type SoftprobeFindInCaseSpec = CaseSpanPredicate;

export type { CapturedHit } from './core/case/find-span';

const DEFAULT_BASE_URL = 'http://127.0.0.1:8080';

/**
 * Ergonomic SDK facade for the Softprobe control runtime (see `docs/design.md` §3.2).
 */
export class Softprobe {
  private readonly client: SoftprobeRuntimeClient;

  constructor(private readonly opts: SoftprobeOptions = {}) {
    const baseUrl = opts.baseUrl ?? process.env.SOFTPROBE_RUNTIME_URL ?? DEFAULT_BASE_URL;
    this.client = createSoftprobeRuntimeClient({
      baseUrl,
      fetchImpl: opts.fetchImpl,
    });
  }

  async startSession(input: SoftprobeSessionStartInput): Promise<SoftprobeSession> {
    const response = await this.client.createSession({ mode: input.mode });
    return new SoftprobeSession(response.sessionId, this.client);
  }

  attach(sessionId: string): SoftprobeSession {
    return new SoftprobeSession(sessionId, this.client);
  }
}

/**
 * Session-bound helper. Holds the parsed case in memory after `loadCaseFromFile`
 * so `findInCase` can do pure, synchronous lookups.
 */
export class SoftprobeSession {
  private readonly rules: unknown[] = [];
  private loadedCase: unknown | null = null;

  constructor(
    readonly id: string,
    private readonly client: SoftprobeRuntimeClient
  ) {}

  async loadCaseFromFile(casePath: string): Promise<void> {
    const caseDocument = JSON.parse(await readFile(casePath, 'utf8'));
    this.loadedCase = caseDocument;
    await this.client.loadCase(this.id, caseDocument);
  }

  /**
   * Pure in-memory lookup against the case most recently loaded via
   * `loadCaseFromFile`. Returns the captured response (and raw span) for
   * the **single** matching inject/extract span.
   *
   * Throws if zero or more than one span matches the predicate, so the
   * author fixes ambiguity at authoring time (see `docs/design.md` §3.2.3).
   */
  findInCase(spec: SoftprobeFindInCaseSpec): CapturedHit {
    if (this.loadedCase === null) {
      throw new Error(
        'findInCase requires a case: call `await session.loadCaseFromFile(path)` before `findInCase`.'
      );
    }

    const matches = findSpans(this.loadedCase, spec);
    if (matches.length === 0) {
      throw new Error(
        `findInCase: no span in the loaded case matches ${formatPredicate(spec)}. ` +
          'Check the predicate (direction / method / path / host) or re-capture the case.'
      );
    }
    if (matches.length > 1) {
      const ids = matches
        .map((m) => m.span.spanId ?? '<unknown>')
        .join(', ');
      throw new Error(
        `findInCase: ${matches.length} spans match ${formatPredicate(spec)}. ` +
          `Disambiguate the predicate — candidate span ids: ${ids}.`
      );
    }

    const [{ span }] = matches;
    return { response: responseFromSpan(span), span };
  }

  async mockOutbound(spec: SoftprobeMockRuleSpec): Promise<void> {
    this.rules.push(this.buildMockRule(spec));
    await this.syncRules();
  }

  async clearRules(): Promise<void> {
    this.rules.length = 0;
    await this.client.updateRules(this.id, { version: 1, rules: [] });
  }

  async close(): Promise<void> {
    await this.client.closeSession(this.id);
  }

  private async syncRules(): Promise<void> {
    await this.client.updateRules(this.id, { version: 1, rules: this.rules });
  }

  private buildMockRule(spec: SoftprobeMockRuleSpec): Record<string, unknown> {
    const when: Record<string, unknown> = {};

    if (spec.direction) when.direction = spec.direction;
    if (spec.service) when.service = spec.service;
    if (spec.host) when.host = spec.host;
    if (!spec.host && spec.hostSuffix) when.host = spec.hostSuffix;
    if (spec.method) when.method = spec.method;
    if (spec.path) when.path = spec.path;
    if (spec.pathPrefix) when.pathPrefix = spec.pathPrefix;

    const rule: Record<string, unknown> = {
      when,
      then: {
        action: 'mock',
        response: spec.response,
      },
    };

    if (spec.id) rule.id = spec.id;
    if (spec.priority !== undefined) rule.priority = spec.priority;

    return rule;
  }
}
