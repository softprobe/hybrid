# Softprobe Test Control API

This document defines the higher-level runtime control API used by test SDKs, CLI tools, and automation.

It does not define the proxy wire protocol. The proxy-facing contract is documented separately in [proxy-otel-api.md](./proxy-otel-api.md).

## Terminology

Use these terms consistently:

- `session`: one test-scoped control context
- `case`: one stored test artifact
- `rule`: one matching rule that influences injection or extraction behavior
- `inject`: runtime prepares data that the proxy may return without forwarding upstream
- `extract`: runtime accepts observed traffic for storage/export

## Core endpoints

- `POST /v1/sessions`
- `POST /v1/sessions/{sessionId}/load-case`
- `POST /v1/sessions/{sessionId}/policy`
- `POST /v1/sessions/{sessionId}/rules`
- `POST /v1/sessions/{sessionId}/fixtures/auth`
- `POST /v1/sessions/{sessionId}/close`

## Purpose

- language SDKs use these endpoints directly or through a local client
- proxy should not call these endpoints directly for request-path lookup

## Relationship to proxy APIs

Typical flow:

1. test code creates a session through this API
2. test code loads a case or adds rules through this API
3. test requests carry `x-softprobe-session-id`
4. proxy calls `/v1/inject` using OTEL protobuf
5. runtime resolves the lookup using the session and case state created through this API

This keeps the user-facing control surface ergonomic while preserving the existing proxy/OTEL wire design.

Detailed request and response payloads are defined by the JSON Schemas in `../schemas/`.
