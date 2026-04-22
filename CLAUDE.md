# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

**Softprobe Hybrid** is a proxy-first HTTP capture/replay testing platform. Tests record real HTTP interactions as JSON "case files" and replay them deterministically without hitting live dependencies.

Canonical request flow:
```
test client → Envoy proxy → app → Envoy proxy → upstream dependency
                   ↓                     ↓
             softprobe-runtime (control API + OTLP backend)
```

Read `docs/design.md` before starting any task. Do not hallucinate scope outside what is written there.

## Agent Operating Rules (from AGENTS.md)

This repo uses a strictly sequentially-gated TDD workflow:

1. Read `tasks.md` before taking any action. Work only on the first `[ ]` task.
2. **TDD mandate:** Write a failing test (Red) → write minimal implementation (Green) → refactor. Never write implementation before a failing test.
3. When a task is Green, mark it `[x]` in `tasks.md` and continue to the next `[ ]` immediately.
4. **Zero scope creep:** If a task asks for an interface, only write the interface. Never add unauthorized features, fallbacks, or abstractions.
5. Fail fast and loud — no silent defaults or fallbacks that mask errors.
6. `tasks.md` is the source of truth for current work state.

## Build and Test Commands

### softprobe-runtime (Go — primary server + CLI)
```bash
cd softprobe-runtime
go build .                          # runtime server
go build ./cmd/softprobe            # CLI binary
go test ./...                       # all unit tests
go test -v ./internal/controlapi/...
go test -v ./internal/proxybackend/...
go test -v ./internal/store/...
```

### softprobe-proxy (Rust / WASM)
```bash
cd softprobe-proxy
rustup target add wasm32-unknown-unknown   # one-time setup
make build                                 # build WASM module
make integration-test                      # Docker Compose + Envoy
cargo build --features non-wasm           # composition tests (native target)
```

### softprobe-js (TypeScript)
```bash
cd softprobe-js
npm install && npm run build
npm test              # unit tests
npm run test:e2e      # e2e tests only
```

### softprobe-go (Go SDK)
```bash
cd softprobe-go && go test ./...
```

### softprobe-python (Python SDK)
```bash
cd softprobe-python && python3 -m unittest discover -s tests
```

### softprobe-java (Java SDK)
```bash
cd softprobe-java && mvn test
mvn -q install -DskipTests   # fast local install for e2e deps
```

### End-to-end tests (requires WASM built first)
```bash
docker compose -f e2e/docker-compose.yaml up --build --wait

# Go e2e (capture + replay + strict policy)
cd e2e && RUNTIME_URL=http://127.0.0.1:8080 PROXY_URL=http://127.0.0.1:8082 \
  APP_URL=http://127.0.0.1:8081 UPSTREAM_URL=http://127.0.0.1:8083 \
  go test -count=1 ./go/...

# TypeScript/Jest
cd e2e/jest-replay && npm install && npm test
cd e2e/jest-hooks && npm install && npm test

# Python/pytest
python3 -m pytest e2e/pytest-replay/ -v

# Java/JUnit (requires softprobe-java installed locally first)
( cd softprobe-java && mvn -q install -DskipTests )
cd e2e/junit-replay && mvn test
```

### Spec validation
```bash
bash spec/scripts/validate-spec.sh
```

## Architecture

### Components

| Directory | Language | Role |
|---|---|---|
| `spec/` | JSON Schema / docs | Canonical schemas and protocol contracts — everything must conform |
| `softprobe-runtime/` | Go | HTTP server + CLI; serves both control API and OTLP proxy API |
| `softprobe-proxy/` | Rust (WASM) | Envoy filter; intercepts HTTP, calls runtime over OTLP |
| `softprobe-js/` | TypeScript | Node SDK (`@softprobe/softprobe-js`) |
| `softprobe-go/` | Go | Go SDK |
| `softprobe-python/` | Python | Python SDK |
| `softprobe-java/` | Java | Java SDK (Maven, Java 21, JUnit 5) |
| `e2e/` | Multi-language | Docker Compose integration harness with real Envoy |
| `docs-site/` | VitePress | User-facing docs (docs.softprobe.dev) |

### softprobe-runtime internals

- `internal/store/store.go` — single mutex-protected in-memory session map; holds `LoadedCase`, `Rules`, `Policy`, `FixturesAuth`, `Extracts`, `Stats` per session; every mutation bumps `Revision`
- `internal/controlapi/mux.go` — all JSON control API routes (`/health`, `/v1/meta`, `/v1/sessions`, `/v1/sessions/{id}/load-case|rules|policy|fixtures/auth|close|stats`)
- `internal/proxybackend/` — OTLP handlers (`/v1/inject`, `/v1/traces`) and rule resolver
- `cmd/softprobe/` — canonical CLI binary (doctor, session, inspect, generate, validate, suite, capture, replay, scrub, export, completion)

The Docker image starts the server with no args; any args are routed to the CLI.

### Key architectural invariants

**SDK-side replay:** The runtime never walks `traces[]` on the inject hot path. `findInCase` is purely in-memory in each SDK. The SDK looks up the span, optionally mutates the response, then posts a `then.action: mock` rule to the runtime via `mockOutbound`.

**Rules are full-replace:** `POST .../rules` replaces all session rules. SDKs must accumulate rules locally and post the full merged set on each `mockOutbound` call — not just the new rule.

**Three-layer rule composition (lowest → highest priority):** session policy < case-embedded rules < session rules. Equal priority within a layer: later order wins.

**API surface separation:** Tests call only the JSON control API. The proxy calls only `/v1/inject` and `/v1/traces`. These never cross.

**Session header propagation:** Inbound test requests carry `x-softprobe-session-id`. The proxy folds it into W3C `tracestate`. Apps propagate `traceparent`/`tracestate` on outbound calls via OpenTelemetry — they must NOT manually forward `x-softprobe-session-id` to dependencies.

**`sessionRevision` cache invalidation:** Any proxy-side inject cache must key on `(sessionId, sessionRevision, requestFingerprint)`. Any mutation that bumps `Revision` invalidates prior cached decisions.

**One canonical CLI:** Language repos ship SDKs and optional thin shims (e.g., `npx softprobe`), not competing CLIs with different verb sets.

### JS package layout (enforced by architecture-guard)

`src/core/` (foundation) must not depend on `src/instrumentations/`. Instrumentation packages depend only on `src/core/` and `src/instrumentations/common/`. `src/instrumentations/common/` holds shared protocol helpers. Never add new files to legacy mixed folders when an equivalent location exists in the new structure.

## CLI Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | Generic failure |
| `2` | Invalid args / usage error |
| `3` | Runtime unreachable |
| `4` | Session not found |
| `5` | Validation failure |
| `10` | `doctor` failure |
| `20` | `suite run` failure |

## Key Reference Files

- `docs/design.md` — definitive platform design (read first for any task)
- `AGENTS.md` — agent operating rules (TDD, sequencing, scope constraints)
- `tasks.md` — current task state (source of truth)
- `spec/protocol/http-control-api.md` — control API contract
- `spec/protocol/proxy-otel-api.md` — proxy OTLP contract
- `spec/protocol/session-headers.md` — header propagation contract
- `spec/examples/cases/fragment-happy-path.case.json` — golden case file used by all e2e harnesses
