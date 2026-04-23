# Tasks

> **Execution:** Work in **document order** (top to bottom). The **first** unchecked `[ ]` item is the active task unless a `Depends on:` line says otherwise.
> **Process:** Follow `AGENTS.md` (TDD for code, no scope beyond design docs).
> **When done:** Change `[ ]` → `[x]` and append a short commit-style note on the same line.

## Architecture (load-bearing context — read before coding)

- **`softprobe-runtime` (OSS, unified)** — serves **both** the HTTP control API ([`spec/protocol/http-control-api.md`](spec/protocol/http-control-api.md)) **and** the proxy OTLP API ([`spec/protocol/proxy-otel-api.md`](spec/protocol/proxy-otel-api.md)) from **one Go process** with a shared session store. OSS uses in-memory; hosted uses Redis.
- **Internal package layout:** `internal/store/` (shared session/case/rules state), `internal/controlapi/` (JSON control handlers), `internal/proxybackend/` (OTLP inject/extract handlers).
- **Hosted service design:** [`docs/hosted-service.md`](docs/hosted-service.md). Infrastructure credentials: `.env.hosted`, `.gcp-softprobe-runtime.json` (gitignored).
- **Hosted auto-detection:** Hosted mode activates automatically when `SOFTPROBE_AUTH_URL`, `REDIS_HOST`, and `GCS_BUCKET` are all set. No explicit feature flag needed.

## Legend

| Mark | Meaning |
|------|---------|
| `[ ]` | Not started |
| `[~]` | In progress |
| `[x]` | Done |

---

## Phase HD — Hosted Service (highest priority)

Design doc: [`docs/hosted-service.md`](docs/hosted-service.md)

### HD1 — Auth middleware

- [x] **HD1.1 — Auth service client.** Add `internal/authn/` package. `Resolve(apiKey) → (tenantID, bucketName, error)` calls `auth.softprobe.ai/api/api-key/validate` with `Authorization: Bearer <key>`. Cache responses in-process with 60 s TTL. Failing tests use a fake auth server.

- [x] **HD1.2 — Bearer middleware.** When hosted mode is active (SOFTPROBE_AUTH_URL + REDIS_HOST + GCS_BUCKET set), all `/v1/*` routes require `Authorization: Bearer <api-key>`. Middleware calls `authn.Resolve`, injects `tenantID` and `bucketName` into the request context. Returns `401` on missing header, `403` on invalid key. Tests cover missing, invalid, and valid key paths.

### HD2 — Redis session store

- [x] **HD2.1 — `Store` interface.** Extract the current concrete `store.Store` struct into a `Store` interface in `internal/store/`. `MemoryStore` is the existing implementation renamed. All callers updated to use the interface. No behavior change; tests stay green.

- [x] **HD2.2 — `RedisStore` implementation.** Implement `Store` interface backed by Redis. Session document serialized as JSON under key `session:{tenantID}:{sessionID}`. Extract GCS paths stored in a Redis list `session:{tenantID}:{sessionID}:extracts`. Both keys TTL 24 h, reset on every write. Atomic revision increment via `WATCH`/`MULTI`/`EXEC`. Activated when `SOFTPROBE_AUTH_URL`, `REDIS_HOST`, and `GCS_BUCKET` are all set; `REDIS_PORT`, `REDIS_PASSWORD` are optional. Failing tests run against `miniredis`.

- [x] **HD2.3 — Tenant-scoped session access.** Session create, read, and write operations assert that the `tenantID` in the request context matches the session's stored `tenantID`. Cross-tenant reads return `404`. Tests cover the isolation invariant.

### HD3 — GCS case and extract storage

- [x] **HD3.1 — GCS client.** Add `internal/gcs/` package wrapping `cloud.google.com/go/storage`. `Put(ctx, bucket, object, data)` and `Get(ctx, bucket, object) → []byte`. Credentials from `GOOGLE_APPLICATION_CREDENTIALS` or workload identity. Activated when hosted mode is detected. Tests use a fake GCS server (`fsouza/fake-gcs-server` or httptest stub).

- [x] **HD3.2 — Extract writes to GCS.** In the hosted path, `POST /v1/traces` (extract) writes the OTLP payload to `gs://{tenantBucket}/extracts/{sessionID}/{uuid}.otlp.json` and appends the object path to the Redis extracts list rather than buffering in memory. Tests verify the GCS write and list append; OSS path is unchanged.

- [x] **HD3.3 — Case file on close.** In the hosted capture `close` path, read all paths from the Redis extracts list, fetch each object from GCS, merge into a case JSON document (existing `WriteCapturedCaseTo` logic), write to `gs://{tenantBucket}/cases/{sessionID}.case.json`, set `loadedCaseRef` on the session document. Tests verify the merge and GCS write.

