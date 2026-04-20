/**
 * Framework-agnostic helpers for reading captured responses out of a
 * case document (OTLP-shaped JSON as produced by `softprobe` capture).
 *
 * These helpers are intentionally small and synchronous. They are shared
 * by SDK code that needs to materialize a captured span into a mock rule
 * (see `docs/design.md` §3.2.1 / §3.2.3).
 */

export interface CaseSpanPredicate {
  direction?: string;
  service?: string;
  host?: string;
  hostSuffix?: string;
  method?: string;
  path?: string;
  pathPrefix?: string;
}

export interface CapturedResponse {
  status: number;
  headers: Record<string, string>;
  body: string;
}

export interface CapturedSpan {
  traceId?: string;
  spanId?: string;
  name?: string;
  attributes?: Array<{ key: string; value: unknown }>;
}

export interface CapturedHit {
  response: CapturedResponse;
  span: CapturedSpan;
}

interface OtlpAttribute {
  key: string;
  value: Record<string, unknown>;
}

interface OtlpSpan extends CapturedSpan {
  attributes?: OtlpAttribute[];
}

interface OtlpResource {
  attributes?: OtlpAttribute[];
}

interface OtlpScopeSpans {
  spans?: OtlpSpan[];
}

interface OtlpResourceSpans {
  resource?: OtlpResource;
  scopeSpans?: OtlpScopeSpans[];
}

interface OtlpTrace {
  resourceSpans?: OtlpResourceSpans[];
}

interface CaseDocument {
  traces?: OtlpTrace[];
}

const HTTP_RESPONSE_HEADER_PREFIX = 'http.response.header.';

/**
 * Returns every inject/extract span in the case whose attributes satisfy
 * `predicate`. Callers decide whether zero / one / many matches is an error.
 */
export function findSpans(
  caseDocument: unknown,
  predicate: CaseSpanPredicate
): Array<{ span: CapturedSpan; serviceName: string | undefined }> {
  const doc = (caseDocument ?? {}) as CaseDocument;
  const matches: Array<{ span: CapturedSpan; serviceName: string | undefined }> = [];

  for (const trace of doc.traces ?? []) {
    for (const resourceSpan of trace.resourceSpans ?? []) {
      const serviceName = readAttributeString(resourceSpan.resource?.attributes, 'service.name');
      for (const scopeSpan of resourceSpan.scopeSpans ?? []) {
        for (const span of scopeSpan.spans ?? []) {
          if (spanSatisfies(span, serviceName, predicate)) {
            matches.push({ span, serviceName });
          }
        }
      }
    }
  }

  return matches;
}

/**
 * Extracts a response object from a captured span using the HTTP attribute
 * profile defined in `spec/protocol/proxy-otel-api.md`.
 *
 * Missing `http.response.body` becomes `""`, missing headers become `{}`.
 * `http.response.status_code` must be present — a captured span without a
 * status is an authoring error, so we fail fast.
 */
export function responseFromSpan(span: CapturedSpan): CapturedResponse {
  const attrs = (span.attributes ?? []) as OtlpAttribute[];
  const headers: Record<string, string> = {};
  let status: number | undefined;
  let body = '';

  for (const attr of attrs) {
    if (attr.key === 'http.response.status_code') {
      const raw = attr.value as Record<string, unknown>;
      if (typeof raw.intValue === 'number') {
        status = raw.intValue;
      } else if (typeof raw.intValue === 'string') {
        status = Number.parseInt(raw.intValue, 10);
      }
    } else if (attr.key === 'http.response.body') {
      body = anyValueToString(attr.value);
    } else if (attr.key.startsWith(HTTP_RESPONSE_HEADER_PREFIX)) {
      const name = attr.key.slice(HTTP_RESPONSE_HEADER_PREFIX.length);
      headers[name] = anyValueToString(attr.value);
    }
  }

  if (status === undefined || Number.isNaN(status)) {
    throw new Error(
      `Captured span ${span.spanId ?? '<unknown>'} is missing http.response.status_code; ` +
        'cannot materialize a captured response.'
    );
  }

  return { status, headers, body };
}

/**
 * Produces a short summary of a predicate for error messages; keeps only
 * fields the user actually set so the output is compact.
 */
export function formatPredicate(predicate: CaseSpanPredicate): string {
  const entries = Object.entries(predicate).filter(([, v]) => v !== undefined && v !== '');
  return entries.length === 0
    ? '{}'
    : `{ ${entries.map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(', ')} }`;
}

function spanSatisfies(
  span: OtlpSpan,
  serviceName: string | undefined,
  predicate: CaseSpanPredicate
): boolean {
  const attrs = span.attributes ?? [];
  const spanType = readAttributeString(attrs, 'sp.span.type') ?? '';
  if (spanType !== 'inject' && spanType !== 'extract') {
    return false;
  }

  const direction = readAttributeString(attrs, 'sp.traffic.direction');
  if (predicate.direction && direction !== predicate.direction) {
    return false;
  }

  const method =
    readAttributeString(attrs, 'http.request.method') ??
    readAttributeString(attrs, 'http.request.header.:method');
  if (predicate.method && method !== predicate.method) {
    return false;
  }

  const urlPath =
    readAttributeString(attrs, 'url.path') ??
    readAttributeString(attrs, 'http.request.header.:path') ??
    '';
  if (predicate.path !== undefined && urlPath !== predicate.path) {
    return false;
  }
  if (predicate.pathPrefix !== undefined && !urlPath.startsWith(predicate.pathPrefix)) {
    return false;
  }

  const host = readAttributeString(attrs, 'url.host') ?? '';
  if (predicate.host !== undefined && host !== predicate.host) {
    return false;
  }
  if (predicate.hostSuffix !== undefined && !host.endsWith(predicate.hostSuffix)) {
    return false;
  }

  const spanService = readAttributeString(attrs, 'sp.service.name') ?? serviceName ?? '';
  if (predicate.service !== undefined && spanService !== predicate.service) {
    return false;
  }

  return true;
}

function readAttributeString(
  attrs: OtlpAttribute[] | undefined,
  key: string
): string | undefined {
  if (!attrs) return undefined;
  for (const attr of attrs) {
    if (attr.key === key) {
      return anyValueToString(attr.value);
    }
  }
  return undefined;
}

function anyValueToString(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  const obj = value as Record<string, unknown>;
  if (typeof obj.stringValue === 'string') return obj.stringValue;
  if (typeof obj.intValue === 'number') return String(obj.intValue);
  if (typeof obj.intValue === 'string') return obj.intValue;
  if (typeof obj.boolValue === 'boolean') return String(obj.boolValue);
  if (typeof obj.doubleValue === 'number') return String(obj.doubleValue);
  return '';
}
