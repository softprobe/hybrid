# Tasks

> **Execution:** Work in **document order** (top to bottom). The **first** unchecked `[ ]` item is the active task unless a `Depends on:` line says otherwise.
> **Process:** Follow `AGENTS.md` (TDD for code, no scope beyond `docs/design.md`).
> **When done:** Change `[ ]` → `[x]` and append a short commit-style note on the same line.

## Architecture (load-bearing context — read before coding)

- **`softprobe-runtime` (OSS, unified)** — serves **both** the HTTP control API ([`spec/protocol/http-control-api.md`](spec/protocol/http-control-api.md)) **and** the proxy OTLP API ([`spec/protocol/proxy-otel-api.md`](spec/protocol/proxy-otel-api.md)) from **one Go process** with a shared in-memory session store. v1: no database required; Redis/Postgres only for HA (see [`docs/platform-architecture.md`](docs/platform-architecture.md) §10.2).
- **Internal package layout:** `internal/store/` (shared session/case/rules state), `internal/controlapi/` (JSON control handlers), `internal/proxybackend/` (OTLP inject/extract handlers — `POST /v1/inject`, `POST /v1/traces`).
- **Deployment:** `SOFTPROBE_RUNTIME_URL` (CLI/SDKs) and proxy WASM `sp_backend_url` both point at the **same** `softprobe-runtime` base URL. No second service needed locally.
- **Canonical language:** Go. Proxy calls `softprobe-runtime` OTLP endpoints over HTTP; it does not link the runtime as a library.

## Legend

| Mark | Meaning |
|------|---------|
| `[ ]` | Not started |
| `[~]` | In progress |
| `[x]` | Done (note appended) |

`Depends on:` — complete those `[x]` before starting this item. If none, order alone defines readiness.

## Shipped to date (summary)

The capture-and-replay **core** is live and covered by e2e tests:

- Unified `softprobe-runtime` with shared in-memory store, `/health`, `/v1/meta`, full session lifecycle (`/v1/sessions`, `/v1/sessions/{id}/{load-case,rules,policy,fixtures/auth,stats,close}`), and OTLP handlers (`/v1/inject`, `/v1/traces`).
- Envoy + Softprobe WASM proxy with ingress (8082) + egress (8084) topology; Docker Compose harness for local dev.
- Strict policy end-to-end (proxy returns a documented error for unmocked outbound).
- Four SDKs (TypeScript, Python, Java, Go) with the minimal parity surface (`loadCaseFromFile`, `loadCase`, `findInCase`, `findAllInCase`, `mockOutbound`, `clearRules`, `setPolicy`, `setAuthFixtures`, `close`) and typed errors (`*RuntimeError`, `*RuntimeUnreachableError`, `*UnknownSessionError`, `*CaseLoadError`, `*CaseLookupAmbiguityError`).
- `softprobe doctor`, `softprobe session {start,load-case,rules apply,policy set --strict,stats,close}`, `softprobe inspect case`, `softprobe generate jest-session`.
- Docs site (`docs-site/`) with concepts, reference, deployment, and guides pages; package-level READMEs for all four SDKs with publication-status disclosure.
- End-to-end acceptance tests: `e2e/go/go-capture/`, `e2e/go/go-replay/`, `e2e/jest-replay/`, `e2e/pytest-replay/`, `e2e/junit-replay/` — all green on the same compose stack using `spec/examples/cases/fragment-happy-path.case.json`.

Full history: see `git log` and [`docs-site/changelog.md`](docs-site/changelog.md).

---

# Delivery phases (in progress)

We promised more in `docs-site/` than `cmd/softprobe`, the runtime, and the SDKs ship today. These phases close the gap. Ordering prefers **truth-in-docs first** (PD6.0), then horizontal plumbing (PD2 auth, PD5 release hygiene) that unblocks everything else, then CLI surface (PD1), observability (PD4), and TS SDK reference alignment (PD3).

## Phase PD6.0 — Immediate doc truth-sync (banners only)

Keep users from copy-pasting broken snippets while we ship the backing features. **Docs-only**; zero code risk; do first.

**Depends on:** none.

- [x] **PD6.0a — Remove `softprobe suite run` from _Shipped_ in `docs-site/roadmap.md`.** Move it to _In progress_ tied to PD1.7. done initially as a banner; superseded by PD1.7 landing — `docs-site/roadmap.md` now calls the suite runner out as shipped in the current build with a link to `e2e/cli-suite-run/`.
  **Verify:** `rg "softprobe suite" docs-site/roadmap.md` points at the harness, not at planned work.

