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

- [x] **PE.3a — Strict policy blocks unmocked traffic.** With a replay session loaded from `captured.case.json` and policy `externalHttp: strict`: done: validated the end-to-end strict-miss path against the live compose stack
  1. Send a request to a **different path** (e.g. `GET /unknown`) through the proxy.
  2. Assert: proxy returns a `5xx` or configured error response (not forwarded to the app workload).
  3. Assert: app received `0` `/hello` hits.  
  **Verify:** Confirms the strict-miss → error path of P1.0d is exercised end-to-end.

---

## Phase PH — Hybrid convergence and core contract completion

Bring repo docs, runtime/CLI contracts, and the four SDKs back in line with the unified hybrid design before moving on to later codegen/export work.

### PH.0 Repo and docs convergence

- [x] **PH.0a — Canonical top-level docs.** Rewrite `README.md`, `docs/repo-layout.md`, `softprobe-runtime/README.md`, `softprobe-proxy/README.md`, and `softprobe-js/README.md` so they all describe the unified proxy-first hybrid product. Move older NDJSON/framework-patching and analytics-agent positioning into clearly-labeled legacy or migration sections instead of presenting them as the product. done: converged the five top-level docs on the unified runtime/proxy-first story and pushed older product language into explicit legacy notes  
  **Verify:** `rg "control API only|NDJSON|framework patch|business-level tracing|analytics" README.md docs/repo-layout.md softprobe-runtime/README.md softprobe-proxy/README.md softprobe-js/README.md` returns only intentional legacy/migration references.

- [x] **PH.0b — Docs-site scope honesty.** Update `docs-site/` pages that currently present not-yet-built OSS features as current GA behavior so they are marked preview/planned or trimmed to the shipped surface, without contradicting `docs/design.md` or `spec/`. done: rewrote the control API and CLI references around the current OSS surface, downgraded future/runtime-auth claims to planned, and removed suite-run overstatement from the main overview pages  
  **Verify:** `docs-site/reference/http-control-api.md` and `docs-site/reference/cli.md` stop claiming unimplemented OSS endpoints/commands as current behavior.

### PH.1 Runtime machine contract

- [x] **PH.1a — `GET /v1/meta`.** Add a machine-readable metadata endpoint on `softprobe-runtime` returning runtime version, `specVersion`, `schemaVersion`, and the minimal compatibility fields the CLI/SDKs need for drift detection. Write handler and contract tests first. done: added `/v1/meta` with runtime/spec/schema metadata and handler coverage  
  **Verify:** `go test ./...` in `softprobe-runtime/` covers success + method guard + field presence.

- [x] **PH.1b — Session stats endpoint + counters.** Add `GET /v1/sessions/{sessionId}/stats` backed by real inject/extract/strict-miss counters in the shared session store. Write failing tests first for at least one inject hit, one extract upload, and one strict miss. done: added session stats handler plus inject/extract/strict-miss counter wiring  
  **Verify:** runtime tests assert counter increments and unknown-session error shape.

- [x] **PH.1c — Runtime auth + stable errors.** Gate control and OTLP handlers behind optional bearer auth via `SOFTPROBE_API_TOKEN`, keep `/health` unauthenticated, and normalize control-plane JSON errors/status codes so SDKs and CLI can rely on them. Write auth/error tests first. done: added optional bearer auth middleware and stable JSON control error envelopes  
  **Verify:** tests cover missing token → `401`, wrong token → `403`, valid token → success, and unknown-session/malformed-body envelopes.

### PH.2 CLI contract completion

- [x] **PH.2a — `generate` dispatch + fragment fixture alignment.** Wire `generate` into `cmd/softprobe/main.go`, then align the fragment golden case/example expectations with the live e2e app response shape so `softprobe-js` generated-session and quickstart tests stop drifting. Write or tighten failing tests first. done: dispatched `generate`, switched the fragment golden case to replayable extract data, and realigned generated/README test expectations  
  **Verify:** `go test ./...` in `softprobe-runtime/` and `npm test -- --runInBand` in `softprobe-js/` are green for the generator and fragment replay examples.

- [x] **PH.2b — `session close`, `session stats`, and explicit `--shell`.** Add CLI subcommands for close/stats, keep shell export as an explicit `--shell` mode, and cover both human-readable and `--json` output with tests first. done: added CLI stats/close commands, explicit `--shell`, and nil-safe CLI writer handling  
  **Verify:** CLI tests parse JSON for `session start --json` / `session stats --json` / `session close --json`; `--shell` prints only the export line.

- [x] **PH.2c — Common JSON envelope + doctor drift detection.** Standardize shipped CLI `--json` commands on a top-level envelope (`status`, `exitCode`, command-specific fields) and expand `doctor` to compare runtime metadata from `/v1/meta` rather than only pinging `/health`. Write failing tests first for drift and unreachable runtime cases. done: added common CLI JSON envelopes and `doctor` health+meta drift detection with JSON failure output  
  **Verify:** CLI tests cover healthy runtime, spec/schema drift, unreachable runtime, and stable JSON field presence.

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

