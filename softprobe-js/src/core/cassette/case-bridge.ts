/**
 * Converts between Softprobe v4.1 cassette records (in-process replay shape)
 * and OTLP JSON spans inside a case document (spec/schemas/case.schema.json).
 */

import type { Protocol, RecordType, SoftprobeCassetteRecord } from '../../types/schema';

type OtlpAttr = { key: string; value: Record<string, unknown> };

interface OtlpSpan {
  traceId?: string;
  spanId?: string;
  parentSpanId?: string;
  name?: string;
  kind?: number;
  startTimeUnixNano?: string;
  attributes?: OtlpAttr[];
}

interface CaseDoc {
  traces?: Array<{
    resourceSpans?: Array<{
      resource?: { attributes?: OtlpAttr[] };
      scopeSpans?: Array<{ spans?: OtlpSpan[] }>;
    }>;
  }>;
}

function otlpString(value: string): Record<string, unknown> {
  return { stringValue: value };
}

function otlpInt(n: number): Record<string, unknown> {
  return { intValue: n };
}

function readAttrString(attrs: OtlpAttr[] | undefined, key: string): string | undefined {
  if (!attrs) return undefined;
  for (const a of attrs) {
    if (a.key !== key) continue;
    const v = a.value;
    if (typeof v.stringValue === 'string') return v.stringValue;
    if (typeof v.intValue === 'number') return String(v.intValue);
    if (typeof v.intValue === 'string') return v.intValue;
  }
  return undefined;
}

function trafficDirectionFromRecordType(t: RecordType): string {
  if (t === 'inbound') return 'inbound';
  if (t === 'outbound') return 'outbound';
  return 'inbound';
}

function spanTypeFromRecordType(t: RecordType): string {
  if (t === 'inbound') return 'inject';
  if (t === 'outbound') return 'extract';
  return 'inject';
}

/**
 * Builds one case `traces[]` element (resourceSpans bundle) from v4.1 records sharing one traceId.
 */
export function recordsToCaseTrace(
  records: SoftprobeCassetteRecord[],
  serviceName: string
): { resourceSpans: NonNullable<CaseDoc['traces']>[0]['resourceSpans'] } {
  const spans: OtlpSpan[] = [];
  for (const r of records) {
    spans.push(recordToOtlpSpan(r));
  }
  return {
    resourceSpans: [
      {
        resource: {
          attributes: [{ key: 'service.name', value: otlpString(serviceName) }],
        },
        scopeSpans: [{ spans }],
      },
    ],
  };
}

export function buildCaseDocumentFromRecords(
  records: SoftprobeCassetteRecord[],
  meta: { caseId: string; version?: string; mode?: string; serviceName?: string }
): Record<string, unknown> {
  const serviceName = meta.serviceName ?? 'softprobe-js';
  const traceId = records[0]?.traceId ?? meta.caseId;
  const sameTrace = records.filter((r) => r.traceId === traceId);
  const traces = [recordsToCaseTrace(sameTrace.length > 0 ? sameTrace : records, serviceName)];
  return {
    version: meta.version ?? '1.0.0',
    caseId: meta.caseId,
    mode: meta.mode ?? 'replay',
    createdAt: new Date().toISOString(),
    traces,
    rules: [],
    fixtures: [],
  };
}