- [x] **PD6.0b — "Not shipped yet" banners in `docs-site/reference/cli.md`.** Add `::: warning Not shipped yet` to the sections for `capture run`, `replay run`, `suite {run,validate,diff}`, `validate {case,rules,suite}`, `inspect session`, `generate test`, `export otlp`, `scrub`, and `completion`. Each banner links to its PD task in this file. done: 9 banners added (one per command group). As each delivery task completes the banner flips to a `::: tip Ships in this build` pointer — done for `suite {run,validate,diff}` (PD1.7g).
  **Verify:** `rg "Not shipped yet" docs-site/reference/cli.md` only matches commands that truly aren't shipped.

- [x] **PD6.0c — Banner on `docs-site/guides/run-a-suite-at-scale.md`.** Replaced with a `::: tip Ships in this build` pointer (including a link to the `e2e/cli-suite-run/` harness) now that PD1.7g is done.
  **Verify:** guide no longer carries a "Not shipped yet" banner; intro tip references the e2e harness.

- [x] **PD6.0d — K8s deployment footnotes.** `docs-site/deployment/kubernetes.md` now carries `::: warning Not shipped yet` banners on the `/metrics`, `SOFTPROBE_LOG_LEVEL`, `{sessionId}` template, and object-storage URL subsections, each linking to the relevant PD4 task (`PD4.1a`, `PD4.2a`, `PD4.3a`, `PD4.4a`).
  **Verify:** each affected subsection carries a planned-note linking to its PD4 task.

- [x] **PD6.0e — FAQ license claim.** Resolved by adopting a dual-license split:
  **Softprobe Source License 1.0** (SPDX `LicenseRef-Softprobe-Source-License-1.0` — an FSL-1.1-derived license with a materially broader Competing Use clause covering on-premises, bundled, and rebranded redistribution in addition to hosted-service redistribution) for `softprobe-runtime/`, `softprobe-proxy/`, and the CLI; **Apache-2.0** for the four SDKs and `spec/`. Root `LICENSE`, per-package `LICENSE`s, `LICENSING.md` path map, rewritten FAQ (with "Can I build a commercial product that uses captured traffic?" guidance), roadmap, VitePress footer, and package manifests (`softprobe-js/package.json`, `softprobe-java/pom.xml`) all landed together.
  **Verify:** `find . -maxdepth 3 -iname LICENSE -not -path '*/node_modules/*'` lists root + all six packages + `spec/`; `head -1 LICENSE` matches `# Softprobe Source License, Version 1.0`; `grep -R "Apache 2.0 for all OSS components" docs-site/` returns no matches.

- [x] **PD6.0f — TS SDK reference banner.** PD3 shipped; replaced the planned `::: warning` with a `::: tip Ships in this build` at the top of `docs-site/reference/sdk-typescript.md` linking Phase PD3, `errors.ts`, `hooks.ts`, `suite.ts`, and `hook-runner.ts`. Aligned the error catalog and class hierarchy with `errors.ts` (`SoftprobeRuntimeUnreachableError`, `SoftprobeUnknownSessionError`, `HookExecutionError`) and the version table with `package.json` / `VERSION` (`2.0.x` npm vs `0.5.x` runtime line).
  **Verify:** tip present at top of page; `rg "Not shipped yet" docs-site/reference/sdk-typescript.md` is empty; error table matches `softprobe-js/src/errors.ts`.

---

## Phase PD5 — Release hygiene

One-time cleanup so every downstream feature has somewhere to publish.

**Depends on:** none (can parallelize all items).

- [x] **PD5.1a — LICENSE coverage across the repo.** Superseded by the dual-license change in PD6.0e: root `LICENSE` (Softprobe Source License 1.0), `softprobe-runtime/LICENSE` and `softprobe-proxy/LICENSE` (same), `softprobe-js/LICENSE` (migrated from MIT → Apache-2.0), `softprobe-python/`, `softprobe-java/`, `softprobe-go/`, and `spec/` (all Apache-2.0). `LICENSING.md` maps every path. `softprobe-js/package.json` and `softprobe-java/pom.xml` now declare Apache-2.0.
  **Verify:** `find . -maxdepth 3 -iname LICENSE -not -path '*/node_modules/*' -not -path '*/target/*'` lists root + `softprobe-{runtime,proxy,js,python,java,go}/LICENSE` + `spec/LICENSE` (8 files total).

- [x] **PD5.2a — CLI version string.** Replaced `const version` in `cmd/softprobe/main.go` with `internal/version` (`Version` var + `SemverTag` / `CLIDetail`). `--version` and `doctor` human first line print `softprobe v… (spec http-control-api@v1)`; `doctor --json` uses `cliVersion` with the same detail string; drift check uses `cli` = `SemverTag()`. `internal/version/version_test.go` pins the ldflags example.
  **Verify:** `go build -ldflags "-X softprobe-runtime/internal/version.Version=v0.5.0" -o /tmp/sp ./cmd/softprobe && /tmp/sp --version` → `softprobe v0.5.0 (spec http-control-api@v1)`.