- [x] **P4.0b — TS SDK `Softprobe` / `SoftprobeSession`.** Implement **`startSession`**, **`attach`**, **`loadCaseFromFile`**, **`findInCase`**, **`mockOutbound`**, **`clearRules`**, **`close`** as the **only** HTTP callers to `/v1/sessions`, `/load-case`, `/rules`, `/close` per **`docs/design.md` §3.2** (no `fetch` in generated Jest modules). **`mockOutbound`** must **merge** then **replace** full rules document per store semantics (`ApplyRules`). Optional: **`setPolicy`**, **`setAuthFixtures`**. done: Softprobe/SoftprobeSession shipped with findInCase + mockOutbound; replayOutbound removed per P4.5 refactor  
  **Verify:** unit tests assert outbound `POST` bodies validate against `session-rules.request.schema.json` / `rule.schema.json`; two `mockOutbound` calls in a row preserve both rules; `clearRules` sends empty `rules`.

- [x] **P4.0c — Jest canonical quickstart.** `softprobe-js/README.md` (or `examples/jest-golden-path/`) documents **one** copy-paste flow: `doctor` → session → load-case/rules via SDK → Jest test with `x-softprobe-session-id`; links `docs/design.md` §5.3. done: updated the Jest replay quickstart with `doctor`, the §5.3 materialization link, and verified the documented `npm test` path against compose  
  **Verify:** documented `npm test` (or CI job) passes.

### P4.6 SDK parity and packaging truth

- [x] **P4.6a — TypeScript parity surface + typed errors.** Add the missing minimal SDK surface in `softprobe-js` (`loadCase`, `findAllInCase`, `setPolicy`, `setAuthFixtures`) plus stable typed errors for runtime unreachable, unknown session, case-load failure, and case-lookup ambiguity. Write failing unit tests first. done: added the missing session APIs plus typed runtime/case error classes and verified the full TS suite  
  **Verify:** `npm test -- --runInBand` covers each new method and error type.

- [x] **P4.6b — Python parity surface + typed errors.** Extend `softprobe-python` with the same minimal parity surface and stable typed errors on top of the thin client. Write failing tests first. done: added `load_case`, `find_all_in_case`, `set_policy`, `set_auth_fixtures`, plus `SoftprobeRuntimeUnreachableError`, `SoftprobeUnknownSessionError`, `SoftprobeCaseLoadError`, and `SoftprobeCaseLookupAmbiguityError` with unit coverage in `tests/test_softprobe.py`  
  **Verify:** Python unit tests cover new methods and error classes/messages.

- [x] **P4.6c — Java parity surface + typed errors.** Extend `softprobe-java` with the same minimal parity surface and stable typed errors. Write failing tests first. done: added `loadCase(String)`, `findAllInCase`, `setPolicy`, `setAuthFixtures`, and the `SoftprobeRuntimeUnreachableException` / `SoftprobeUnknownSessionException` / `SoftprobeCaseLoadException` / `SoftprobeCaseLookupAmbiguityException` hierarchy with JUnit coverage in `ParitySurfaceTest`  
  **Verify:** `mvn test -q` covers new methods and exception classes.

- [x] **P4.6d — Go parity surface + typed errors.** Align `softprobe-go` with the documented minimal parity surface and typed errors, or trim any remaining mismatched docs in the same task scope if the feature is intentionally absent. Write failing tests first. done: added `LoadCase([]byte)`, `FindAllInCase`, `SetAuthFixtures`, plus `UnreachableError`, `UnknownSessionError`, `CaseLoadError`, and `CaseLookupAmbiguityError`, with `errors.As` recoverability covered in `parity_surface_test.go`  
  **Verify:** `go test ./...` in `softprobe-go/` covers new methods and error recovery via `errors.As`.

- [x] **P4.6e — Package READMEs and publication truth.** Add or refresh package-level READMEs for Go, Python, and Java and correct any docs that imply public registry publication which this repo does not currently automate or release. done: added `softprobe-python/README.md`, `softprobe-java/README.md`, and `softprobe-go/README.md` with source-based usage; refreshed `softprobe-js/README.md` publish note; annotated `docs-site/installation.md`, `reference/sdk-python.md`, `reference/sdk-java.md`, `reference/sdk-go.md`, `guides/replay-in-pytest.md`, `guides/replay-in-junit.md`, and `guides/replay-in-go.md` with explicit "not yet published" warnings  
  **Verify:** each SDK repo has a README with source/local usage that matches the current release reality.

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

### P4.5 Replace `replayOutbound` with `findInCase` + `mockOutbound` (SDK-side case lookup)

**Normative design:** `docs/design.md` §3.2 — runtime only evaluates explicit `mock`/`error` rules; OTLP case walking and response materialization move into each SDK so test authors can mutate captured data before mocking.

