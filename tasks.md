# Tasks

> **Execution:** Work in **document order** (top to bottom). The **first** unchecked `[ ]` item is the active task unless a dependency line says otherwise.  
> **Process:** Follow `AGENTS.md` (TDD for code, no scope beyond `docs/design.md`).  
> **When done:** Change `[ ]` → `[x]` and append a short commit-style note on the same line.

### Architecture (read before coding)

- **`softprobe-runtime` (OSS, unified)** — Serves **both** the HTTP control API ([http-control-api.md](spec/protocol/http-control-api.md)) **and** the proxy OTLP API ([proxy-otel-api.md](spec/protocol/proxy-otel-api.md)) from **one process** with a **shared in-memory session store**. No external sync needed. **v1: no database required.** Add Redis/Postgres only for HA ([docs/platform-architecture.md](docs/platform-architecture.md) §10.2).  
- **Internal package layout:** `internal/store/` (shared session/case/rules state), `internal/controlapi/` (JSON control handlers), `internal/proxybackend/` (OTLP inject/extract handlers — `POST /v1/inject`, `POST /v1/traces`).  
- **Deployment:** `SOFTPROBE_RUNTIME_URL` (CLI/SDKs) and proxy WASM `sp_backend_url` both point to the **same** `softprobe-runtime` base URL. No second service needed locally.  
- **Canonical language:** **Go**. **Proxy** calls `softprobe-runtime` OTLP endpoints over HTTP; it does not link the runtime as a library.

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

## Phase P0.6 — Unified service: store extraction and OTLP route stubs

Refactor `softprobe-runtime` so that a **single in-memory store** is shared by both the control API handler group and the new OTLP handler group. This is the prerequisite for implementing real inject/extract resolution in P1.

**Depends on:** P0.5b complete (all control endpoints tested).

### P0.6 Store extraction and OTLP stubs

- [x] **P0.6a — Extract `Store` to `internal/store/`.** Move `Store`, `Session`, and all mutation methods from `internal/runtimeapp/session.go` to a new package `softprobe-runtime/internal/store/store.go`. Rename `internal/runtimeapp/` to `internal/controlapi/` (update all imports in `mux.go`, `main.go`, `cmd/softprobe/main.go`, and tests). No logic changes — all existing tests must stay green. done: extracted shared session state into `internal/store` and renamed handlers to `internal/controlapi`  
  **Verify:** `go test ./...` passes; `internal/store/` is the sole definition of `Store` and `Session`; no circular imports.

- [x] **P0.6b — OTLP route stubs in `mux.go`.** Register `POST /v1/inject` and `POST /v1/traces` stubs (returning `501 Not Implemented` with a JSON body `{"error":"not implemented"}`) that accept the shared `*store.Store`. Tests confirm both routes exist and return `501` before P1 fills them in. done: added OTLP stub routes and route tests  
  **Verify:** Integration test asserts both OTLP routes are registered and return `501`.

- [x] **P0.6c — Align proxy config: `sp_backend_url` default.** In `softprobe-proxy/config/development.yaml` (and any deploy manifests), document that `sp_backend_url` defaults to the same base URL as `SOFTPROBE_RUNTIME_URL` (e.g. `http://localhost:8080`). Update `softprobe-runtime/README.md` and `softprobe-proxy/docs/deployment.md` to show the single-URL local setup. done: aligned local docs/config to the unified runtime URL  
  **Verify:** README / deployment doc shows `sp_backend_url = http://localhost:<runtime-port>` matching the control runtime URL.

---

## Phase P1 — OTLP handlers in `softprobe-runtime`

Implement inject/extract OTLP endpoints inside **`softprobe-runtime`** (package `internal/proxybackend/`), backed by the **shared session store** from `internal/store/`. The proxy WASM calls `POST /v1/inject` and `POST /v1/traces` on this same service. The logic designed in P1.0–P1.3 (originally scoped to a conceptually separate proxy backend and prototyped in Rust inside `softprobe-proxy`) is now ported to Go inside the unified runtime.

**Depends on:** P0.6b (OTLP route stubs registered and store extracted).

### P1.0 Inject resolution (logic)

> **Note:** P1.0–P1.3 were prototyped in Rust inside `softprobe-proxy`. These tasks must be re-implemented in **Go** inside `softprobe-runtime/internal/proxybackend/` backed by `internal/store/`. The Rust work is the reference; a task is not truly `[x]` until the Go unit tests pass in the unified runtime.

