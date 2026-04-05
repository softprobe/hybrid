# Softprobe Proxy OTEL API

This document defines the proxy-facing wire protocol used by the Envoy/WASM extension.

This is the canonical contract for proxy integration. It is protobuf-first and follows the current `proxy` implementation design.

---

## 1) Transport

- HTTP
- `Content-Type: application/x-protobuf`
- body encoded as OTEL `TracesData`

---

## 2) Endpoints

### `POST /v1/inject`

Purpose:

- request-path lookup for possible injected response data

Request:

- OTEL protobuf `TracesData`
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

- `200` with OTEL protobuf body:
  - hit
  - proxy parses `http.response.*` attributes and injects the response
- `404`:
  - miss
  - proxy forwards upstream normally
- other non-success responses:
  - error path
  - proxy applies local fallback or strict failure behavior

Response body on hit:

- OTEL protobuf `TracesData`
- span attributes used by the current proxy parser:
  - `http.response.status_code`
  - `http.response.header.<name>`
  - `http.response.body`

### `POST /v1/traces`

Purpose:

- response-path extraction upload for observed traffic

Request:

- OTEL protobuf `TracesData`
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

- injection lookup is modeled as an OTEL/protobuf request-response exchange
- extraction upload is modeled as an asynchronous OTEL/protobuf write
- miss semantics are represented by HTTP `404`, not by a JSON response envelope

This contract should remain stable unless the proxy implementation changes materially.