- [x] **P4.5a — Design + schema update.** Remove `replay` action + `consume` field from `spec/schemas/rule.schema.json`; update `docs/design.md` §3.2 with `findInCase` + `mockOutbound` flow and the Division of Labour table; realign `spec/examples/rules/strict-block.rule.json`. done: design + rule schema updated, examples realigned  
  **Verify:** `spec/examples/**` validates against `spec/schemas/rule.schema.json`.

- [x] **P4.5b — TS SDK `findInCase`.** New pure in-memory lookup against the loaded case (`softprobe-js/src/core/case/find-span.ts`). Returns a mutable `CapturedHit { response, span }`; throws on zero/multi matches with span-ids in the message. done: find-span helper + unit tests (single/zero/multi, pathPrefix/host, pseudo-headers)  
  **Verify:** `softprobe-js` unit tests green.

- [x] **P4.5c — TS SDK drop `replayOutbound`.** Remove `replayOutbound` + `SoftprobeReplayRuleSpec`; simplify rule builder to `buildMockRule`; add `updateRules` on the runtime client. done: SDK surface now only has `findInCase` + `mockOutbound`  
  **Verify:** `softprobe-js` unit tests green; no references to `replayOutbound` remain in TS package.

- [x] **P4.5d — Jest e2e migration.** Port `e2e/jest-replay/fragment.replay.test.ts` to `findInCase` + `mockOutbound`. done: jest-replay passes through compose stack using SDK-side lookup  
  **Verify:** `cd e2e/jest-replay && npx jest` passes.

- [x] **P4.5e — Runtime: delete `replay` action.** Remove `replayResponseFromCase` and the replay branch in `softprobe-runtime/internal/proxybackend/inject.go`; runtime now only honors `mock`/`error`/`passthrough`/`capture_only`. Strict policy still returns error for injects with no matching explicit rule. done: inject handler rewritten around `selectInjectRule`; controlapi + inject tests updated  
  **Verify:** `go test ./...` in `softprobe-runtime/` green.

- [x] **P4.5f — Runtime session store audit (LoadedCase).** Verify `LoadedCase` is still only used for `case.rules[]` (rule extraction) and OTLP proxy export, not for inject materialization. done: no remaining case-trace walks on the inject hot path  
  **Verify:** `rg replayResponseFromCase softprobe-runtime/` returns no matches.

- [x] **P4.5g — Codegen update.** `softprobe generate jest-session` emits `session.findInCase(...)` + `session.mockOutbound(..., response: hit.response)` instead of `replayOutbound`. done: generator + golden + integration tests updated  
  **Verify:** `go test ./...` and generated module compiles/runs.

- [x] **P4.5h — Doc sweep.** Refresh `docs/design.md` and nearby docs to describe the new client-side lookup flow. done: §3.2 rewritten with concrete examples and Division of Labour table  
  **Verify:** no references to `replayOutbound` or `replay` action remain in `docs/design.md`.

#### Python SDK parity (new ergonomic layer)

- [x] **P4.5i — Python SDK: `Softprobe` + `SoftprobeSession` ergonomic classes.** Mirror the TS surface on top of the existing thin `Client`. Expose `start_session`, `attach`, `load_case_from_file`, `find_in_case`, `mock_outbound`, `clear_rules`, `close`. done: `softprobe/softprobe.py` + `softprobe/core/case_lookup.py`, 13 unit tests green  
  **Verify:** `python3 -m unittest discover -s tests` in `softprobe-python/` green.

- [x] **P4.5j — Python SDK: `find_in_case` + `mock_outbound`.** Case lookup mirrors the TS helper (traces → resourceSpans → scopeSpans → spans; pseudo-header fallbacks); `mock_outbound` builds schema-conformant rules via the thin `Client`. done: covered by P4.5i test suite  
  **Verify:** unit tests assert rule payloads and ambiguous/missing-match error messages.

- [x] **P4.5k — e2e/pytest-replay/: fragment happy path.** New harness `e2e/pytest-replay/` drives the same compose stack as `e2e/jest-replay/`. done: `test_fragment_replay.py` green against the existing stack  
  **Verify:** `python3 -m pytest e2e/pytest-replay/` green.

#### Java SDK parity (new ergonomic layer)

- [x] **P4.5l — Java SDK: `Softprobe` + `SoftprobeSession` ergonomic classes.** Mirror the TS surface on top of the existing thin `Client`. Add Jackson `databind` dependency for OTLP tree parsing; keep the regex parser in `Client` for flat control-plane responses. done: `Softprobe.java`, `SoftprobeSession.java`, `CaseLookup.java`, `CapturedResponse`, `CapturedHit`, `CaseSpanPredicate`, `MockRuleSpec`; 12 unit tests green  
  **Verify:** `mvn test` in `softprobe-java/` green.

- [x] **P4.5m — Java SDK: `findInCase` + `mockOutbound`.** Case lookup + rule serialization covered by `SoftprobeSessionTest` (single/zero/multi match, pseudo-headers, rule payload shape). done: covered by P4.5l test suite  
  **Verify:** rule payloads validate against `rule.schema.json`.

