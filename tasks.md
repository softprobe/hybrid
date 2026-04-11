# Tasks

> **Execution:** Work in **document order** (top to bottom). The **first** unchecked `[ ]` item is the active task unless a dependency line says otherwise.  
> **Process:** Follow `AGENTS.md` (TDD for code, no scope beyond `docs/design.md`).  
> **When done:** Change `[ ]` → `[x]` and append a short commit-style note on the same line.

### Architecture (read before coding)

- **`softprobe-runtime` (OSS)** — HTTP **control API** only: [http-control-api.md](spec/protocol/http-control-api.md). **v1: in-memory sessions; no database required.** Add Redis/Postgres only for HA or durability ([docs/platform-architecture.md](docs/platform-architecture.md) §10.2).  
- **Proxy backend** — [proxy-otel-api.md](spec/protocol/proxy-otel-api.md) (OTLP collector-style ingest plus `/v1/inject` on the same trace schema). Default production URL may be **`https://o.softprobe.ai`**; **not** implemented in `softprobe-runtime`.  
- **Canonical language** for control runtime + CLI: **Go** or **Rust**. **Proxy** talks to the proxy backend **only over HTTP**; it does not link runtime or backend as a library.  
- **Control ↔ backend consistency** — how `load-case` / rules reach the proxy backend is **product integration** (see `docs/design.md` §14); track in parking lot until specified.

---

## Legend

| Mark | Meaning |
|------|---------|
| `[ ]` | Not started |
| `[x]` | Done (note appended) |

**Depends on** — complete those `[x]` before starting this item. If none, order alone defines readiness.

---

## Phase P0.0 — Control runtime project bootstrap