- [ ] **PD5.3a — Runtime container image on ghcr.** CI workflow in `.github/workflows/` builds + pushes `ghcr.io/softprobe/softprobe-runtime:<sha>` + `:v<tag>` from `softprobe-runtime/Dockerfile`. Write the workflow with a smoke-step that pulls the image and runs `docker run ghcr.io/softprobe/softprobe-runtime:<sha> --version`.
  **Verify:** workflow green on a PR; image pullable post-tag.

- [ ] **PD5.3b — Proxy WASM OCI bundle on ghcr.** Same pattern for `ghcr.io/softprobe/softprobe-proxy:<tag>` — OCI image containing `sp_istio_agent.wasm`. Validate via Istio `WasmPlugin` URL in a smoke job.
  **Verify:** `oras pull` surfaces the wasm blob; documented `WasmPlugin.url` resolves.

- [ ] **PD5.4a — softprobe-js npm publish workflow.** Tag-triggered CI job runs `npm publish`. Align `package.json#version` with release tag.
  **Verify:** dry-run succeeds; next release tag lands on npm.

- [ ] **PD5.4b — softprobe-python PyPI publish workflow.** Tag-triggered; publishes to TestPyPI first, then PyPI.
  **Verify:** dry-run succeeds against TestPyPI.

- [ ] **PD5.4c — softprobe-java Maven Central publish workflow.** Tag-triggered; signs + publishes to OSSRH; auto-promote.
  **Verify:** Sonatype staging validation green.

- [ ] **PD5.4d — softprobe-go module path.** Rename module to `github.com/softprobe/softprobe-go`, update in-repo `replace` directives to reference the external path as a fallback, tag `v0.5.0`.
  **Verify:** `go get github.com/softprobe/softprobe-go@v0.5.0` from a clean GOPATH succeeds.

---

## Phase PD2 — Runtime auth plumbing in SDKs and CLI

`docs-site/reference/http-control-api.md` promises: _"the CLI and SDKs read `SOFTPROBE_API_TOKEN` from the environment and attach the header automatically."_ Today none of them do. Users enabling auth are locked out.

**Depends on:** none.

- [x] **PD2.1a — CLI attaches `Authorization: Bearer`.** Every HTTP call in `cmd/softprobe/main.go` (and `generate_jest_session.go`) reads `SOFTPROBE_API_TOKEN` and attaches the header. Failing tests first: fake runtime asserts the header. done: added `newRuntimeRequest` helper that attaches `Authorization: Bearer $SOFTPROBE_API_TOKEN` when set; migrated all 7 call sites (doctor health + meta, session start/stats/close/load-case/rules apply/policy set). New `cmd/softprobe/auth_test.go` runs the full CLI surface through a capturing fake server with the token set, asserts every request carries the expected header, and also covers the env-unset case. `go test ./...` green.
  **Verify:** all existing CLI commands work with and without the env var; `main_test.go` covers both cases.

- [x] **PD2.1b — softprobe-js honors `SOFTPROBE_API_TOKEN`.** `new Softprobe({ apiToken })` option overrides the env. `RuntimeClient` attaches the header on every request. done: added `apiToken` to `SoftprobeRuntimeClientOptions`, threaded through the `Softprobe` facade; `postJson` attaches `Authorization: Bearer …` when `apiToken ?? process.env.SOFTPROBE_API_TOKEN` is non-empty (trimmed). Added 5 auth tests in `runtime-client.test.ts` + 1 facade test in `softprobe.test.ts`. Full `npm test` green (329 tests).
  **Verify:** `src/__tests__/runtime-auth.test.ts` asserts the header value and constructor override.

- [x] **PD2.1c — softprobe-python honors `SOFTPROBE_API_TOKEN`.** `Softprobe(api_token=…)` overrides env. done: `Client(api_token=…)` keyword, threaded through the `Softprobe` facade; `_post_json` attaches `Authorization: Bearer …` when `api_token ?? os.environ["SOFTPROBE_API_TOKEN"]` is non-empty (whitespace-trimmed). Added 5 transport-level tests in `tests/test_client.py` + 1 facade test in `tests/test_softprobe.py`. Full `python3 -m unittest discover tests` green (29 tests).
  **Verify:** `tests/test_auth.py` covers both paths.

- [x] **PD2.1d — softprobe-java honors `SOFTPROBE_API_TOKEN`.** `Softprobe(baseUrl, apiToken)` constructor; builder overrides env. done: added `Client(String, Transport, String)` ctor + `Client.withApiToken(baseUrl, apiToken)` factory + `Softprobe(String, Transport, String)` facade ctor + `Softprobe.withApiToken`. `postJson` now builds a mutable headers map and attaches `authorization: Bearer …` when resolved token is non-empty (after trim); `sendWithHttpClient` iterates all headers so the bearer survives the real HTTP path. Added 3 transport-level tests in `ClientTest` + 1 facade test in `SoftprobeSessionTest`. `mvn test` green (27 tests).
  **Verify:** JUnit test covers both paths.