- [x] **HD3.4 — `load-case` stores to GCS.** `POST /v1/sessions/{id}/load-case` in the hosted path writes the received case body to `gs://{tenantBucket}/cases/{sessionID}.case.json` and stores the GCS URI as `loadedCaseRef`. The in-memory `LoadedCase` bytes path still works for OSS. Tests cover both paths.

### HD4 — Hosted-only endpoints

- [x] **HD4.1 — `GET /v1/cases/{caseId}`.** Returns the case JSON by reading `loadedCaseRef` from the session document and fetching from GCS. `404` if no case loaded. Only registered in hosted mode. Tests cover hit, miss, and auth failure.

- [x] **HD4.2 — `GET /v1/sessions`.** Returns a JSON array of open sessions for the authenticated tenant (reads from Redis by tenant key prefix). Paginated with `?limit=` and `?cursor=`. Only registered in hosted mode. Tests cover empty, populated, and cross-tenant isolation.

### HD5 — Cloud Run deployment

- [x] **HD5.1 — VPC connector + Cloud Run config.** Document (in `docs/hosted-service.md` §7) the `gcloud` commands to create a Serverless VPC Access connector in `us-central1` pointing at the default VPC, and to deploy the runtime Cloud Run service with all env vars from `.env.hosted`. Include the `--vpc-connector` flag so the service can reach Redis at `10.42.202.91:6379`.

- [x] **HD5.2 — Hosted smoke test.** Add `e2e/hosted/smoke_test.go` that runs against `SOFTPROBE_RUNTIME_URL` (skips if unset). Covers: auth rejection (no key), session create → load-case → inject hit → close → `GET /v1/cases/{id}`. Used for post-deploy verification.

---

## Phase PD1 — CLI completeness (after HD)

### PD1.1 — Stability foundations

- [x] **PD1.1a — Stable exit codes.** Map documented codes in `cmd/softprobe`: `2` invalid args, `3` runtime unreachable, `4` session not found, `5` schema/validation error, `10` doctor fail, `20` suite fail. Failing tests first.

- [x] **PD1.1c — Universal `--json`.** Add the common `status/exitCode/error?` envelope to every mutating subcommand: `session load-case`, `session rules apply`, `session policy set`, `inspect case`, `generate jest-session`.

### PD1.3 — Session subcommands

- [x] **PD1.3a — `session start --policy FILE --case FILE`.** Chain session-create → apply-policy → load-case atomically.
- [x] **PD1.3b — `session policy set --file PATH`.** Accept a policy file alongside the existing `--strict` shortcut.
- [x] **PD1.3c — `session close --out PATH`.** For capture sessions, override the capture file output path.
- [x] **PD1.3d — `inspect session`.** Read-only: dump policy, rules, loaded-case summary, stats for a live session. Human + `--json`.

### PD1.4 — Validate subcommands

- [x] **PD1.4a — `validate case FILE`.** Schema-validate against `spec/schemas/case.schema.json`. Exit `5` on invalid.
- [x] **PD1.4b — `validate rules FILE`.** Same for `rule.schema.json`.
- [x] **PD1.4c — `validate suite FILE`.** Same for `suite.schema.json`.

### PD1.5 — Capture subcommands

- [x] **PD1.5a — `capture run --driver CMD --out PATH`.** Orchestrate: start capture session → export `SOFTPROBE_SESSION_ID` → run driver → close session → write case.
- [x] **PD1.5b — `capture run --timeout DURATION`.** Enforce wall-clock timeout on driver.
- [x] **PD1.5c — `capture run --redact-file PATH`.** Apply redaction rules during capture.

### PD1.6 — Replay subcommands

- [x] **PD1.6a — `replay run --session ID`.** Report inject hit/miss stats for a live session (wraps `session stats`).

### PD1.8 — Remaining subcommands

- [x] **PD1.8b — `export otlp --case … --endpoint …`.** Stream case traces to an OTLP HTTP endpoint.
- [x] **PD1.8c — `scrub FILE [--rules PATH]`.** Apply redaction rules to a case file in place.
- [x] **PD1.8d — `completion {bash,zsh,fish}`.** Emit shell-completion scripts.

---

## Phase PD2 — Auth e2e

- [x] **PD2.1f — e2e auth path.** Compose override sets `SOFTPROBE_API_TOKEN=sp_test` on the runtime; `e2e/go`, `jest-replay`, `pytest-replay`, `junit-replay` all run green picking up the token from env.

---

## Phase PD4 — Runtime ops