- [x] **P1.0a — Composition order.** Port resolver to Go: applies **session policy → case-embedded rules → session rules** per `docs/design.md` §8.3; higher `priority` wins; later layer wins on tie. Rust reference: `softprobe-proxy/src/resolver.rs`. done: added `internal/proxybackend` resolver and precedence tests  
  **Verify:** Go unit tests in `internal/proxybackend/resolver_test.go` cover conflicting priorities — mirror cases from `resolver_composition_test.rs`.

- [x] **P1.0b — Mock action.** `then.action = mock` yields `http.response.*` attributes per `proxy-otel-api.md`. Rust reference: `softprobe-proxy/src/injection.rs`. done: added mock response attribute encoder and test  
  **Verify:** Go unit test with fixed rule and expected attributes.

- [x] **P1.0c — Replay from case.** `then.action = replay` with `consume: once` / `many`. Rust reference: `softprobe-proxy/src/inject_ingest.rs`. done: added replay queue semantics and tests  
  **Verify:** Go unit tests for queue exhaustion vs repeat.

- [x] **P1.0d — Passthrough and error.** `passthrough` and `error` per `docs/design.md` §8.1; strict-miss fallback to `error` when policy requires. done: added explicit decision helpers and tests  
  **Verify:** Go unit tests for strict miss → error.

### P1.1 Wire `POST /v1/inject`

- [x] **P1.1a — OTLP JSON ingest.** Replace the `501` stub from P0.6b: parse OTLP JSON `TracesData` per `proxy-otel-api.md`; extract `sp.session.id` and HTTP identity from span attributes; look up session in shared store; run resolver. done: added JSON inject parser and wired `/v1/inject` to the shared store  
  **Verify:** Go integration test: create session via control API → call `/v1/inject` with golden JSON payload → resolver runs.

- [x] **P1.1b — Hit / miss responses.** `200` + OTLP response attributes on hit; `404` on miss per spec. done: inject handler now returns OTLP JSON hit responses and 404 misses  
  **Verify:** Go contract test asserts both branches; shapes match what `softprobe-proxy` parser expects.

### P1.2 Extract path

- [x] **P1.2a — Accept `POST /v1/traces`.** Replace the `501` stub: parse OTLP `TracesData` JSON; look up session in shared store; buffer spans per capture mode. Return `2xx`. done: added trace upload handler and capture buffering in the shared store  
  **Verify:** Go integration test sends sample `TracesData`; `2xx`; session store updated.

### P1.3 Capture → case file

- [x] **P1.3a — Writer.** In capture mode, aggregate buffered extract spans into **one** JSON case file per session compatible with `case.schema.json`. Trigger on session close or explicit flush. Rust reference: `softprobe-proxy/src/case_writer.rs`. done: close now flushes buffered capture payloads into `e2e/captured.case.json`  
  **Verify:** Go integration test: create capture session → POST several `/v1/traces` payloads → close session → written file passes `spec/scripts/validate-spec.sh`.

---

## Phase P2 — `softprobe-proxy` ↔ `softprobe-runtime` OTLP integration

Work in `softprobe-proxy`. The proxy WASM calls `softprobe-runtime` OTLP endpoints; `sp_backend_url` defaults to the same URL as `SOFTPROBE_RUNTIME_URL` in local/OSS setups.

### P2.0 Inject path (proxy extension)

- [x] **P2.0a — Config.** OTLP backend URL for inject/extract is configurable (`sp_backend_url` or env); for local/OSS deployment this defaults to the **same URL as `SOFTPROBE_RUNTIME_URL`** (unified service). done: config default and startup log point at proxy backend URL  
  **Verify:** Log or doctor shows effective backend URL at startup; `config/development.yaml` and deployment docs show `sp_backend_url` = `SOFTPROBE_RUNTIME_URL` for local setup.

- [x] **P2.0b — Tagged traffic.** For requests carrying `x-softprobe-session-id`, proxy builds inject span set and `POST /v1/inject` to **`softprobe-runtime`** (`sp_backend_url`); **200** injects; **404** forwards upstream. done: inject dispatch uses OTLP request builder and backend request helper  
  **Verify:** Docker Compose or integration test with mock upstream + backend.

- [x] **P2.0c — Strict / error mapping.** Proxy behavior for backend errors (5xx, timeout) per `docs/design.md` / `proxy-otel-api.md`. done: backend response classifier maps non-200 to local fallback  
  **Verify:** Integration test with backend returning 500 or unreachable.

### P2.1 Extract path (proxy extension)