- [x] **PD2.1e — softprobe-go honors `SOFTPROBE_API_TOKEN`.** `softprobe.Options{APIToken}` overrides env. done: added `WithAPIToken` client option + `Options.APIToken`/`APITokenSet` fields; `postJSON` attaches `Authorization: Bearer …` when the resolved token is non-empty (trim). Resolution picks the explicit option first (so an explicit empty string can disable the env fallback); otherwise reads `SOFTPROBE_API_TOKEN` at request time so `t.Setenv` works without reconstruction. Added 6 tests in a new `auth_test.go` (explicit option, env fallback, override, unset, whitespace, facade wiring). `go test ./softprobe/` green.
  **Verify:** `auth_test.go` asserts header; `errors.As` still recovers typed errors when auth is on.

- [ ] **PD2.1f — e2e auth path.** Compose override sets `SOFTPROBE_API_TOKEN=sp_test` on the runtime; each of `e2e/go`, `jest-replay`, `pytest-replay`, `junit-replay` runs green picking up the token from env.
  **Verify:** `SOFTPROBE_API_TOKEN=sp_test docker compose … up --wait` + all four harnesses green.

---

## Phase PD1 — CLI contract completeness

The CLI reference (`docs-site/reference/cli.md`) and `index.md` ("All CLI commands support `--json` output and stable exit codes") describe a much larger surface than `cmd/softprobe` implements today.

**Depends on:** PD2 (auth) for multi-command orchestration; PD5.2 (version string) for `--version` correctness.

### PD1.1 Global CLI contract

- [ ] **PD1.1a — Stable exit codes.** Map the documented codes in `cmd/softprobe`: `2` invalid args, `3` runtime unreachable, `4` session not found, `5` schema/validation error, `10` doctor fail, `20` suite fail. Failing tests first.
  **Verify:** one test per documented code.

- [x] **PD1.1b — Global flags.** `--verbose/-v` (stderr diagnostics), `--quiet/-q`, `--help/-h`, honor `NO_COLOR`. Consistent across subcommands.
  **Verify:** CLI tests parse help output and verbose stderr.

- [ ] **PD1.1c — Universal `--json`.** Add the common `status/exitCode/error?` envelope (per `cli.md` stability table) to every mutating subcommand: `session load-case`, `session rules apply`, `session policy set`, `inspect case`, `generate jest-session`.
  **Verify:** one JSON-parsing test per subcommand covers `status` + `exitCode` + at least one command-specific field.

### PD1.2 `doctor` expansion

- [x] **PD1.2a — Proxy WASM binary check.** Try `$WASM_PATH`, `/etc/envoy/sp_istio_agent.wasm` and similar. Missing → warning only (non-fatal).
  **Verify:** test with file present vs absent.

- [x] **PD1.2b — Header-echo smoke test.** Optional POST through a configured proxy URL; assert `x-softprobe-session-id` round-trip. Missing → warning.
  **Verify:** in-process fake proxy test.

- [x] **PD1.2c — `--verbose` mode.** Log HTTP request/response details to stderr.
  **Verify:** verbose test asserts URL + status in stderr.

### PD1.3 `session` subcommand completeness

- [ ] **PD1.3a — `session start --policy FILE --case FILE`.** Chain session-create → apply-policy → load-case atomically.
  **Verify:** test asserts all three HTTP calls fire in order.

- [ ] **PD1.3b — `session policy set --file PATH`.** Accept a policy file alongside the existing `--strict` shortcut.
  **Verify:** CLI test parses YAML/JSON and posts the expected body.

- [ ] **PD1.3c — `session close --out PATH`.** For capture sessions, override the capture file output path (used with PD4.3 template substitution).
  **Verify:** capture e2e test writes to a custom path.

- [ ] **PD1.3d — `inspect session`.** Read-only: dump policy, rules, loaded-case summary, stats for a live session. Human + `--json`.
  **Verify:** integration test applies policy + rules, then asserts inspect output shape.

### PD1.4 `validate` subcommand

- [ ] **PD1.4a — `validate case FILE`.** Schema-validate against `spec/schemas/case.schema.json`. Exit `5` on invalid.
  **Verify:** tests cover valid + invalid fixtures.

- [ ] **PD1.4b — `validate rules FILE`.** Same for `rule.schema.json`.
  **Verify:** tests cover valid + invalid.

- [ ] **PD1.4c — `validate suite FILE`.** Same for `suite.schema.json` (ships in PD1.7a).
  **Verify:** tests cover valid + invalid.

### PD1.5 `capture` subcommand