- [x] **P4.5n — e2e/junit-replay/: fragment happy path.** New Maven project `e2e/junit-replay/` runs the fragment replay through the SDK against the compose stack. done: `FragmentReplayTest` green  
  **Verify:** `mvn test` in `e2e/junit-replay/` green.

- [x] **P4.5o — Full e2e validation across three SDKs.** Jest, pytest, and JUnit harnesses all pass against the same compose stack. done: all three run green sequentially in the same session  
  **Verify:** `npx jest` + `pytest e2e/pytest-replay/` + `mvn -q test` (e2e/junit-replay) all green.

- [x] **P4.5p — softprobe-go SDK.** New module `softprobe-go/` with `Softprobe` facade, `SoftprobeSession` (`LoadCaseFromFile`, `FindInCase`, `MockOutbound`, `ClearRules`, `Close`), thin HTTP `Client`, and shared `CaseLookup` helpers (`FindSpans`, `ResponseFromSpan`, `FormatPredicate`, HTTP/2 pseudo-header fallback). 12 unit tests via an in-process `Transport` seam, mirroring softprobe-js / softprobe-python / softprobe-java. done: `go test ./...` green in `softprobe-go/`  
  **Verify:** `cd softprobe-go && go test ./...` green.

- [x] **P4.5q — e2e/go/go-replay/: fragment happy path.** Package under `e2e/go/go-replay/` drives the same `StartSession` → `LoadCaseFromFile` → `FindInCase` → `MockOutbound` → `GET APP_URL/hello` flow as the jest/pytest/junit harnesses (`softprobe-go` via `replace` in `e2e/go.mod`). done: `TestFragmentReplayThroughTheMesh` green against the compose stack  
  **Verify:** `cd e2e && go test -count=1 ./go/go-replay/...` green.

- [x] **P4.5r — Port TestReplayEgressInjectMocksUpstream to softprobe-go.** Rewrote the egress replay integration test in `e2e/replay_flow_test.go` around `softprobe-go`'s `Softprobe` / `SoftprobeSession` instead of raw HTTP POSTs; deleted the now-dead `TestReplayFlowUsesCapturedCase` which had been exercising the auto-ingress-replay path removed in P4.5e. Fixed shape-drift in the jest/pytest harnesses (which had been asserting a stale nested `dependency` body against an older app binary) so they match the live `{message, dep}` flat shape. done: full e2e suite (`TestCaptureFlowProducesValidCaseFile`, `TestReplayEgressInjectMocksUpstream`, `TestStrictPolicyBlocksUnmockedTraffic`) + all four SDK harnesses green  
  **Verify:** `cd e2e && go test -count=1 ./...` green and all four SDK harnesses (`jest`, `pytest`, `mvn`, `go test -count=1 ./go/go-replay/...`) green. (Go e2e code consolidated under `e2e/go/`: `go-capture/`, `go-replay/`, `e2etestutil/`, runtime integration tests.)

---

## Phase P5 — Codegen, export, performance

- [x] **P5.0 — `softprobe generate jest-session`.** CLI subcommand emits a TypeScript module per **`docs/design.md` §3.2** using **`@softprobe/sdk`** only (`Softprobe`, `SoftprobeSession`, `findInCase`, `mockOutbound`); **no emitted `fetch`**. Default output path documented. done: generator now emits findInCase + mockOutbound pairs, golden + integration tests green (P4.5g)  
  **Verify:** golden file diff test; generated file compiles; one e2e or integration test imports it.

- [ ] **P5.1 — Codegen MVP.** `generate test` (or equivalent) emits compiling tests using **only** public SDK APIs. **Verify:** generated project passes tests in CI.

- [ ] **P5.2 — OTLP export from case files.** `export otlp` pushes golden case traces to a test collector. **Verify:** integration test with otel-collector or mock.

- [ ] **P5.3 — Proxy inject cache.** If implemented: cache key `(sessionId, sessionRevision, requestFingerprint)` with invalidation on revision bump (proxy or backend). **Verify:** benchmark or test proves no stale inject after `load-case`.

---

## Phase P6 — Optional deep instrumentation

- [ ] **P6.1 — Package skeleton.** e.g. `@softprobe/js-http-hooks` behind feature flag; **no** default product dependency. **Verify:** README states non-default; tree-shaking or flag off by default.

---

## Phase DG — User-facing documentation site (`docs.softprobe.dev`)

Close gaps between `docs/design.md` + `spec/` + `tasks.md` and the public VitePress site under `docs-site/` (deployed to `docs.softprobe.dev`). All changes are **docs-only**; no code, contract, or schema changes. **Invariants:** every new page must be reachable from `docs-site/.vitepress/config.ts` sidebar, must pass `npm run docs:build`, and must uphold `docs/design.md` §15 invariants (no contradiction with design/spec; code snippets use only documented SDK APIs; links to `spec/` for normative shapes).

**Depends on:** `docs-site/` scaffolding committed (current `main`). No runtime code dependencies.