- [x] **PD4.1a — Prometheus `/metrics` endpoint.** Emit `softprobe_sessions_total{mode}`, `softprobe_inject_requests_total{result}`, `softprobe_inject_latency_seconds`, `softprobe_extract_spans_total`. Failing tests scrape and parse exposition format.
- [x] **PD4.2a — `SOFTPROBE_LOG_LEVEL`.** Wire into the runtime logger; values `debug|info|warn|error`.
- [x] **PD4.3a — `{sessionId}` template in `SOFTPROBE_CAPTURE_CASE_PATH`.** Interpolate `{sessionId}`, `{ts}`, `{mode}`. Plain path still works.
- [x] **PD4.4a — Object-storage case writers.** Add `file://` (default), `s3://`, `gs://`, `azblob://` schemes to `internal/proxybackend/case_writer.go`. (Note: `gs://` is covered by HD3 for the hosted path; this task wires it into the OSS `SOFTPROBE_CAPTURE_CASE_PATH` config.)

---

## Phase PD5 — Release and packaging

- [x] **PD5.4a — softprobe-js npm publish workflow.** Tag-triggered CI: `npm publish`. Align `package.json#version` with release tag.
- [x] **PD5.4b — softprobe-python PyPI publish workflow.** Tag-triggered; TestPyPI first, then PyPI.
- [x] **PD5.4c — softprobe-java Maven Central publish workflow.** Tag-triggered; signs + publishes to OSSRH; auto-promote.
- [x] **PD5.4d — softprobe-go module path.** Rename module to `github.com/softprobe/softprobe-go`, update `replace` directives, tag `v0.5.0`.

---

## Phase PD6 — Docs cleanup

- [x] **PD6.5e — Config and docs cutover.** Remove `SOFTPROBE_CONFIG_PATH` + cassette-directory workflow from primary docs/examples; replace with runtime-backed defaults in `softprobe-js/README.md` and docs-site TS pages.

---

## Phase PD7 — Dogfooding (after HD + PD1)

- [x] **PD7.1a — `DOGFOOD_REF` policy.** Add `spec/dogfood/REFERENCE.md` defining the reference build (initially `main@<sha>`, post-PD5.3a → released tags). Document the invariant: case refresh must land in a PR with no runtime or SDK code changes.
- [x] **PD7.1b — Deterministic capture driver.** `cmd/softprobe-dogfood-capture/` + `spec/dogfood/capture.sh`: start e2e compose, run canonical CLI flow, canonicalize session/trace IDs to stable placeholders, write `spec/examples/cases/control-plane-v1.case.json`.
- [x] **PD7.1c — `make capture-refresh` target.** Runs driver, prints diff. Refuses if working tree has uncommitted runtime or SDK changes.
- [x] **PD7.2a — `dogfood_replay_test.go`.** In-process runtime harness; load `control-plane-v1.case.json`; run each CLI subcommand through the canonical flow; assert every outbound request matched a recorded rule.
- [x] **PD7.2b — Failure taxonomy.** Replay errors distinguish code regression, case staleness, transport failure; map to documented exit codes.
- [x] **PD7.3a — TS SDK parity test.** `parity-dogfood.test.ts` drives full facade against `control-plane-v1.case.json` via fake runtime.
- [x] **PD7.3b — Python SDK parity test.** `test_parity_dogfood.py` — same semantics.
- [x] **PD7.3c — Java SDK parity test.** `ParityDogfoodTest.java` — same semantics.
- [x] **PD7.3d — Go SDK parity test.** `parity_dogfood_test.go` — same semantics.
- [x] **PD7.4a — Nightly refresh job.** `.github/workflows/dogfood-refresh.yml`: scheduled + manual dispatch; opens PR if `make capture-refresh` produces a diff. Never auto-merges.
- [x] **PD7.4b — Refresh playbook.** `docs-site/guides/contribute-dogfood.md`: when dogfood test fails, run `make capture-refresh`, inspect diff, fix regression or land refresh PR.
- [x] **PD7.5a — `docs/snippets.suite.yaml`.** Extract copy-paste CLI flows from guides into a suite; CI fails if a snippet no longer matches real CLI behavior.

---

## Backlog (no active milestone)

- [ ] OpenAPI bundle for the control API.
- [ ] Ruby and .NET SDKs.
- [ ] Case diffing in the browser (web UI).
- [ ] Cloud-managed rules — version-controlled, shareable rule bundles in the hosted service.
- [ ] OpenTelemetry Collector exporter — ship captured traces into existing observability pipelines.
- [ ] HD Phase 2: Supabase dashboard (sign up → API key → quickstart command), lazy GCS bucket provisioning, free tier quota enforcement (10 sessions/day).
- [ ] HD Phase 3: Web UI case browser (wire otel-server `/api/tenants/{tenantID}/sessions` + `/v1/cases/{caseId}`).