- [ ] **PD1.5a — `capture run --driver CMD --out PATH`.** Orchestrate: start capture session → export `SOFTPROBE_SESSION_ID` → run driver → close session → write case.
  **Verify:** e2e test with a trivial driver (`sh -c 'curl …'`).

- [ ] **PD1.5b — `capture run --timeout DURATION`.** Enforce wall-clock timeout on driver.
  **Verify:** fake slow driver → timeout exit.

- [ ] **PD1.5c — `capture run --redact-file PATH`.** Apply redaction rules during capture.
  **Verify:** captured bytes omit redacted values.

### PD1.6 `replay run` diagnostic

- [ ] **PD1.6a — `replay run --session ID`.** Report inject hit/miss stats for a live session (wraps `session stats`).
  **Verify:** test against a session with known injects.

### PD1.7 `suite` subcommand (the big one)

**Depends on:** PD1.4 (`validate suite`).

- [x] **PD1.7a — Suite YAML schema + parser.** Land `spec/schemas/suite.schema.json` and `softprobe-runtime/internal/suite/`.
  **Verify:** unit tests validate examples; invalid suites fail. done: schema lives in `spec/schemas/suite.schema.json`; parser in `softprobe-runtime/cmd/softprobe/suite_parse.go` with defaults/overrides/env and `${VAR:-default}` expansion.

- [x] **PD1.7b — `suite run` (sequential).** Read case globs, start one session per case, load + run, collect results. No parallelism yet.
  **Verify:** e2e test runs a 2-case suite against the compose stack. done: `suite_pipeline.go` drives session start → load-case → findInCase mocks → SUT call → assert; `TestSuiteRunPipeline*` exercises it end-to-end via `httptest`.

- [x] **PD1.7c — `suite run --parallel N`.** Bounded worker pool; per-case session isolation.
  **Verify:** e2e test with N=4. done: `runSuiteCasesPipeline` owns a semaphore-gated worker pool keyed on `--parallel`, default `min(32, cpu*4)`.

- [x] **PD1.7d — `--junit PATH` / `--report PATH`.** JUnit XML + HTML report writers.
  **Verify:** XML validates against the JUnit XSD; HTML opens. done: `writeJUnit` / `writeSuiteHTMLReport` in `suite.go`.

- [x] **PD1.7e — `suite validate`.** Parse YAML, resolve globs, check hook references.
  **Verify:** tests cover missing-file and missing-hook errors. done: `runSuiteValidate` shares parser+schema with `suite run` and reports missing case files.

- [x] **PD1.7f — `suite diff --baseline --current`.** Compare two case sets for drift (status codes, body shape).
  **Verify:** test with known drift. done: `runSuiteDiff` hashes outbound span signatures and reports added/removed.

- [x] **PD1.7g — Hook sidecar + e2e harness.** Node sidecar embedded in the Go binary loads user TS/JS hooks and serves `RequestHook`/`MockResponseHook`/`BodyAssertHook`/`HeadersAssertHook` over newline-delimited JSON; `e2e/cli-suite-run/` drives the compose stack end-to-end (MockResponseHook rewrites `/fragment`, BodyAssertHook validates `/hello`). Shares hook shapes with the TS SDK adapter.
  **Verify:** `go test ./cmd/softprobe/` green (incl. `TestSuiteRunPipelineWithHookSidecar`) and `e2e/cli-suite-run` passes against docker-compose (see harness README).

### PD1.8 Auxiliary: `generate test`, `export otlp`, `scrub`, `completion`

- [x] **PD1.8a — `generate test --framework {jest,vitest,pytest,junit}`.** Full test-file generator (not just the session helper). Share traversal with `generate jest-session`.
  **Verify:** one golden-output test per framework.

- [ ] **PD1.8b — `export otlp --case … --endpoint …`.** Stream case traces to an OTLP HTTP endpoint.
  **Verify:** e2e test hits a mock OTel collector.

- [ ] **PD1.8c — `scrub FILE [--rules PATH]`.** Apply redaction rules to a case file in place.
  **Verify:** before/after diff test.

- [ ] **PD1.8d — `completion {bash,zsh,fish}`.** Emit shell-completion scripts.
  **Verify:** golden-output test per shell.

---

## Phase PD4 — Runtime observability and capture operations

`docs-site/deployment/kubernetes.md` and `reference/cli.md` describe operational features the runtime doesn't implement.

**Depends on:** none.

- [ ] **PD4.1a — Prometheus `/metrics` endpoint.** Emit `softprobe_sessions_total{mode=…}`, `softprobe_inject_requests_total{result=hit|miss|error}`, `softprobe_inject_latency_seconds` histogram, `softprobe_extract_spans_total`. Failing tests scrape the endpoint and parse the exposition format.
  **Verify:** tests assert counter + histogram increments under load.