- [x] **P2.1a — Async upload.** After upstream response (passthrough path), proxy sends `POST /v1/traces` to **`softprobe-runtime`** (`sp_backend_url`) without blocking longer than configured deadline. done: async extract upload builder posts to `/v1/traces` with bounded timeout  
  **Verify:** Integration test asserts backend received extract.

---

## Phase PE — End-to-end golden path acceptance test

Validates the **complete capture → replay loop** using real components: `softprobe-runtime` (Go, unified), `softprobe-proxy` (Envoy+WASM), a minimal **app workload** stand-in, and Go tests driving the control API. This is the acceptance gate for the entire P0.6 + P1 + P2 work.

**Depends on:** P0.6c + P1.3a (case writer done) + P1.1b (inject hit/miss working) + P2.0b and P2.1a (proxy wired to unified runtime).

**Test harness:** `e2e/docker-compose.yaml` with five services:

| Service | Role |
|---------|------|
| `softprobe-runtime` | Unified Go service (control API + OTLP handler) |
| `softprobe-proxy` | Envoy + WASM; `sp_backend_url` = runtime URL |
| `app` | Tiny **SUT** (`e2e/app/main.go`): ingress via proxy **8082**, egress to dependency via proxy **8084** |
| `upstream` | Tiny **dependency** (`e2e/upstream/main.go`); reached only **app → proxy → upstream** |
| `test-runner` | Smoke client: health checks and one `GET` **through ingress** (not the SUT) |

**Product topology:** `client → proxy → app → proxy → upstream`. The test-runner is the **client**; Envoy intercepts **ingress and egress**; see [e2e/README.md](e2e/README.md) and [docs/design.md](docs/design.md) §3.4.

### PE.1 Capture flow

- [x] **PE.1a — Docker Compose harness.** Add `e2e/docker-compose.yaml` with the five services above. `app` (`e2e/app/main.go`) serves `/hello` using a dependency reached **via proxy egress**; `upstream` (`e2e/upstream/main.go`) is that dependency. Verify all services start and pass health checks. done: added compose harness, proxy/app/upstream/test-runner, and validated `docker compose up --wait`  
  **Verify:** `docker compose -f e2e/docker-compose.yaml up --wait` exits `0`; runtime `/health` reachable.

- [x] **PE.1b — Capture session test.** Script or Go test in `e2e/`:
  1. `softprobe session start --mode capture` → capture `SESSION_ID`.
  2. `curl` (or Go `http.Client`) sends `GET http://<proxy>:8082/hello` (forwarded to **app**) with header `x-softprobe-session-id: $SESSION_ID`.
  3. App responds; proxy sends `POST /v1/traces` to runtime.
  4. `softprobe session close --session $SESSION_ID` → runtime writes `e2e/captured.case.json`.
  5. Assert: `e2e/captured.case.json` exists and passes `spec/scripts/validate-spec.sh`.
  6. Assert: case file contains exactly one trace with `url.full` matching the exercised path and `http.response.status_code = 200`.  
  **Verify:** Script exits `0`; case file is schema-valid. done: added `e2e/capture_flow_test.go` and validated against the compose stack

### PE.2 Replay flow

- [x] **PE.2a — Replay session test.** Continues from PE.1b (uses `captured.case.json`): done: replay now matches captured extract spans and keeps app workload at zero
  1. Reset app `/hello` hit counter to `0` (or use a fresh `app` container).
  2. `softprobe session start --mode replay` → capture `REPLAY_SESSION_ID`.
  3. `softprobe session load-case --session $REPLAY_SESSION_ID --file e2e/captured.case.json`.
  4. `curl` sends `GET http://<proxy>:8082/hello` with `x-softprobe-session-id: $REPLAY_SESSION_ID`.
  5. Assert: response status = `200` and body = `{"message":"hello"}` — matching the captured case.
  6. Assert: **app** received **0 new** `/hello` hits (inject was used, not passthrough to live workload).
  7. Assert: a second identical request (same session) also returns the captured response (`consume: many` semantics).  
  **Verify:** Script exits `0`; app call count remains `0`.

### PE.3 Strict miss

- [ ] **PE.3a — Strict policy blocks unmocked traffic.** With a replay session loaded from `captured.case.json` and policy `externalHttp: strict`:
  1. Send a request to a **different path** (e.g. `GET /unknown`) through the proxy.
  2. Assert: proxy returns a `5xx` or configured error response (not forwarded to the app workload).
  3. Assert: app received `0` `/hello` hits.  
  **Verify:** Confirms the strict-miss → error path of P1.0d is exercised end-to-end.