### DG1 — Complete normative reference pages

- [x] **DG1.1 — `docs-site/reference/proxy-otel-api.md`.** Mirror `spec/protocol/proxy-otel-api.md` for end users: `POST /v1/inject` and `POST /v1/traces` request/response shape, OTLP `TracesData` envelope, required span attributes, hit/`200` vs miss/`404` contract, extract-path semantics, error/timeout behavior. Link to the normative spec file at top of page. done: new user-facing reference page with worked OTLP request/response examples, SLO guidance, and session correlation explanation; added to sidebar.
  **Verify:** page is reachable from sidebar under "Reference"; links in `concepts/architecture.md` and `concepts/capture-and-replay.md` that currently describe inject/extract point at this page; `npm run docs:build` succeeds.

- [x] **DG1.2 — `docs-site/reference/rule-schema.md`.** Normative rule shape per `spec/schemas/rule.schema.json`: full enumeration of `when` matchers (`direction`, `service`, `host`, `hostSuffix`, `notHostSuffix`, `method`, `path`, `pathPrefix`, header predicates, body JSONPath predicates), `then` actions (`mock`, `error`, `passthrough`, `capture_only`) with payload shapes, `id`/`priority` semantics, `consume: once|many` **v1 caveat** ("may appear in documents; v1 inject does not dequeue from `traces[]`"). Show one YAML and one JSON example each. done: new normative reference covering all 5 actions (including `replay` as deprecated), SDK shorthand vs wire-schema distinction, replace-vs-merge semantics, validation workflow.
  **Verify:** sidebar entry under "Reference"; cross-links from `concepts/rules-and-policy.md` and `guides/mock-external-dependency.md` resolve; rule examples validate against `spec/schemas/rule.schema.json`.

- [x] **DG1.3 — Expand `docs-site/reference/case-schema.md` with OTLP attribute vocabulary.** Add a new "OTLP attribute vocabulary" section enumerating required and optional attributes from `spec/protocol/case-otlp-json.md`: `sp.session.id`, `sp.traffic.direction`, `url.full`, `http.request.method`, `http.request.header.*`, `http.request.body`, `http.response.status_code`, `http.response.header.*`, `http.response.body`, `service.name`, resource vs span placement. Include size-limit guidance. done: replaced old "Softprobe-specific attributes" with a structured vocabulary (identity / HTTP identity / payload / legacy aliases) plus v1 size guidance aligned with `case-otlp-json.md`.
  **Verify:** `rg "sp.session.id" docs-site/reference/case-schema.md` returns a match; `rg "http.response.body" docs-site/reference/case-schema.md` returns a match.

### DG2 — Surface the default codegen happy path

- [x] **DG2.1 — `docs-site/guides/generate-jest-session.md`.** Walk-through of `softprobe generate jest-session --case … --out …` per `docs/design.md` §3.2: prerequisites, generated file anatomy (imports `@softprobe/sdk` only, strings together `startSession` → `loadCaseFromFile` → `findInCase` + `mockOutbound`), regeneration workflow after capture refresh, sidecar YAML for policy/fixtures, diff-review tips. done: new how-to guide modeled on the actual generator output (`cmd/softprobe/generate_jest_session.go`), with test wrapper, Makefile/npm regen snippet, and diff-review tips; sidebar updated.
  **Verify:** generated module snippet in the guide matches the current golden output of `softprobe generate jest-session` (spot-check against `cmd/softprobe` golden file); sidebar entry under "How-to guides".

- [x] **DG2.2 — Quickstart Path A / Path B split.** Update `docs-site/quickstart.md` to present the **generator flow as "Path A (recommended)"** and the ad-hoc `findInCase` + `mockOutbound` flow as "Path B (when you need full control)". Keep both paths copy-paste complete. done: section 5 now offers a "Path A — Codegen (recommended)" subsection using `generate jest-session` + a `startReplaySession()` wrapper, and "Path B — Ad-hoc `findInCase` + `mockOutbound`" keeping the original copy-paste example.
  **Verify:** quickstart still runs end-to-end from either path against the e2e compose stack.

- [x] **DG2.3 — CLI `generate` subcommands.** In `docs-site/reference/cli.md`, split the `generate` section into per-framework subsections (`generate jest-session`, `generate test`). Document flags (`--case`, `--out`, `--framework`), output file location conventions, and interaction with sidecar YAML. done: `generate jest-session` now has flag table, output conventions, exit codes, and link to the codegen guide; `generate test` lists per-framework status (jest beta, vitest/pytest preview, junit alpha).
  **Verify:** `rg "generate jest-session" docs-site/reference/cli.md` returns at least one match; table of contents reflects new subsections.

### DG3 — Concepts polish (load-bearing invariants)

