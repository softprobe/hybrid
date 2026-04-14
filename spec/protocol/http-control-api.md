# Softprobe Test Control API

This document defines the higher-level runtime control API used by test SDKs, CLI tools, and automation.

**Primary surface for users and agents:** Documentation and tutorials should lead with the **canonical `softprobe` CLI**, which **calls** this API on the **Softprobe Runtime**. Direct HTTP use is appropriate for SDK authors, debugging, and contract tests.

This document does **not** define the proxy data-plane protocol. Request-path **inject** and async **extract** use the OTLP trace protocol described in [proxy-otel-api.md](./proxy-otel-api.md) (protobuf or JSON payloads); test authors normally do not implement that protocol.

## Who implements this API

The **`softprobe-runtime`** service (OSS / self-hosted) implements these **JSON** endpoints. The **`softprobe` CLI and language SDKs are HTTP clients** of that service.

**Inject and extract** for the mesh use the **OTLP endpoints** defined in [proxy-otel-api.md](./proxy-otel-api.md). In the **OSS reference layout**, the **same `softprobe-runtime` process** serves **both** this control API and **`POST /v1/inject`** / **`POST /v1/traces`** from one base URL and one in-memory session store ([`docs/design.md`](../../docs/design.md) §2.4, §4.3). Hosted or split deployments may run the OTLP “proxy backend” as a separate service (e.g. **`https://o.softprobe.ai`**) as long as the wire contract matches. Envoy/WASM does **not** call this JSON API on the request path.

**Datastore:** the reference control runtime does **not** require a database for v1 (in-memory sessions). Deployment, HA, and languages are defined in [platform-architecture.md](../../docs/platform-architecture.md#10-softprobe-runtime-implementation-and-deployment) and [repo-layout.md](../../docs/repo-layout.md).

## Terminology

Use these terms consistently:

- `session`: one test-scoped control context
- `case`: one stored test artifact
- `rule`: one matching rule that influences injection or extraction behavior
- `inject`: runtime prepares data that the proxy may return without forwarding upstream
- `extract`: runtime accepts observed traffic for storage/export

## Core endpoints

- `POST /v1/sessions` — request: [session-create.request.schema.json](../schemas/session-create.request.schema.json); response: [session-create.response.schema.json](../schemas/session-create.response.schema.json)
- `POST /v1/sessions/{sessionId}/load-case` — request: [session-load-case.request.schema.json](../schemas/session-load-case.request.schema.json); response: [session-load-case.response.schema.json](../schemas/session-load-case.response.schema.json)
- `POST /v1/sessions/{sessionId}/policy` — request: [session-policy.request.schema.json](../schemas/session-policy.request.schema.json); response: [session-policy.response.schema.json](../schemas/session-policy.response.schema.json)
- `POST /v1/sessions/{sessionId}/rules` — request: [session-rules.request.schema.json](../schemas/session-rules.request.schema.json); response: [session-rules.response.schema.json](../schemas/session-rules.response.schema.json)
- `POST /v1/sessions/{sessionId}/fixtures/auth` — request: [session-fixtures-auth.request.schema.json](../schemas/session-fixtures-auth.request.schema.json); response: [session-fixtures-auth.response.schema.json](../schemas/session-fixtures-auth.response.schema.json)
- `POST /v1/sessions/{sessionId}/close` — request: [session-close.request.schema.json](../schemas/session-close.request.schema.json); response: [session-close.response.schema.json](../schemas/session-close.response.schema.json)

Unknown-session failures on mutating control endpoints return [session-error.response.schema.json](../schemas/session-error.response.schema.json) with HTTP `404`.

## Purpose

- the **canonical `softprobe` CLI** calls these endpoints (preferred for humans, CI, and AI agents)
- language SDKs use these endpoints directly or through a local client
- proxy should not call these endpoints directly for request-path lookup

New sessions start with `sessionRevision = 0`.

## Relationship to proxy APIs

Typical flow:

1. test code creates a session through **this** JSON API (control runtime)
2. test code loads a case or adds rules through **this** API
3. test requests carry `x-softprobe-session-id`
4. proxy calls **`POST /v1/inject`** on the OTLP backend using OTLP trace payloads (not this JSON API)
5. that backend resolves the lookup using session/case state that stays **consistent** with this API (in OSS, **same process** as this control API — see `docs/design.md` §4.3)

This keeps the user-facing control surface ergonomic while the mesh data plane uses OTLP to the inject/extract backend (unified or separate per deployment).

Detailed request and response payloads are defined by the JSON Schemas in `../schemas/`.
