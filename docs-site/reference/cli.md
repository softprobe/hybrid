# CLI reference

The `softprobe` CLI is the primary interface for humans, CI, and AI agents. It speaks only HTTP to the runtime — no local state, no config files to manage. Commands that emit structured results support `--json` for machine-readable output (not `completion`, which prints a shell script). All commands return stable exit codes.

## Global options

| Flag | Default | Purpose |
|---|---|---|
| `--runtime-url URL` | `$SOFTPROBE_RUNTIME_URL` or `http://127.0.0.1:8080` | Where to find the runtime |
| `--json` | off | Emit structured JSON instead of human text |
| `--verbose` / `-v` | off | Extra diagnostic logging to stderr |
| `--quiet` / `-q` | off | Suppress non-error output |
| `--help` / `-h` | — | Print command help |
| `--version` | — | Print `softprobe SEMVER (spec http-control-api@v…)`; release builds set `SEMVER` with `-ldflags "-X softprobe-runtime/internal/version.Version=v0.5.0"` |

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Generic failure (see stderr) |
| `2` | Invalid arguments |
| `3` | Runtime unreachable |
| `4` | Session not found |
| `5` | Schema / validation error |
| `10` | `doctor` reports one or more failed checks |
| `20` | `suite run` completed with failures |

Agents and CI should check `$?` (or the exit code field in JSON) before parsing further.

---

## `softprobe doctor`

Check the local environment. Run this first whenever something is wrong.

```bash
softprobe doctor                # human-readable
softprobe doctor --json         # machine-readable
softprobe doctor --verbose      # include HTTP request/response details
```

Example output (shape; paths and markers vary by machine):

```
softprobe v0.5.0 (spec http-control-api@v1)
runtime healthy: ok
runtimeVersion: 0.0.0-dev
specVersion: http-control-api@v1
schemaVersion: 1
✓ runtime-reachable
✓ version-drift
✓ schema-version
⚠ wasm-binary: WASM binary not found at /etc/envoy/sp_istio_agent.wasm
⚠ header-echo: x-softprobe-session-id: header not echoed by proxy (may indicate misconfig)
```

Exit code: `0` on all green, `10` on any failure. Warnings don't affect exit code.

### What it checks

| Check | Failing condition | Exit contribution |
|---|---|---|
| **Runtime reachable** | HTTP error on `GET /health` | fatal → exit `10` |
| **CLI ↔ runtime version drift** | Runtime's `specVersion` / `schemaVersion` differs from CLI's embedded expectations in a way that breaks compatibility | fatal → exit `10` |
| **Schema version** | Runtime `schemaVersion` not in the CLI's supported list | fatal → exit `10` |
| **Proxy WASM present** | Optional — checks well-known paths (`/etc/envoy/…`, `$WASM_PATH`). Missing → warning only | non-fatal |
| **Header echo** | Optional — smoke-tests `x-softprobe-session-id` through the proxy. Absent → warning only | non-fatal |

### Spec-drift detection

`doctor` compares three version fields when it reaches a runtime:

| Field | Where reported | Semantics |
|---|---|---|
| `cliVersion` | Embedded in the binary (`softprobe --version`) | Changes with every CLI release |
| `runtimeVersion` | Runtime's `/health` payload | Changes with every runtime release |
| `specVersion` | Runtime's `/v1/meta` payload | Changes only on breaking protocol changes |
| `schemaVersion` | Runtime's `/v1/meta` payload | Changes only on breaking schema changes |

Compatibility rule: the CLI embeds the expected `specVersion` and `schemaVersion` constants (`http-control-api@v1`, schema `1`, today). If `/v1/meta` reports different values, `doctor` exits `10` with a `runtime_drift` error — the protocol or schema is not wire-compatible with this binary.

This gives agents and CI a cheap, deterministic way to detect "someone upgraded half the stack" without staring at version strings.

### `--json` output