- [x] **DG3.1 — Author-time vs request-time callout.** In `docs-site/concepts/rules-and-policy.md` and `docs-site/concepts/capture-and-replay.md`, add a **callout block** (VitePress `::: tip` or `::: info`) stating the `docs/design.md` §8 preface invariant: **"The runtime never walks `traces[]` on the inject hot path."** Case-based lookup happens **only** in the SDK via `findInCase`; the runtime evaluates **explicit rules** only. done: `::: info Author-time vs request-time` callouts added to both concept pages with a link to design §5.3.
  **Verify:** `rg "never walks" docs-site/concepts/` returns matches in both files.

- [x] **DG3.2 — `capture_only` action in actions tables.** Add `capture_only` as a documented `then.action` value in: `docs-site/concepts/rules-and-policy.md`, `docs-site/guides/mock-external-dependency.md`, `docs-site/reference/rule-schema.md` (DG1.2). Explain: "matches the request for observability / extract purposes but still forwards to the real upstream". Note that `mockOutbound` does **not** emit `capture_only` rules — those are applied via raw `rules apply` or case-embedded rules. done: rule-schema.md and rules-and-policy.md already covered `capture_only`; added a dedicated "Observe-only: `capture_only` rules" section to `guides/mock-external-dependency.md` with YAML + CLI example.
  **Verify:** `rg "capture_only" docs-site/` returns matches in all three files.

- [x] **DG3.3 — Rule composition tie-break.** In `docs-site/concepts/rules-and-policy.md`, add explicit documentation of the tie-break rule from `docs/design.md` §8.3: **"when two rules share `priority`, the later composition layer wins (session rules > case-embedded rules > policy); within a single layer, later entries win."** Add a small worked example. done: precedence section now names the two tie-breakers explicitly and includes a case-rule-vs-session-rule worked example.
  **Verify:** `rg "later layer wins|later composition layer" docs-site/concepts/rules-and-policy.md` returns a match.

- [x] **DG3.4 — Proxy inject cache & `sessionRevision`.** Add a new subsection to `docs-site/concepts/architecture.md` documenting §4.4 / §8.4: proxy-side inject-decision caching is **optional**, and when enabled **must** be keyed on `(sessionId, sessionRevision, requestFingerprint)` and invalidated on every revision bump. Cross-link from `reference/http-control-api.md`. done: new "Proxy inject cache (optional, `sessionRevision`-keyed)" subsection in architecture.md with the four MUST requirements and a note that the OSS reference proxy does not cache.
  **Verify:** section heading appears in architecture.md ToC; cross-link from HTTP control API page resolves.

- [x] **DG3.5 — OpenTelemetry outbound propagation callout.** Reinforce the `docs/design.md` §3.3 integration risk — outbound calls from the app **must** propagate W3C `traceparent` / `tracestate` via OpenTelemetry. Add a diagram/callout in `docs-site/concepts/architecture.md` and a short troubleshooting entry in `docs-site/guides/troubleshooting.md` ("my egress mocks aren't hit" → check OTel propagation). done: added a "Trace context propagation (critical)" subsection with a warning callout in architecture.md and a "My egress mocks aren't hit" anchor target in troubleshooting with per-language fixes + debug steps.
  **Verify:** diagram or callout present in architecture.md; troubleshooting entry resolvable by `rg "OTel propagation|traceparent" docs-site/guides/troubleshooting.md`.

### DG4 — SDK surface completeness

- [x] **DG4.1 — Errors section in each SDK reference.** Add a unified "Errors" section to `docs-site/reference/sdk-typescript.md`, `sdk-python.md`, `sdk-java.md`, `sdk-go.md` enumerating stable error types / codes for: (a) runtime unreachable, (b) unknown session, (c) strict miss (as surfaced to the test), (d) invalid rule payload, (e) `findInCase` zero matches, (f) `findInCase` multiple matches. Include idiomatic catch examples per language. This maps to task **P4.4a**. done: uniform "Error catalog" table + idiomatic catch example + class-hierarchy table added to all four SDK references.
  **Verify:** `rg "## Errors" docs-site/reference/sdk-*.md` returns one match per SDK file.

- [x] **DG4.2 — `mockOutbound` merge-vs-replace semantics.** Add an explicit note in all four SDK references and in `docs-site/concepts/rules-and-policy.md`: **runtime `POST …/rules` replaces the entire rules document; SDKs merge on the client so consecutive `mockOutbound` calls accumulate.** Document the `clearRules()` escape hatch. done: `::: info` callouts next to each SDK's `mockOutbound` plus a new "SDKs merge, the runtime replaces" section in `concepts/rules-and-policy.md` with a channel-by-channel table.
  **Verify:** `rg "merge.*replace|replace.*merge|accumulate" docs-site/reference/sdk-*.md docs-site/concepts/rules-and-policy.md` returns matches in all five files.

- [x] **DG4.3 — `findInCase` throw behavior.** Uniformly document that `findInCase` **throws / returns an error** when zero spans or more than one span match, surfacing the matching span ids in the message. This is authoring-time validation, not a runtime miss. Add to all four SDK references. done: `::: warning` callouts next to each SDK's `findInCase` / `find_in_case` / `FindInCase` spelling out zero / multi-match behavior, with `.matches` / `getMatches()` / `Matches` field.
  **Verify:** `rg "zero|ambiguous|multi.*match" docs-site/reference/sdk-*.md` returns matches.