- [ ] **PD4.2a — `SOFTPROBE_LOG_LEVEL` honored.** Wire into the runtime's logger; values `debug|info|warn|error`.
  **Verify:** test per level captures expected output.

- [ ] **PD4.3a — `{sessionId}` template in `SOFTPROBE_CAPTURE_CASE_PATH`.** Interpolate `{sessionId}`, `{ts}`, `{mode}` before writing. Back-compat: plain path still works.
  **Verify:** capture e2e test writes to interpolated path; baseline test (no placeholder) still green.

- [ ] **PD4.4a — Object-storage case writers.** Add schemes `file://` (default), `s3://`, `gs://`, `azblob://` to `internal/proxybackend/case_writer.go`. Credentials via standard workload identity paths.
  **Verify:** unit tests against minio / fake-gcs-server / azurite emulators; local `file://` stays the default.

---

## Phase PD3 — TypeScript SDK reference reality alignment

**Done.** The TS SDK reference (`docs-site/reference/sdk-typescript.md`) now matches the shipped package: short error names, `/hooks` + `/suite` subpaths, `runSuite`, and `setLogger` / `SOFTPROBE_LOG` (see PD6.0f tip + error table refresh).

**Depends on:** PD1.7 (for `runSuite` to have a backing format).

- [x] **PD3.1a — Error aliases + unified base.** Introduce `SoftprobeError` base class; export `RuntimeError` (alias for `SoftprobeRuntimeError`), `CaseLookupError` (alias for `SoftprobeCaseLookupAmbiguityError`), `CaseLoadError` (alias for `SoftprobeCaseLoadError`). Keep the long names as canonical.
  **Verify:** Jest tests import each documented name.

- [x] **PD3.1b — `@softprobe/softprobe-js/hooks` subpath.** Add `RequestHook`, `MockResponseHook`, `BodyAssertHook`, `HeadersAssertHook`, `Issue` types in `src/hooks/index.ts`; wire into `package.json#exports`.
  **Verify:** TS test imports from the subpath.

- [x] **PD3.1c — `@softprobe/softprobe-js/suite` subpath + `runSuite`.** After PD1.7b lands, expose a Node-side `runSuite(suiteYamlPath, { hooks, baseUrl, appUrl, filter, parallel })` that registers `describe`/`it`.
  **Verify:** in-repo Jest test runs a 2-case suite through `runSuite`.

- [x] **PD3.1d — `setLogger` + `SOFTPROBE_LOG`.** Module-level logger hook; default no-op; env var turns debug on.
  **Verify:** test captures debug output when enabled, nothing when disabled.

- [x] **PD3.1e — Version string alignment.** Reconcile `softprobe-js/package.json#version = 2.0.10` with the docs' v0.5.x narrative. Pick a single source of truth and align docs + SDK + runtime version tables.
  **Verify:** version table in `sdk-typescript.md` matches `package.json` and `softprobe --version`.

- [x] **PD3.1f — Hook runtime (TS) + end-to-end proof.** PD3.1b/c only shipped types; nothing actually *ran* a hook. Land `src/hook-runner.ts` (`applyRequestHook`, `applyMockResponseHook`, `runBodyAssert`, `runHeadersAssert`, `HookExecutionError`), wire MockResponseHook invocation into `runSuite` so hooks reach `session.mockOutbound()` on the wire, ship `examples/hooks/` (README + suite.yaml + one hook file per kind + captured case), and add `src/__tests__/hooks-e2e.test.ts` covering all four hook kinds plus a full `runSuite()` → fake runtime round-trip.
  **Verify:** the e2e test suite decodes the `/v1/sessions/{id}/rules` request body and asserts the `unmaskCard` transform reaches the runtime; 12 new tests green.

- [x] **PD3.1g — Docker-compose e2e harness for hooks + suite.yaml.** Add `e2e/jest-hooks/` (package.json, jest config, tsconfig, `suites/fragment.suite.yaml`, `hooks/rewrite-dep.ts` + `hooks/assert-hello.ts`, `fragment.hooks.test.ts`) that drives `runSuite()` from `@softprobe/softprobe-js/suite` against the real `e2e/docker-compose.yaml` stack. The test registers a MockResponseHook via suite.yaml, fetches `GET /v1/sessions/{id}/state` on the live `softprobe-runtime` container, and asserts the mock rule carries the hook-transformed body. Also exercises a BodyAssertHook on the live SUT response via the `onCase` handle.
  **Verify:** `docker compose -f e2e/docker-compose.yaml up -d --wait && cd e2e/jest-hooks && npm install && npm test` is green.