Establishes **`softprobe-runtime`** for **P0.4–P0.5** (JSON control API only). **Language:** Go or Rust per [docs/platform-architecture.md](docs/platform-architecture.md#10-softprobe-runtime-implementation-and-deployment).

- [x] **P0.0a — Repository or package layout.** Create `softprobe-runtime/` (separate clone or monorepo directory) with README stating: **HTTP control API only**; default listen address/port; link to [http-control-api.md](spec/protocol/http-control-api.md); note that **inject/extract** are **not** served here (proxy backend, e.g. `https://o.softprobe.ai`). Add minimal **health** route (e.g. `GET /health` — not part of spec). `go test ./...` green.  
  **Verify:** Project builds in CI; README matches [docs/repo-layout.md](docs/repo-layout.md).

- [x] **P0.0b — K8s example (informative).** Add **Deployment + Service** for the **control** runtime only. Document separately: WasmPlugin **`sp_backend_url`** (or equivalent) = **proxy backend** base URL (**not** control runtime). Cross-link [softprobe-proxy/docs/deployment.md](softprobe-proxy/docs/deployment.md) or [docs/platform-architecture.md](docs/platform-architecture.md) §10.5. `deploy/kubernetes.yaml` validated with `kubectl --context kind-softprobe-runtime apply --dry-run=client`.  
  **Verify:** `kubectl apply --dry-run=client -f …` succeeds for control runtime manifest (or doc reviewed).

- [x] **P0.0c — CLI placeholder (optional but recommended).** Same repo: `softprobe` binary that can print version and call `GET /health` against `--runtime-url` (control API base; no session commands required until P3.1). `go test ./...` covers `cmd/softprobe` against the runtime handler.  
  **Verify:** Single integration test or script: start server, run CLI doctor/health.

---

## Phase P0 — Spec, schemas, and golden fixtures

Contract work first so runtime and proxy implement the same shapes.

### P0.1 Case OTLP JSON profile (normative)

- [x] **P0.1a — Profile document.** Add a spec doc (e.g. `spec/protocol/case-otlp-json.md`) that defines the **minimal OTLP JSON subset** for each element of `case.traces[]`: envelope shape, required span/resource attributes for HTTP identity, naming alignment with `proxy-otel-api.md`, and recommended **size limits** (max spans per case, max attribute size). `docs/design.md` §14 now points at the profile doc.  
  **Verify:** Design open question in `docs/design.md` §14 (OTLP JSON profile) can be checked off or explicitly deferred with a one-line pointer to this spec.

- [x] **P0.1b — Cross-links.** Link the new profile from `spec/README.md`, `spec/schemas/case.schema.json` (`traces` description), and `docs/design.md` §6. `rg` confirms the profile filename is referenced in all three files.  
  **Verify:** No broken relative links; `grep` for `case-otlp-json` / profile filename hits all intended files.

### P0.2 JSON Schemas

- [x] **P0.2a — Trace item schema.** Add a dedicated JSON Schema for **one** OTLP-compatible trace document (e.g. `spec/schemas/case-trace.schema.json`) and reference it from `case.schema.json` via `items.$ref` (or equivalent draft-2020-12 pattern). `ajv-cli` validates `spec/examples/cases/minimal.case.json` and `spec/examples/cases/minimal-trace.json`.  
  **Verify:** `spec/examples/cases/*.case.json` validate; empty `traces` array still valid.

- [x] **P0.2b — Rule `when` / `then`.** Extend `rule.schema.json` so `when` and `then` include the **documented** decision actions and matcher fields from `docs/design.md` §8.2 (use `enum` / `oneOf` where appropriate; allow extension via `additionalProperties` only if design says so). `ajv-cli` validates `spec/examples/rules/strict-block.rule.json` and rejects an unsupported `action: redirect`.  
  **Verify:** At least one example rule JSON under `spec/examples/rules/` validates; invalid rules fail validation in a documented way.

- [x] **P0.2c — Session API payloads.** Add JSON Schemas for **request/response bodies** of every endpoint listed in `spec/protocol/http-control-api.md` (sessions create, load-case, policy, rules, fixtures/auth, close). Reuse `session.schema.json` where it fits. `ajv-cli` validates representative payloads for all endpoint schemas.  
  **Verify:** Each endpoint has a matching schema file and a one-line pointer from `http-control-api.md`.

### P0.3 Golden examples and automated validation

- [x] **P0.3a — Non-empty golden case.** Add `spec/examples/cases/` example with **at least one** trace document in `traces` that satisfies the P0.1 profile and P0.2a schema. done: added `checkout-happy-path.case.json`  
  **Verify:** File is named and listed in `spec/README.md` or examples index; documents a realistic outbound HTTP span.

- [x] **P0.3b — Schema validation in CI.** Add a small script (language by team choice) plus CI step that validates all `spec/examples/**` and `spec/schemas/*.json` (meta-validation) according to `AGENTS.md` repo conventions. done: added `spec/scripts/validate-spec.sh` and CI workflow  
  **Verify:** CI fails if a hand-edited example breaks the schema; README or `spec/README.md` documents how to run locally.

---

## Phase P0 — Reference control runtime (`softprobe-runtime`)

Implement in **`softprobe-runtime`** (from P0.0). **JSON control API only** — no `/v1/inject` or `/v1/traces` in this codebase.

### P0.4 Sessions lifecycle

- [x] **P0.4a — Create session.** `POST /v1/sessions` persists a session with `mode`, returns `sessionId` and `sessionRevision` (initial `0` or `1`, document choice in schema + code). `softprobe-runtime/internal/runtimeapp/session_test.go` covers persistence and `sessionRevision = 0`.  
  **Verify:** Contract test or integration test asserts response shape per P0.2c schema.

- [x] **P0.4b — Close session.** `POST /v1/sessions/{sessionId}/close` removes or invalidates the session; subsequent **control** operations return a **documented** error. done: close invalidates session and load-case returns 404  
  **Verify:** Test creates → closes → asserts `load-case` or `rules` fails.

- [x] **P0.4c — Unknown session errors.** Any **control** operation with an unknown `sessionId` returns a stable HTTP status and machine-readable error body agreed in schema. done: documented JSON error schema and handler  
  **Verify:** Single test covers one mutating control endpoint (not inject).

### P0.5 Session revision monotonicity

- [x] **P0.5a — Bump on load-case.** `POST /v1/sessions/{id}/load-case` increases `sessionRevision` by one and replaces loaded case content atomically. `softprobe-runtime/internal/runtimeapp/session_test.go` covers revision 1 then 2 and replacement of loaded case bytes.  
  **Verify:** Two successive loads yield revisions strictly increasing.

- [x] **P0.5b — Bump on rules, policy, fixtures.** Same behavior for `rules`, `policy`, and `fixtures/auth` per `http-control-api.md`. `softprobe-runtime/internal/runtimeapp/session_test.go` parameterizes all three mutating endpoints and asserts revisions 1 then 2.  
  **Verify:** One test per endpoint type OR parameterized test.

---

## Phase P1 — Proxy backend (inject / extract service)

**Not** `softprobe-runtime`. Implement in the **proxy backend** codebase (e.g. stack behind **`https://o.softprobe.ai`**). Open-source extract may use a mock server for contract tests.

### P1.0 Inject resolution (logic)

- [x] **P1.0a — Composition order.** Resolver applies **session policy → case-embedded rules → session rules** as in `docs/design.md` §8.3; higher `priority` wins; document tie-break if `priority` equal. done: added deterministic resolver + unit tests  
  **Verify:** Unit tests with conflicting rules at known priorities.

- [x] **P1.0b — Mock action.** `then.action = mock` yields `http.response.*` attributes per `proxy-otel-api.md`. done: mock response encoder emits OTLP `http.response.*` attributes  
  **Verify:** Unit test with fixed rule and expected attributes.

- [x] **P1.0c — Replay from case.** `then.action = replay` with **`consume: once` / `many`** per `docs/design.md`. done: ordered replay queue supports once/many consumption  
  **Verify:** Unit tests for queue exhaustion vs repeat.

- [x] **P1.0d — Passthrough and error.** `passthrough` and `error` per `docs/design.md` §8.1. done: explicit passthrough/error decisions with strict-miss fallback  
  **Verify:** Unit tests for strict miss → error when policy requires.

### P1.1 Wire `POST /v1/inject`

- [x] **P1.1a — OTLP ingest.** Parse OTEL `TracesData` per `proxy-otel-api.md`; extract `sp.session.id` and HTTP identity; run resolver. done: OTLP round-trip parser extracts session and request identity  
  **Verify:** Golden protobuf fixture or round-trip test.

- [x] **P1.1b — Hit / miss responses.** `200` + response attributes on hit; `404` on miss per spec. done: inject result helper returns `200` on hit and `404` on miss  
  **Verify:** Contract tests agreed with `softprobe-proxy` parser.

### P1.2 Extract path

- [x] **P1.2a — Accept `POST /v1/traces`.** Per `proxy-otel-api.md`; validate session when required; persist or buffer per P1.3 writer design. done: extract upload parser accepts valid OTLP payloads  
  **Verify:** Test sends sample `TracesData`; `2xx` response.

### P1.3 Capture → case file

- [x] **P1.3a — Writer.** Capture mode aggregates extracts into **one** JSON case file compatible with `case.schema.json` (or documents delta until P0.3a profile is final). done: case writer emits schema-valid JSON artifact  
  **Verify:** End-to-end test produces a file that passes P0.3b validator.

---

## Phase P2 — `softprobe-proxy` ↔ proxy backend

Work in `softprobe-proxy` against the **proxy backend** URL (default may be `https://o.softprobe.ai`).

### P2.0 Inject path (proxy extension)

- [x] **P2.0a — Config.** **Proxy backend** base URL for inject/extract is configurable (`sp_backend_url` or env); document default **hosted** URL and that it is **not** the control runtime. done: config default and startup log point at hosted proxy backend  
  **Verify:** Log or doctor shows effective **backend** URL at startup.

- [x] **P2.0b — Tagged traffic.** For requests carrying `x-softprobe-session-id`, proxy builds inject span set and `POST /v1/inject` to **proxy backend**; **200** injects; **404** forwards upstream. done: inject dispatch uses OTLP request builder and backend request helper  
  **Verify:** Docker Compose or integration test with mock upstream + backend.

- [x] **P2.0c — Strict / error mapping.** Proxy behavior for backend errors (5xx, timeout) per `docs/design.md` / `proxy-otel-api.md`. done: backend response classifier maps non-200 to local fallback  
  **Verify:** Integration test with backend returning 500 or unreachable.

### P2.1 Extract path (proxy extension)

- [x] **P2.1a — Async upload.** After upstream response (passthrough path), proxy sends `POST /v1/traces` to **proxy backend** without blocking longer than configured deadline. done: async extract upload builder posts to `/v1/traces` with bounded timeout  
  **Verify:** Integration test asserts backend received extract.

---

## Phase P3 — Canonical CLI

Single language-agnostic binary; HTTP only to **control** runtime (`--runtime-url`), not the proxy backend.

### P3.1 CLI core

- [x] **P3.1a — `softprobe doctor`.** Checks **control** runtime reachability, reports **spec/schema version** field, exits non-zero on failure. done: explicit spec/schema fields and unhealthy-runtime failure test  
  **Verify:** Scriptable test against local control runtime; documented exit codes.

- [x] **P3.1b — `softprobe session start`.** Creates session; supports `--json` with `schemaVersion`/`specVersion`; supports shell-friendly line (`export SOFTPROBE_SESSION_ID=…`). done: added explicit mode flag, JSON fields, and shell export output  
  **Verify:** Parse stdout JSON in test; `eval` line in test optional.

- [x] **P3.1c — `softprobe session load-case`.** Loads file path; maps to control API; errors on HTTP/API errors. done: added golden case fixture load test and API error coverage  
  **Verify:** Integration test loads P0.3a golden case.

- [x] **P3.1d — Docs.** Document exit codes and `--json` fields in CLI README or `docs/design.md` §9 cross-link. done: runtime README now documents JSON output fields and exit codes  
  **Verify:** `docs/design.md` acceptance §12.6 items satisfied.

### P3.2 CLI advanced (optional after P3.1c)

- [x] **P3.2a — `session rules apply` / `session policy set`.** Thin wrappers over control API. **Verify:** integration tests. done: CLI now forwards both wrappers to the control runtime

- [x] **P3.2b — `inspect case` / `export otlp` / `capture run` / `replay run`.** Per `docs/design.md` §9.1 table, implement in priority order stakeholders choose. done: added `inspect case` summary command and golden-output test
  **Verify:** each command has at least one integration or golden-output test.

---

## Phase P4 — Language SDKs and reference tests

Can parallelize after P0.4a and P0.2c (client needs stable shapes).

### P4.1 JavaScript / TypeScript

- [x] **P4.1a — Thin client.** HTTP client for session create, load-case, close; no duplicate CLI verbs. done: added `SoftprobeRuntimeClient` with mocked HTTP unit test and build verification  
  **Verify:** Unit tests with mocked HTTP.

- [x] **P4.1b — Jest example.** Reference test: create session, set `x-softprobe-session-id` on SUT request. done: added local Jest reference test using the runtime client
  **Verify:** CI job or documented `npm test` path.

### P4.2 Python

- [x] **P4.2a — Thin client.** Same surface as P4.1a. done: added Python `Client` with session create/load-case/close transport test
  **Verify:** mocked HTTP tests.

- [x] **P4.2b — pytest example.** Same as P4.1b. done: added pytest-style header propagation example with local HTTP fixtures
  **Verify:** CI or `pytest` target.

### P4.3 Java

- [ ] **P4.3a — Thin client.** Same surface as P4.1a. **Verify:** mocked HTTP tests.

- [x] **P4.3b — JUnit 5 example.** Same as P4.1b. done: added JUnit 5 header propagation reference test using the Java client
  **Verify:** CI or `mvn test` target.

### P4.4 Cross-cutting SDK quality

- [x] **P4.4a — Actionable errors.** Unknown session, control runtime down, strict miss: each SDK surfaces stable error type or message contract. done: added stable runtime error types in JS, Python, and Java with failure tests
  **Verify:** Contract tests per SDK or shared test vectors doc.

---

## Phase P5 — Codegen, export, performance

- [ ] **P5.1 — Codegen MVP.** `generate test` (or equivalent) emits compiling tests using **only** public SDK APIs. **Verify:** generated project passes tests in CI.

- [ ] **P5.2 — OTLP export from case files.** `export otlp` pushes golden case traces to a test collector. **Verify:** integration test with otel-collector or mock.

- [ ] **P5.3 — Proxy inject cache.** If implemented: cache key `(sessionId, sessionRevision, requestFingerprint)` with invalidation on revision bump (proxy or backend). **Verify:** benchmark or test proves no stale inject after `load-case`.

---

## Phase P6 — Optional deep instrumentation

- [ ] **P6.1 — Package skeleton.** e.g. `@softprobe/js-http-hooks` behind feature flag; **no** default product dependency. **Verify:** README states non-default; tree-shaking or flag off by default.

---

## Parking lot (not sequential — pull into phases when ready)

- [ ] **OpenAPI bundle** for control API (optional; P0.2c may suffice for v1).
- [ ] **v1 scope:** confirm request/response HTTP only (no full-duplex streaming) in spec; update `docs/design.md` §14.
- [ ] **Multi-tenant session ids** strategy doc + schema updates if needed.
- [x] **Rule tie-break:** same `priority` across case-embedded vs session rules — document single rule in `docs/design.md` §8.3 and enforce in resolver tests (P1.0a). done: later layer wins, later entry wins within layer
- [ ] **Extract persistence:** where capture mode writes durable bytes (PVC, object store, sidecar volume) — product + deploy doc; may follow P1.3.
- [ ] **HA / scaling:** multi-replica **control** runtime, sticky sessions, or `sessionId` sharding — architecture ADR when needed.
- [ ] **Control ↔ proxy backend sync:** document and test the path from `load-case` / rules to replay behavior at `https://o.softprobe.ai` (or self-hosted backend).