### DG5 — Missing how-to guides

- [x] **DG5.1 — `docs-site/guides/ship-rules-with-a-case.md`.** Explain `case.rules[]` and `case.fixtures[]` (case-embedded): when to ship rules with the case file vs apply them as session rules; precedence (per §8.3); how to author, validate (`spec/schemas/case.schema.json`), and diff. done: new guide covers embed-vs-apply decision matrix, authoring paths, precedence worked example, fixtures read-back, and a review checklist.
  **Verify:** sidebar entry; links from `concepts/rules-and-policy.md` and `reference/case-schema.md` resolve.

- [x] **DG5.2 — `docs-site/guides/auth-fixtures.md`.** Walk through `POST /v1/sessions/{id}/fixtures/auth` from `docs/design.md` §7.5: when HTTP-based auth is captured via case traces vs when to use fixtures (non-HTTP tokens, cookies, session material). Show the control-API shape and the SDK wrapper in each language. done: new guide with when-to-use decision, control-API payload, SDK wrappers in TS / Python / Java / Go, hook context usage, and CI example.
  **Verify:** sidebar entry; links from `reference/http-control-api.md` and SDK references resolve.

- [x] **DG5.3 — `docs-site/guides/debug-strict-miss.md`.** What the SUT sees when strict policy blocks an outbound (HTTP status, body, headers from `docs/design.md` §8.1), how to correlate with runtime logs, how to relax policy temporarily, how to add the missing rule. done: new guide documents the 599 + `x-softprobe-strict-miss: 1` contract, provides a symptom → diagnosis decision tree, and walks through the three fixes (mockOutbound / policy relaxation / passthrough). Troubleshooting page now cross-links it from the strict-miss entry.
  **Verify:** sidebar entry; troubleshooting page cross-links the new guide for the "strict miss" symptom.

### DG6 — Deployment & operations

- [x] **DG6.1 — `docs-site/deployment/envoy-standalone.md`.** Standalone Envoy + WASM YAML (no Istio), including the listener pair for ingress (8082) and egress (8084), `sp_backend_url` pointing at the runtime, WASM plugin config, health-check routing. Mirror the shape of `e2e/docker-compose.yaml` but without Docker Compose-isms. done: new deployment page modeled on `e2e/envoy.yaml` with two-listener config, `pluginConfig` reference table, iptables routing notes, and a smoke-test procedure. Sidebar updated.
  **Verify:** sidebar entry under "Deployment"; YAML validates with `envoy --mode validate` (optional) or at minimum `yamllint`.

- [x] **DG6.2 — HA staging in `docs-site/deployment/kubernetes.md`.** Add a section mapping the design's staged HA story: **v1 in-memory single-replica → add Redis/Postgres for session-state HA → multi-process split (control vs OTLP) when scaling demands it.** Include example Helm values or manifest deltas for each stage. done: section 6 rewritten as "Stage 1 in-memory single-replica → Stage 2 Redis-backed multi-replica → Stage 3 multi-process split" with manifest deltas and a sizing table.
  **Verify:** `rg "in-memory|single-replica|multi-process" docs-site/deployment/kubernetes.md` returns matches.

- [x] **DG6.3 — Flesh out `docs-site/deployment/hosted.md`.** Add subsections: **Regions** (available regions, latency, data residency), **Retention** (how long cases live, export options), **SLA** (uptime, support tiers, status page URL), **Rate limits** (cross-link from `reference/http-control-api.md`). done: Regions table now includes cloud-provider + data-residency columns; Rate limits section documents 429 + Retry-After + quota headers; SLA section includes exclusions, credits, status-page subscription.
  **Verify:** headings appear in page ToC; each subsection is at least a paragraph, not a placeholder.

### DG7 — CLI machine contract

- [x] **DG7.1 — `--json` fields table in `docs-site/reference/cli.md`.** For each command that supports `--json`, list the stable output fields (`sessionId`, `sessionRevision`, `specVersion`, `schemaVersion`, `caseId`, `status`, `error`, …). State the stability contract: breaking changes require a version bump visible via `softprobe doctor`. done: new "`--json` field stability" section with common envelope + per-command table + stability contract referencing `spec/schemas/cli-*.response.schema.json`.
  **Verify:** table with at least `doctor`, `session start`, `session load-case`, `inspect case` rows; `rg "specVersion" docs-site/reference/cli.md` returns matches.

- [x] **DG7.2 — Document `softprobe doctor` spec-drift detection.** Expand the `doctor` section in `docs-site/reference/cli.md` to describe: which version fields are compared, non-zero exit code on drift, JSON output on drift. Link to `docs/design.md` §9.2 indirectly via the normative spec. done: `doctor` now documents "What it checks" table, "Spec-drift detection" subsection comparing `cliVersion`/`runtimeVersion`/`specVersion`/`schemaVersion`, and sample `--json` output including drift mode.
  **Verify:** `doctor` section documents exit codes for drift and unreachable runtime.