- [x] **PD3.1h — Proxy session-id propagation (was OPEN-1).** The Envoy + Softprobe WASM proxy used to derive its tracestate session id from the inbound `traceId` (`sp-session-00000000-…`) when no session id was present on the request, inventing a synthetic id in a proxy-specific format. That id polluted the SDK session-id namespace and caused `/v1/inject` to always miss against the runtime.
  **Fix (landed):** `softprobe-proxy/src/otel.rs::SpanBuilder::with_context` now treats the session id as opaque — it reads `x-softprobe-session-id` (and `x-sp-session-id` / `tracestate`) verbatim regardless of format, and leaves `session_id` empty when none is present. `context.rs` skips both `/v1/inject` and `/v1/traces` dispatch when `session_id` is empty, avoiding useless round trips and 400s on the runtime. Unit tests in `session_id_tests` pin the opaque-format contract. `e2e/jest-hooks/fragment.hooks.test.ts` drives traffic through the ingress proxy on `:8082` and asserts `body.dep === 'mutated-by-hook'`, proving the SDK-issued `sess_…` id survives end-to-end through both WASM hops.
  **Verify:** `cd softprobe-proxy && make build` rebuilds the WASM; `docker compose -f e2e/docker-compose.yaml up -d --wait && cd e2e/jest-hooks && npm test` is green.

---

## Phase PD6 — Doc truth sync (after each code phase lands)

- [ ] **PD6.1 — Remove CLI banners.** As each PD1 task lands, remove the corresponding "Not shipped yet" banner from `cli.md` and the guide pages.
  **Verify:** `rg "Not shipped yet" docs-site/` returns matches only for still-pending work.

- [ ] **PD6.2 — Update `cli.md` field-stability table.** Keep the `--json` field table honest as PD1.1c expands coverage.
  **Verify:** table rows match the subcommands that actually emit JSON.

- [ ] **PD6.3 — Refresh SDK references.** Update version tables, install snippets, and error catalogs as PD5.4 and PD3 land.
  **Verify:** each reference page's import snippet compiles against the published SDK.

- [ ] **PD6.4 — Refresh `roadmap.md`.** Move items from _In progress_ to _Shipped_ as phases complete.
  **Verify:** roadmap entries match `git tag` + `docs-site/changelog.md`.

---

## Phase PD7 — Dogfood `softprobe` against itself

Use our own capture-and-replay engine to pin the CLI + SDK control-plane contract against a recorded runtime. Cases become both tests and living protocol documentation. Design note and rationale: [`docs/dogfooding.md`](docs/dogfooding.md).

**Depends on:** PD2 (auth headers must land before capture so cases reflect the real contract). PD1.7 is recommended for PD7.5 only.

### PD7.1 Reference build + capture driver

- [ ] **PD7.1a — `DOGFOOD_REF` policy.** Add `spec/dogfood/REFERENCE.md` defining the reference build used to record cases: initially `main@<sha>` promoted via a protected-branch check; post-PD5.3a switch to released tags (`ghcr.io/softprobe/softprobe-runtime:v<tag>`). Document the invariant: a case refresh MUST land in a PR that contains no runtime or SDK code changes (per `docs/dogfooding.md` §5 best practice 9).
  **Verify:** `rg 'DOGFOOD_REF' spec/dogfood/REFERENCE.md` matches; CI doc lint passes; PD5.3a graduation note present.

- [ ] **PD7.1b — Deterministic capture driver.** Land `cmd/softprobe-dogfood-capture/` (Go) and `spec/dogfood/capture.sh` that: (1) starts `e2e/docker-compose.yaml`, (2) runs the canonical CLI flow from `docs/dogfooding.md` §7 (`doctor` → `session start` → `load-case` → `rules apply` → `policy set --strict` → `session stats` → `session close`) with egress proxy on `:8084` and `SOFTPROBE_API_TOKEN=sp_dogfood`, (3) post-processes the captured case to canonicalize session ids, timestamps, and trace ids to stable placeholders, (4) writes `spec/examples/cases/control-plane-v1.case.json`. Use `spec/examples/cases/fragment-happy-path.case.json` as the inner case the CLI loads.
  **Verify:** running the driver twice produces byte-identical output; golden test in `cmd/softprobe-dogfood-capture/` pins the canonicalization.

- [ ] **PD7.1c — `make capture-refresh` target.** Root `Makefile` target (and `softprobe-runtime/Makefile` counterpart) runs the driver and prints `git diff -- spec/examples/cases/control-plane-v1.case.json`. Refuses to run if the working tree has uncommitted runtime or SDK changes (per `docs/dogfooding.md` §5.9).
  **Verify:** unit test invokes the target on a clean tree and asserts no-op diff on the second invocation; dirty-tree test asserts non-zero exit.

### PD7.2 CLI replay test (additive, not replacement)

