# Softprobe Proxy OTEL API

This document defines the proxy-facing wire protocol used by the Envoy/WASM extension.

This is the canonical contract for proxy integration. The proxy backend exposes standard OTLP collector ingestion endpoints and a Softprobe-specific inject endpoint that accepts the same OTLP trace payload family.

## Who implements this API

The **proxy backend** implements **`POST /v1/inject`** and standard OTLP collector endpoints such as **`POST /v1/traces`**. In many deployments this is a **hosted** service (for example **`https://o.softprobe.ai`**); self-hosted backends are allowed if they honor the same contract. The **`softprobe-runtime`** OSS service that implements the [HTTP control API](./http-control-api.md) does **not** implement this OTLP API.

The proxy (Envoy/WASM) is a **client** and does **not** call the JSON control API on the request path. Internal datastore, scaling, and how the backend stays aligned with control API session data are **opaque** here—see [platform-architecture.md](../../docs/platform-architecture.md#10-softprobe-runtime-implementation-and-deployment) and `docs/design.md` open questions.

---

## 1) Transport

- HTTP
- OTLP `TracesData` payloads
- `Content-Type: application/x-protobuf` or `application/json`
- `Accept: application/x-protobuf` or `application/json` on endpoints that return OTLP payloads

The current proxy implementation may remain protobuf-first, but the backend contract accepts the same trace schema in either OTLP protobuf or OTLP JSON.

---

## 2) Endpoints

### `POST /v1/inject`

Purpose:

- request-path lookup for possible injected response data

Request:

- OTLP `TracesData` in protobuf or JSON form
- contains one or more spans describing the candidate HTTP exchange
- primary marker: `sp.span.type = "inject"`

Expected request attributes include:

- `sp.span.type = "inject"`
- `sp.session.id`
- `sp.service.name`
- `sp.traffic.direction`
- `url.host`
- `url.path`
- `http.request.header.<name>`
- `http.request.body`

Response semantics:

- `200` with OTLP `TracesData` body in the negotiated response format:
  - hit
  - proxy parses `http.response.*` attributes and injects the response
- `404`:
  - miss
  - proxy forwards upstream normally
- other non-success responses:
  - error path
  - proxy applies local fallback or strict failure behavior

Response body on hit:

- OTLP `TracesData`
- span attributes used by the current proxy parser:
  - `http.response.status_code`
  - `http.response.header.<name>`
  - `http.response.body`

### `POST /v1/traces`

Purpose:

- response-path extraction upload for observed traffic

Request:

- OTLP `TracesData` in protobuf or JSON form
- primary marker: `sp.span.type = "extract"`

Expected extract attributes include:

- `sp.span.type = "extract"`
- `sp.session.id`
- `sp.service.name`
- `sp.traffic.direction`
- `url.host`
- `url.path`
- `http.request.header.<name>`
- `http.request.body`
- `http.response.header.<name>`
- `http.response.status_code`
- `http.response.body`

Response semantics:

- `2xx` => accepted
- non-`2xx` => extraction failure

---

## 3) Why this contract exists

This matches the current proxy code structure:

- injection lookup is modeled as an OTLP trace request-response exchange
- extraction upload is modeled as an asynchronous OTLP write
- miss semantics are represented by HTTP `404`, not by a JSON response envelope

This contract should remain stable unless the proxy implementation changes materially.