### DG8 — Roadmap, versioning, changelog

- [x] **DG8.1 — `docs-site/roadmap.md`.** User-facing roadmap translating `tasks.md` phases into shipped / in-progress / planned sections. Hide internal granularity (e.g. "P1.0a"); surface user-visible milestones ("Go SDK", "Jest codegen", "hosted service GA"). done: new page with Shipped (v0.5), In progress (Redis multi-replica, hosted GA, codegen expansion, hook runtime, suite parallelism), Planned (multi-process split, Ruby/.NET, diff UI, cloud rules, OTel exporter), Non-goals, Contribute sections.
  **Verify:** sidebar entry; page renders clean tables per status; no mention of "P0.6b" style task ids.

- [x] **DG8.2 — `docs-site/versioning.md`.** Short page: current version (v0.5), versioning policy (semver for protocol + SDK major version alignment), what counts as a breaking change per `docs/design.md` §9.2 contract. done: new page covers current version, release cadence, four-surface compatibility matrix (SDK/CLI/spec/schema), per-surface breaking-change rules, SDK↔platform pairing, and deprecation policy.
  **Verify:** sidebar entry; links to changelog.

- [x] **DG8.3 — `docs-site/changelog.md` seed.** Seed with entries for v0.1–v0.6 sourced from `docs/design.md` §16 history. Add a short maintainer note at the top explaining entry format. done: new page seeded with v0.1 through v0.5 entries mapped from `design.md` §16, using Keep-a-Changelog style grouping (Added/Changed/Deprecated/…) and an Unreleased placeholder at the bottom.
  **Verify:** sidebar entry; entries for v0.5 and v0.6 present at minimum.

### DG9 — Glossary, QA, sidebar, build

- [x] **DG9.1 — `docs-site/glossary.md`.** Sourced from `docs/design.md` §4.1: Session, Case, Rule, Policy, Inject, Extract, Capture, Replay, Fixture, `sessionRevision`, `sp_backend_url`, `x-softprobe-session-id`. Cross-link each term to its primary concept page. done: new glossary page with 23 definitions, each cross-linking to its primary concept/reference page; anchors validated against the built site.
  **Verify:** sidebar entry (probably under "Reference" or as a top-level singleton).

- [x] **DG9.2 — Link-check sweep.** Add and run a link-checker (e.g. `lychee` or `markdown-link-check`) across `docs-site/`. Fix any dead internal anchors introduced by DG1–DG8. done: in-tree shell link-checker crawls every `](/...)` markdown link against built HTML. Fixed broken anchors: `#session-header-missing`, `#403-forbidden-…` (underscore prefix needed), `#post-v1sessionssessionidfixturesauth`, `#session-id-missing-from-egress-captures`, `#replay-deprecated`, `#mockoutbound`, and glossary author-time/SDK anchors. 257 internal links pass.
  **Verify:** link-check script passes locally; add the command to `docs-site/README.md` under a new "QA" section; optionally add a GitHub Actions workflow stub (can be parked).

- [x] **DG9.3 — Sidebar, nav, and build.** Update `docs-site/.vitepress/config.ts` to include every new page added in DG1–DG8 in the appropriate sidebar section. Run `npm run docs:build` clean; preview locally; run `docker compose up --wait` in `e2e/` and spot-check that documented snippets still work against the live stack. done: sidebar gained "About" section (Roadmap, Versioning, Changelog, Glossary, FAQ); top-nav version menu refreshed (Changelog, Roadmap, Versioning, GitHub releases). `npm run docs:build` exits 0 with no broken internal link anchors.
  **Verify:** `npm run docs:build` exits `0` with no warnings; all new pages reachable via sidebar click-through; e2e smoke test still green.

---

## Parking lot (not sequential — pull into phases when ready)

- [ ] **OpenAPI bundle** for control API (optional; P0.2c may suffice for v1).
- [ ] **v1 scope:** confirm request/response HTTP only (no full-duplex streaming) in spec; update `docs/design.md` §14.
- [ ] **Multi-tenant session ids** strategy doc + schema updates if needed.
- [x] **Rule tie-break:** same `priority` across case-embedded vs session rules — document single rule in `docs/design.md` §8.3 and enforce in resolver tests (P1.0a). done: later layer wins, later entry wins within layer
- [ ] **Extract persistence:** where capture mode writes durable bytes (PVC, object store, sidecar volume) — product + deploy doc; may follow P1.3.
- [ ] **HA / scaling:** multi-replica **control** runtime, sticky sessions, or `sessionId` sharding — architecture ADR when needed.
- [x] **Control ↔ proxy backend sync:** Resolved in v0.5 design update — `softprobe-runtime` serves both API surfaces from one process with a shared in-memory store. No external sync required for v1. Multi-process HA split is a future parking lot item when scale demands it.