```json
{
  "status": "ok",
  "exitCode": 0,
  "cliVersion": "v0.5.0 (spec http-control-api@v1)",
  "runtimeVersion": "0.0.0-dev",
  "specVersion": "http-control-api@v1",
  "schemaVersion": "1",
  "checks": [
    {
      "name": "runtime-reachable",
      "status": "ok",
      "details": { "url": "http://127.0.0.1:8080", "latencyMs": 4 }
    },
    {
      "name": "version-drift",
      "status": "ok",
      "details": { "cli": "v0.5.0", "runtime": "0.0.0-dev" }
    },
    {
      "name": "schema-version",
      "status": "ok",
      "details": { "expected": "1", "got": "1" }
    },
    {
      "name": "wasm-binary",
      "status": "warn",
      "details": { "message": "WASM binary not found at /etc/envoy/sp_istio_agent.wasm" }
    }
  ]
}
```

On drift, `status: "drift"` and the specific check fails with `status: "fail"` plus a message explaining the incompatible pair. See [`--json` field stability](#json-field-stability) below.

---

## `softprobe session`

Manage sessions directly (usually wrapped inside tests or suite runs).

### `session start`

```bash
softprobe session start --mode replay --json
# {"sessionId":"sess_01H...","sessionRevision":1}

softprobe session start --mode capture --shell
# export SOFTPROBE_SESSION_ID=sess_01H...
```

| Flag | Values | Default |
|---|---|---|
| `--mode` | `capture`, `replay`, `generate` | (required) |
| `--json` | — | off |
| `--shell` | emit `export` line for eval | off |
| `--policy FILE` | YAML policy to apply | — |
| `--case FILE` | case file to load immediately | — |

### `session load-case`

```bash
softprobe session load-case --session $SESSION_ID --file cases/x.case.json
```

Uploads the case to the runtime (which parses + validates it).

### `session rules apply`

```bash
softprobe session rules apply --session $SESSION_ID --file rules/stripe.yaml
```

Replaces the session's rule document with the contents of the file.

### `session policy set`

```bash
softprobe session policy set --session $SESSION_ID --strict
softprobe session policy set --session $SESSION_ID --file policy.yaml
```

`--strict` is a shortcut for `externalHttp: strict, defaultOnMiss: error`.

### `session close`

```bash
softprobe session close --session $SESSION_ID
softprobe session close --session $SESSION_ID --out cases/captured.case.json
```

Closes the session. For capture sessions, optionally redirects the flushed case file path.

### `session stats`

```bash
softprobe session stats --session $SESSION_ID --json
# {"extractedSpans":2,"injectedSpans":4,"sessionRevision":5,"mode":"replay"}
```

---

## `softprobe capture`

One-shot capture orchestration.

### `capture run`

```bash
softprobe capture run \
  --driver "npm run smoke-test" \
  --target http://127.0.0.1:8082 \
  --out cases/checkout.case.json
```

Starts a session, exports `SOFTPROBE_SESSION_ID` for the driver process, runs it, closes the session, and writes the case file.

| Flag | Purpose |
|---|---|
| `--driver CMD` | Shell command to run; receives `SOFTPROBE_SESSION_ID` in env |
| `--target URL` | Base URL for informational purposes (currently unused) |
| `--out PATH` | Where to write the case file |
| `--redact-file PATH` | Redaction rules to apply before the driver runs |
| `--timeout DURATION` | Maximum runtime (e.g. `300s`, default `10m`) |

---

## `softprobe replay`

### `replay run`

```bash
softprobe replay run --session $SESSION_ID --json
# {"sessionId":"sess_...","hits":12,"misses":0}
```

Diagnostic: reports inject statistics for a live session. Usually more useful than most users need; prefer `suite run` for real workflows.

---

## `softprobe suite`

::: tip Ships in this build
`softprobe suite run`, `validate`, and `diff` are implemented. `suite run` spawns a Node sidecar per invocation to resolve `RequestHook` / `MockResponseHook` / `BodyAssertHook` / `HeadersAssertHook` references from your `suite.yaml`, and emits JUnit XML + a standalone HTML report. A worked end-to-end harness — docker-compose + two cases off one capture file (one hook-driven, one YAML-only `source: inline`) — lives at [`e2e/cli-suite-run/`](https://github.com/softprobe/hybrid/tree/main/e2e/cli-suite-run).
:::

Read one or more case files and run them as a test suite.

### `suite run`

The recommended way to regression-test captured traffic at scale. Full walkthrough: [Run a suite at scale](/guides/run-a-suite-at-scale).

```bash
softprobe suite run suites/checkout.suite.yaml \
  --parallel 32 \
  --hooks hooks/checkout.ts \
  --junit out/junit.xml \
  --report out/report.html
```

| Flag | Default | Purpose |
|---|---|---|
| `--runtime-url URL` | `http://127.0.0.1:8080` | Control-plane runtime |
| `--app-url URL` | `$APP_URL` → `http://127.0.0.1:8081` | SUT base URL; prefer the ingress proxy so `x-softprobe-session-id` becomes `tracestate` on egress |
| `--parallel N` | `min(32, cpu*4)` | Concurrent cases |
| `--hooks PATH` | — | Hook file; repeatable. TS accepted on Node 22+ (uses `--experimental-strip-types`); otherwise compile to `.js`/`.mjs` |
| `--junit PATH` | — | Emit JUnit XML |
| `--report PATH` | — | Emit standalone HTML report |
| `--json` | off | Emit JSON envelope on stdout |
| `--filter SUBSTR` | — | Keep cases whose resolved path **or** `name:` contains `SUBSTR` |
| `--fail-fast` | off | Stop on first failure |
| `--env-file PATH` | — | Load `KEY=VALUE` lines into process env before YAML expansion |

Default human output:

```text
suite: <name>
  OK   <path> [<name>] (<ms>)
  FAIL <path> [<name>] (<ms>): <error>
  ...
result: passed=<n> failed=<n> total=<n>
```

The bracketed `[<name>]` surfaces `cases[i].name` so two cases that share the same `path:` (e.g. one capture driving multiple override shapes) stay distinguishable. JSON output carries both `path` and `displayName`; JUnit XML puts the `caseId` in `name` and the path in `classname`.

Exit code: `0` if all cases passed, `20` if any failed.

### `suite validate`

```bash
softprobe suite validate suites/checkout.suite.yaml
```

Parses the YAML, resolves globs, and checks that every referenced hook exists. Does not run any case.

### `suite diff`

```bash
softprobe suite diff \
  --baseline cases/checkout/baseline/*.case.json \
  --current cases/checkout/current/*.case.json
```

Compares two sets of captures for drift. Useful when regenerating cases.

---

## `softprobe inspect`

Read-only inspection commands.

### `inspect case`

```bash
softprobe inspect case cases/checkout.case.json
```

Prints a table of spans: direction, method, URL, status, body size. Add `--json` for machine use.

### `inspect session`

```bash
softprobe inspect session --runtime-url $SOFTPROBE_RUNTIME_URL --session $SESSION_ID
softprobe inspect session --session $SESSION_ID --json
```

Dumps the current session's policy, rules, loaded case summary, and statistics. Uses `GET /v1/sessions/{id}/state` on the runtime when available; older runtimes fall back to stats-only.

---

## `softprobe validate`

Schema validation for any of the supported artifact types.

```bash
softprobe validate case cases/checkout.case.json
softprobe validate rules rules/stripe.yaml
softprobe validate suite suites/checkout.suite.yaml
```

Exit code: `0` on valid, `5` on invalid.

---

## `softprobe generate`

Code generation. The generator is the **default happy path** for Jest; it compiles a case file into an importable session helper so tests never call the runtime's JSON API directly.

### `generate jest-session`

```bash
softprobe generate jest-session \
  --case cases/checkout.case.json \
  --out test/generated/checkout.replay.session.ts
```

Emits a TypeScript module that creates a replay session, loads the case, and registers one `findInCase` + `mockOutbound` pair per unique outbound hop — using only `@softprobe/softprobe-js`. No hand-rolled `fetch` is emitted. See the [generate-jest-session guide](/guides/generate-jest-session) for the full workflow and [design doc §3.2](https://github.com/softprobe/hybrid/blob/main/docs/design.md#32-default-happy-path-replay--jest-codegen-first) for the rationale.

**Flags:**

| Flag | Required | Purpose |
|---|---|---|
| `--case PATH` | yes | Input case file (`*.case.json`) |
| `--out PATH` | yes | Output TypeScript file; convention: `test/generated/<scenario>.replay.session.ts` |
| `--framework jest` | no | Reserved for future frameworks (Vitest, Mocha); `jest` is the default and only value in v0.5 |

**Output conventions:**

- Import specifier is hard-coded to `@softprobe/softprobe-js` (the canonical TS SDK package).
- Case-file path is stored **relative to the generated file** via `path.dirname(__filename)`, so moving the test directory doesn't break the import.
- Rules are deduplicated by `(direction, method, path)` and sorted lexicographically for stable diffs.

**Exit codes:**

| Code | Meaning |
|---|---|
| `0` | Module written successfully |
| `2` | Missing or malformed flags |
| `5` | Case file failed schema validation |

**Regenerate after every capture refresh.** See [generate-jest-session → regeneration workflow](/guides/generate-jest-session#regeneration-workflow).

### `generate test`

```bash
softprobe generate test \
  --case cases/checkout.case.json \
  --framework vitest \
  --out test/checkout.replay.test.ts
```

Emits a **full test file** (not just a session helper) for the chosen framework. Currently supports:

| `--framework` | Status |
|---|---|
| `jest` | beta |
| `vitest` | preview |
| `pytest` | preview |
| `junit` | alpha |

For stable codegen, prefer `generate jest-session` and write the `describe` / `it` wrapper yourself.

---

## `softprobe export`

### `export otlp`

```bash
softprobe export otlp \
  --case 'cases/**/*.case.json' \
  --endpoint http://otel-collector:4318/v1/traces
```

Streams **case file** traces to an OTLP/HTTP JSON endpoint (for example a collector). This is separate from live proxy capture, which posts to **`sp_backend_url`** on the Softprobe runtime; see [Proxy integration posture](https://github.com/softprobe/hybrid/blob/main/docs/proxy-integration-posture.md).

---

## `softprobe scrub`

Redact sensitive fields from an on-disk case file in place (optional `--rules` file; otherwise conservative defaults). Intended for post-capture review workflows.

```bash
softprobe scrub cases/checkout.case.json
softprobe scrub 'cases/**/*.case.json' --rules redactions.yaml
```

---

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `SOFTPROBE_RUNTIME_URL` | `http://127.0.0.1:8080` | Default for `--runtime-url` |
| `SOFTPROBE_API_TOKEN` | — | Bearer token for `Authorization: Bearer <token>` (hosted runtime or token-protected OSS runtime) |
| `SOFTPROBE_SESSION_ID` | — | Read by `--session` flags if set |
| `SOFTPROBE_CAPTURE_CASE_PATH` | `e2e/captured.case.json` | Output path for captured case files; supports `{sessionId}`, `{ts}`, `{mode}` placeholders and `file://` URIs |
| `SOFTPROBE_LOG_LEVEL` | `info` | Runtime log level: `debug`, `info`, `warn`, `error` |
| `NO_COLOR` | off | Disable ANSI color |

Session-creating commands (`session start`, `capture run`) can export `SOFTPROBE_SESSION_ID` via `--shell` so subsequent commands don't need `--session`.

---

## `--json` field stability

CI pipelines and AI agents depend on the JSON shape being stable. The following fields are considered **stable** — renaming, removing, or changing their type requires a major-version bump of the CLI. New fields **may** be added at any time.

### Common envelope

Every `--json` response carries these fields (at the top level or nested under `data` depending on the command). **`softprobe completion`** does not use this envelope — it prints a shell script only.

| Field | Type | Present on | Purpose |
|---|---|---|---|
| `status` | `"ok"` \| `"fail"` \| `"drift"` | structured `--json` commands | Outcome marker |
| `exitCode` | integer | structured `--json` commands | Mirrors the process exit code |
| `error` | object \| null | on failure | `{ "code": "…", "message": "…" }` |

### Per-command stable fields

| Command | Stable fields |
|---|---|
| `doctor --json` | `cliVersion`, `runtimeVersion`, `specVersion`, `schemaVersion`, `checks[]` (each: `name`, `status`, `details?`) |
| `session start --json` | `sessionId`, `sessionRevision`, `mode`, `specVersion`, `schemaVersion` |
| `session load-case --json` | `sessionId`, `sessionRevision`, `caseId`, `traceCount` |
| `session rules apply --json` | `sessionId`, `sessionRevision`, `ruleCount` |
| `session policy set --json` | `sessionId`, `sessionRevision` |
| `session close --json` | `sessionId`, `stats` (containing `extractedSpans`, `injectedSpans`, `strictMisses`), `capturePath?` |
| `session stats --json` | `sessionId`, `sessionRevision`, `stats.*` |
| `inspect case --json` | `caseId`, `version`, `traceCount`, `spanSummary[]` (each: `direction`, `method`, `host`, `path`, `status`) |
| `capture run --json` | `sessionId`, `exitCode` (of wrapped command), `stats`, `capturePath` |
| `replay run --json` | `sessionId`, `exitCode`, `stats` |
| `suite run --json` | `suite`, `total`, `passed`, `failed`, `cases[]` (each: `caseId`, `status`, `durationMs`, `error?`) |
| `suite validate --json` | `suite`, `errors[]` |
| `suite diff --json` | `added[]`, `removed[]` (outbound span signature strings) |
| `generate jest-session --json` | `outputPath`, `rulesEmitted`, `caseId` |
| `generate test --json` | `outputPath`, `framework`, `rulesEmitted`, `caseId` |
| `validate case --json` | `path`, `valid`, `errors[]` |
| `validate rules --json` | `path`, `valid`, `errors[]` |
| `validate suite --json` | `path`, `valid`, `errors[]` |
| `inspect session --json` | `sessionId`, `sessionRevision`, `mode`, `policy?`, `rules?`, `caseSummary`, `stats` |
| `export otlp --json` | `sent`, `failed` |
| `scrub --json` | `files[]` (each: `path`, `replaced`, `error?`, `updatedAtMs?`) |

### Stability contract

- Stable fields are guaranteed **present** when the command succeeds. If a field is optional, its presence depends on context (e.g. `capturePath` only on capture-mode sessions).
- Breaking changes require a CLI major-version bump (`softprobe --version` tracks both CLI and `specVersion`).
- The `specVersion` field in `doctor` and `session start` output lets you detect drift before parsing other fields — if `specVersion` doesn't match your agent's expectations, abort.

The full schemas are published in `spec/schemas/cli-*.response.schema.json`.

---

## Shell integration

### Bash / zsh completion

```bash
softprobe completion bash > /usr/local/etc/bash_completion.d/softprobe
softprobe completion zsh > "${fpath[1]}/_softprobe"
```

### Fish

```bash
softprobe completion fish > ~/.config/fish/completions/softprobe.fish
```

### Session-in-subshell pattern

```bash
(
  eval "$(softprobe session start --mode capture --shell)"
  run-the-traffic
  softprobe session close --session "$SOFTPROBE_SESSION_ID"
)
```

The subshell scope prevents the variable from leaking into your regular shell session.

---

## Versioning

The CLI follows [semver](https://semver.org). Breaking changes to commands or flags require a major version bump. The `doctor` command warns when the CLI and runtime are more than one minor version apart.

```bash
softprobe --version
# softprobe v0.5.0 (spec http-control-api@v1)
```

## See also

- [Suite YAML reference](/reference/suite-yaml)
- [HTTP control API](/reference/http-control-api) — what the CLI calls under the hood
- [Troubleshooting](/guides/troubleshooting)