- [ ] **PD7.2a — `cmd/softprobe/dogfood_replay_test.go`.** Start a real `softprobe-runtime` (via the existing in-process harness used by `auth_test.go`), load `spec/examples/cases/control-plane-v1.case.json` into a session, run each CLI subcommand through the canonical flow, assert every outbound HTTP request matched a recorded rule. Existing `main_test.go` and `auth_test.go` stay untouched (per `docs/dogfooding.md` §5.2).
  **Verify:** `go test ./cmd/softprobe/... -run Dogfood` green; mutating the committed case to expect a different `Authorization` value fails the test with a clear message.

- [ ] **PD7.2b — Failure taxonomy.** Replay errors distinguish *code regression*, *case staleness*, and *transport failure* (per `docs/dogfooding.md` §5.6), each mapping to a documented CLI exit code (`3` runtime unreachable, `5` schema/validation) from `docs-site/reference/cli.md`.
  **Verify:** three negative tests, one per class; each asserts the exit code and a substring of the error message.

### PD7.3 Cross-SDK parity using the same case

- [ ] **PD7.3a — TS SDK parity test.** `softprobe-js/src/__tests__/parity-dogfood.test.ts` loads `spec/examples/cases/control-plane-v1.case.json` into a fake runtime and drives the full facade (`loadCaseFromFile`, `mockOutbound`, `setPolicy`, `setAuthFixtures`, `close`) through the recorded conversation. Asserts every outbound HTTP hits a recorded rule (`docs/dogfooding.md` §3 in-scope item 2).
  **Verify:** removing auth header support from `postJson` regresses this test.

- [ ] **PD7.3b — Python SDK parity test.** `softprobe-python/tests/test_parity_dogfood.py` — same semantics as PD7.3a.
  **Verify:** as PD7.3a, against the Python `Client`.

- [ ] **PD7.3c — Java SDK parity test.** `softprobe-java/src/test/java/.../ParityDogfoodTest.java` — same semantics as PD7.3a.
  **Verify:** as PD7.3a, against the Java `Client`.

- [ ] **PD7.3d — Go SDK parity test.** `softprobe-go/softprobe/parity_dogfood_test.go` — same semantics as PD7.3a.
  **Verify:** as PD7.3a, against the Go `Client`.

### PD7.4 CI refresh workflow

- [ ] **PD7.4a — Nightly refresh job.** `.github/workflows/dogfood-refresh.yml` runs on a schedule and on manual dispatch. If `make capture-refresh` produces a diff, it opens a PR titled `chore(dogfood): refresh control-plane-v1.case.json` with the diff embedded in the body. Never auto-merges; never runs on PRs that modify runtime or SDK code (per `docs/dogfooding.md` §5.5, §5.9).
  **Verify:** workflow green on a seeded manual dispatch; verify the PR-open path by temporarily perturbing the recorded case.

- [ ] **PD7.4b — Refresh playbook.** `docs-site/guides/contribute-dogfood.md` documents: when a dogfood test fails locally, run `make capture-refresh`, inspect the diff, and either fix the code regression or land the protocol change as a standalone refresh PR. Links from `AGENTS.md` section 12 and from `docs/dogfooding.md` §5.5.
  **Verify:** guide renders on the docs site; cross-links present.

### PD7.5 Docs-as-suite (optional)

**Depends on:** PD1.7 (suite runner), PD7.2a (proves the replay shape).

- [ ] **PD7.5a — `docs/snippets.suite.yaml`.** Extract every copy-paste CLI flow from `docs-site/guides/` into a suite. CI runs `softprobe suite run docs/snippets.suite.yaml` and fails if a documented snippet no longer matches the real CLI behavior (per `docs/dogfooding.md` §3 in-scope item 3).
  **Verify:** intentionally breaking one snippet fails the suite with a line-accurate error; reverting it returns green.

---

## Parking lot (non-sequential)

- [ ] **OpenAPI bundle** for the control API (optional; `spec/schemas/session-*.schema.json` may suffice for v1).
- [ ] **Multi-tenant session-id strategy** + schema updates.
- [ ] **Redis / Postgres session store** for multi-replica HA (promised on the roadmap; architecture ADR when we pick up the work).
- [ ] **Multi-process runtime split** (`softprobe-control` + `softprobe-otlp`) — promised on the roadmap; lands after Redis store.
- [ ] **Hosted service GA on `o.softprobe.ai`** — commercial track; docs pages already carved out.
- [ ] **Hook runtime v1** — Node sidecar executing TS/JS hooks from the Go CLI.
- [ ] **Ruby and .NET SDKs** — after the four current SDKs reach v1 GA.
- [ ] **Case diffing in the browser** — web UI to diff two case files or two replays.
- [ ] **Cloud-managed rules** — version-controlled, shareable rule bundles in the hosted service.
- [ ] **OpenTelemetry Collector exporter** — ship captured traces into existing observability pipelines without the `export otlp` shim.
- [ ] **gRPC / WebSockets / long-running SSE** in the WASM filter — v1 ships HTTP/1.1 + HTTP/2 request-response only.