function recordToOtlpSpan(record: SoftprobeCassetteRecord): OtlpSpan {
  const attrs: OtlpAttr[] = [
    { key: 'sp.traffic.direction', value: otlpString(trafficDirectionFromRecordType(record.type)) },
    { key: 'sp.span.type', value: otlpString(spanTypeFromRecordType(record.type)) },
    { key: 'softprobe.protocol', value: otlpString(record.protocol) },
    { key: 'softprobe.identifier', value: otlpString(record.identifier) },
  ];

  if (record.protocol === 'http') {
    const idx = record.identifier.indexOf(' ');
    const method = idx < 0 ? 'GET' : record.identifier.slice(0, idx).trim();
    const rest = idx < 0 ? record.identifier : record.identifier.slice(idx + 1).trim();
    attrs.push({ key: 'http.request.method', value: otlpString(method) });
    if (rest.startsWith('http://') || rest.startsWith('https://')) {
      try {
        const u = new URL(rest);
        attrs.push({ key: 'url.host', value: otlpString(u.host) });
        attrs.push({ key: 'url.path', value: otlpString(`${u.pathname}${u.search}` || '/') });
      } catch {
        attrs.push({ key: 'url.path', value: otlpString(rest) });
      }
    } else {
      attrs.push({ key: 'url.path', value: otlpString(rest.startsWith('/') ? rest : `/${rest}`) });
    }
  }

  const reqBody = record.requestPayload as { body?: unknown } | undefined;
  if (reqBody?.body !== undefined) {
    const s = typeof reqBody.body === 'string' ? reqBody.body : JSON.stringify(reqBody.body);
    attrs.push({ key: 'http.request.body', value: otlpString(s) });
  }

  const res = record.responsePayload as {
    statusCode?: number;
    status?: number;
    body?: unknown;
    headers?: Record<string, string>;
  } | undefined;
  const statusNum = res?.statusCode ?? res?.status;
  if (statusNum !== undefined) {
    attrs.push({ key: 'http.response.status_code', value: otlpInt(Number(statusNum)) });
  }
  if (res?.body !== undefined) {
    const s = typeof res.body === 'string' ? res.body : JSON.stringify(res.body);
    attrs.push({ key: 'http.response.body', value: otlpString(s) });
  }
  if (res?.headers) {
    for (const [hk, hv] of Object.entries(res.headers)) {
      attrs.push({ key: `http.response.header.${hk.toLowerCase()}`, value: otlpString(hv) });
    }
  }

  if (record.requestPayload !== undefined) {
    attrs.push({
      key: 'softprobe.record.request_payload_json',
      value: otlpString(JSON.stringify(record.requestPayload)),
    });
  }
  if (record.responsePayload !== undefined) {
    attrs.push({
      key: 'softprobe.record.response_payload_json',
      value: otlpString(JSON.stringify(record.responsePayload)),
    });
  }

  const startNanos = isoToUnixNanos(record.timestamp);
  return {
    traceId: record.traceId,
    spanId: record.spanId,
    ...(record.parentSpanId ? { parentSpanId: record.parentSpanId } : {}),
    ...(record.spanName ? { name: record.spanName } : {}),
    ...(startNanos ? { startTimeUnixNano: startNanos } : {}),
    kind: record.type === 'inbound' ? 2 : 3,
    attributes: attrs,
  };
}

function isoToUnixNanos(iso: string | undefined): string | undefined {
  if (!iso) return undefined;
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return undefined;
  return String(ms * 1_000_000);
}

function unixNanosToIso(nanos: string | undefined): string {
  if (!nanos) return new Date().toISOString();
  const n = BigInt(nanos);
  const ms = Number(n / 1_000_000n);
  return new Date(ms).toISOString();
}

/**
 * Flattens a case document into v4.1 cassette records (HTTP-focused; postgres/redis
 * spans are recognized via `softprobe.protocol` when present).
 */
export function caseDocumentToCassetteRecords(caseDocument: unknown): SoftprobeCassetteRecord[] {
  const doc = caseDocument as CaseDoc;
  const out: SoftprobeCassetteRecord[] = [];
  for (const trace of doc.traces ?? []) {
    for (const rs of trace.resourceSpans ?? []) {
      for (const ss of rs.scopeSpans ?? []) {
        for (const span of ss.spans ?? []) {
          const rec = otlpSpanToRecord(span);
          if (rec) out.push(rec);
        }
      }
    }
  }
  return out;
}