---

## Phase P3 — Canonical CLI

Single language-agnostic binary; HTTP only to **`softprobe-runtime`** (`--runtime-url`) for JSON control API operations.

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

### P4.0 TypeScript + Jest first (materialization path)

**Normative design:** `docs/design.md` §5.3 (inject resolved in/near data plane; control API materialization). **First-stage SDK:** `softprobe-js` + Jest before expanding ergonomic APIs to Python/Java.

- [x] **P4.0a — Design + repo-layout alignment.** Document inject placement, materialization model, TS+Jest first tier (`docs/design.md` §5.3, §7.0, §8 intro); update `docs/repo-layout.md` §2 `softprobe-js` bullets. done: design §5.3/§7.0/§8 + repo-layout softprobe-js first-stage note  
  **Verify:** Links in `docs/design.md` / `docs/repo-layout.md` resolve.

- [ ] **P4.0b — TS SDK `Softprobe` / `SoftprobeSession`.** Implement **`startSession`**, **`attach`**, **`loadCaseFromFile`**, **`mockOutbound`**, **`replayOutbound`**, **`clearRules`**, **`close`** as the **only** HTTP callers to `/v1/sessions`, `/load-case`, `/rules`, `/close` per **`docs/design.md` §3.2** (no `fetch` in generated Jest modules). **`mockOutbound` / `replayOutbound`** must **merge** then **replace** full rules document per store semantics (`ApplyRules`). Optional: **`setPolicy`**, **`setAuthFixtures`**.  
  **Verify:** unit tests assert outbound `POST` bodies validate against `session-rules.request.schema.json` / `rule.schema.json`; two `mockOutbound` calls in a row preserve both rules; `clearRules` sends empty `rules`.

- [ ] **P4.0c — Jest canonical quickstart.** `softprobe-js/README.md` (or `examples/jest-golden-path/`) documents **one** copy-paste flow: `doctor` → session → load-case/rules via SDK → Jest test with `x-softprobe-session-id`; links `docs/design.md` §5.3.  
  **Verify:** documented `npm test` (or CI job) passes.

### P4.1 JavaScript / TypeScript

- [x] **P4.1a — Thin client.** HTTP client for session create, load-case, close; no duplicate CLI verbs. done: added `SoftprobeRuntimeClient` with mocked HTTP unit test and build verification  
  **Verify:** Unit tests with mocked HTTP.

- [x] **P4.1b — Jest example.** Reference test: create session, set `x-softprobe-session-id` on SUT request. done: added local Jest reference test using the runtime client  
  **Verify:** CI job or documented `npm test` path.

### P4.2 Python

**Follows** P4.0 ergonomic patterns where applicable (same control API contract).

- [x] **P4.2a — Thin client.** Same surface as P4.1a. done: added Python `Client` with session create/load-case/close transport test  
  **Verify:** mocked HTTP tests.

- [x] **P4.2b — pytest example.** Same as P4.1b. done: added pytest-style header propagation example with local HTTP fixtures  
  **Verify:** CI or `pytest` target.

### P4.3 Java

**Follows** P4.0 ergonomic patterns where applicable (same control API contract).

- [x] **P4.3a — Thin client.** Same surface as P4.1a. **Verify:** mocked HTTP tests. done: Java control client already exposes create/load-case/close with mocked HTTP tests

- [x] **P4.3b — JUnit 5 example.** Same as P4.1b. done: added JUnit 5 header propagation reference test using the Java client  
  **Verify:** CI or `mvn test` target.

### P4.4 Cross-cutting SDK quality

- [x] **P4.4a — Actionable errors.** Unknown session, control runtime down, strict miss: each SDK surfaces stable error type or message contract. done: added stable runtime error types in JS, Python, and Java with failure tests
  **Verify:** Contract tests per SDK or shared test vectors doc.

---

## Phase P5 — Codegen, export, performance

- [ ] **P5.0 — `softprobe generate jest-session`.** CLI subcommand emits a TypeScript module per **`docs/design.md` §3.2** using **`@softprobe/sdk`** only (`Softprobe`, `SoftprobeSession`, **`mockOutbound`** / `replayOutbound`); **no emitted `fetch`**. Default output path documented. **Verify:** golden file diff test; generated file compiles; one e2e or integration test imports it.

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
- [x] **Control ↔ proxy backend sync:** Resolved in v0.5 design update — `softprobe-runtime` serves both API surfaces from one process with a shared in-memory store. No external sync required for v1. Multi-process HA split is a future parking lot item when scale demands it.