function otlpSpanToRecord(span: OtlpSpan): SoftprobeCassetteRecord | null {
  const attrs = span.attributes ?? [];
  const id = readAttrString(attrs, 'softprobe.identifier');
  const proto = (readAttrString(attrs, 'softprobe.protocol') ?? 'http') as Protocol;
  const direction = readAttrString(attrs, 'sp.traffic.direction');
  const spanType = readAttrString(attrs, 'sp.span.type');
  const method = readAttrString(attrs, 'http.request.method');

  if (!id && !method && proto === 'http') {
    return null;
  }

  const recordType = mapToRecordType(direction, spanType);
  const identifier =
    id ??
    (() => {
      const m = method ?? 'GET';
      const host = readAttrString(attrs, 'url.host');
      const p = readAttrString(attrs, 'url.path') ?? '/';
      return host ? `${m} https://${host}${p}` : `${m} ${p}`;
    })();

  const reqBodyStr = readAttrString(attrs, 'http.request.body');
  const statusRaw = readAttrString(attrs, 'http.response.status_code');
  const resBodyStr = readAttrString(attrs, 'http.response.body');
  const HTTP_RESPONSE_HEADER_PREFIX = 'http.response.header.';
  const headerObj: Record<string, string> = {};
  for (const a of attrs) {
    if (a.key.startsWith(HTTP_RESPONSE_HEADER_PREFIX)) {
      const name = a.key.slice(HTTP_RESPONSE_HEADER_PREFIX.length);
      headerObj[name] = readAttrString(attrs, a.key) ?? '';
    }
  }

  const record: SoftprobeCassetteRecord = {
    version: '4.1',
    traceId: span.traceId ?? '',
    spanId: span.spanId ?? '',
    ...(span.parentSpanId ? { parentSpanId: span.parentSpanId } : {}),
    ...(span.name ? { spanName: span.name } : {}),
    timestamp: unixNanosToIso(span.startTimeUnixNano),
    type: recordType,
    protocol: proto,
    identifier,
  };

  const lumpReq = readAttrString(attrs, 'softprobe.record.request_payload_json');
  if (lumpReq) {
    try {
      const parsedReq = JSON.parse(lumpReq) as unknown;
      record.requestPayload = parsedReq as SoftprobeCassetteRecord['requestPayload'];
    } catch {
      // ignore
    }
  } else if (reqBodyStr !== undefined && reqBodyStr !== '') {
    record.requestPayload = { body: tryJson(reqBodyStr) };
  }

  const fromHttp: Record<string, unknown> = {};
  if (statusRaw !== undefined) {
    const statusCode = Number.parseInt(statusRaw, 10);
    if (!Number.isNaN(statusCode)) {
      fromHttp.statusCode = statusCode;
    }
  }
  if (resBodyStr !== undefined && resBodyStr !== '') {
    fromHttp.body = tryJson(resBodyStr);
  }
  if (Object.keys(headerObj).length > 0) {
    fromHttp.headers = headerObj;
  }

  const lumpRes = readAttrString(attrs, 'softprobe.record.response_payload_json');
  if (lumpRes) {
    try {
      const parsedRes = JSON.parse(lumpRes) as unknown;
      if (parsedRes !== null && typeof parsedRes === 'object' && !Array.isArray(parsedRes)) {
        record.responsePayload = { ...fromHttp, ...(parsedRes as Record<string, unknown>) };
      } else {
        record.responsePayload = parsedRes as SoftprobeCassetteRecord['responsePayload'];
      }
    } catch {
      if (Object.keys(fromHttp).length > 0) {
        record.responsePayload = fromHttp;
      }
    }
  } else if (Object.keys(fromHttp).length > 0) {
    record.responsePayload = fromHttp;
  }

  return record;
}

function tryJson(s: string): unknown {
  try {
    return JSON.parse(s) as unknown;
  } catch {
    return s;
  }
}

function mapToRecordType(direction: string | undefined, spanType: string | undefined): RecordType {
  if (direction === 'outbound' || spanType === 'extract') return 'outbound';
  if (direction === 'inbound' || spanType === 'inject') return 'inbound';
  return 'metadata';
}

export function filterRecordsByTraceId(records: SoftprobeCassetteRecord[], traceId: string): SoftprobeCassetteRecord[] {
  return records.filter((r) => r.traceId === traceId);
}
