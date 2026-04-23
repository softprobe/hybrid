# End-to-end harness (Docker Compose)

This directory validates **capture** and **replay** with real **Envoy**, **Softprobe WASM**, and **`softprobe-runtime`**. Envoy is **not** the application under test—it is the **dual interception** layer for **ingress** and **egress** HTTP.

## Canonical topology

```text
client → proxy → app → proxy → upstream
```

| Leg | Meaning |
|-----|---------|
| **client → proxy → app** | **Ingress:** external or test traffic **to** the application (SUT). |
| **app → proxy → upstream** | **Egress:** the application’s HTTP **client** calls **to** a dependency (another service). |

On **each** leg the proxy sees the **request and response** and talks to **`softprobe-runtime`** on **`/v1/inject`** and **`/v1/traces`** (OTLP). Tests use the **JSON control API** on the same runtime (sessions, `load-case`, policy).

In Kubernetes / Istio, **ingress** and **egress** are usually the **same sidecar** process (traffic redirected in both directions). This repo’s compose file uses **one Envoy** with **two listeners** so we can run the same model without iptables:

- **8082** — “outside” traffic **into** the app (routes to **`app:8081`**).
- **8084** — traffic **from** the app **out** to dependencies (routes to **`upstream:8083`**).

The **`app`** process is configured with **`EGRESS_PROXY_URL=http://softprobe-proxy:8084`** so its outbound `GET /fragment` goes **through** Envoy again. Outbound calls use **OpenTelemetry** (`go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`) to propagate **W3C `traceparent` / `tracestate`**—the proxy already embeds session correlation in `tracestate`; the app does **not** copy `x-softprobe-session-id` manually.

## Services in `docker-compose.yaml`

| Service | Role |
|--------|------|
| **`app`** | Minimal **SUT** (Go HTTP on **8081**): `/hello` calls the dependency via **8084**. |
| **`upstream`** | Minimal **HTTP dependency** (Go on **8083**): `/fragment` returns JSON used to build the `/hello` response. |
| **`softprobe-proxy`** | Envoy + WASM; listeners **8082** (ingress) and **8084** (egress). |
| **`softprobe-runtime`** | Control + OTLP API; optional `SOFTPROBE_CAPTURE_CASE_PATH` → `e2e/captured.case.json`. |
| **`test-runner`** | Health checks and one `GET` through **8082**. |

## Environment variables (tests)

- **`RUNTIME_URL`** — `softprobe-runtime` base URL.
- **`PROXY_URL`** — Envoy **ingress** URL (e.g. `http://softprobe-proxy:8082`).
- **`APP_URL`** — Direct **`app`** URL (counters on **`/hello`**).
- **`UPSTREAM_URL`** — Direct **`upstream`** URL (counters on **`/fragment`**).
- **`EGRESS_PROXY_URL`** (optional) — Fallback **egress** listener URL if the case file cannot supply `url.host` for `/fragment` (defaults to `http://127.0.0.1:8084` on the host).

## Running

Requires the WASM binary at `softprobe-proxy/target/wasm32-unknown-unknown/release/sp_istio_agent.wasm`. From repo root:

```bash
docker compose -f e2e/docker-compose.yaml up --build --wait
```

The stack publishes the runtime on **8080**, the proxy ingress on **8082**, the app on **8081**, the upstream on **8083**, and the egress listener on **8084**. Every harness below runs against this **same** stack with **no per-SDK setup** beyond the normal toolchain (`node` / `python3` / `mvn`).

### Go — `e2e/go/` (runtime integration + SDK fragment replay)

All Go-based e2e code lives under **`e2e/go/`**, alongside the other language harnesses (`jest-replay/`, `pytest-replay/`, …). The **`e2e/go.mod`** module root stays **`e2e/`** so **`app/`**, **`upstream/`**, and **`test-runner/`** keep their existing Docker `working_dir` paths.

Shared helpers (no tests): **`e2e/go/e2etestutil/`**.

```bash
# Go e2e only (capture + runtime integration + go-replay harness)
cd e2e && RUNTIME_URL=http://127.0.0.1:8080 PROXY_URL=http://127.0.0.1:8082 \
  APP_URL=http://127.0.0.1:8081 UPSTREAM_URL=http://127.0.0.1:8083 go test -count=1 ./go/...

# Entire e2e Go module (includes no-op packages like app/ when present)
cd e2e && go test -count=1 ./...
```

- **`e2e/go/go-capture/`** — `TestCaptureFlowProducesValidCaseFile`: drives the proxy in **capture** mode and validates the OTLP-shaped case artifact (valid `traceId` / `spanId` bytes, ingress + egress extract spans). Writes **`e2e/captured.case.json`** (gitignored) next to **`e2e/go.mod`**.
- **`e2e/go/`** (`package main`) — `TestReplayEgressInjectMocksUpstream`: uses `softprobe-go` (`StartSession` → `LoadCaseFromFile` → `FindInCase` → `MockOutbound`) against the captured case, then hits the **egress** listener and asserts the live upstream is never contacted.
- **`e2e/go/`** (`package main`) — `TestStrictPolicyBlocksUnmockedTraffic`: asserts `externalHttp: strict` rejects unmocked outbound traffic with a 5xx.

To run only capture or only the runtime integration pair:

```bash
cd e2e && go test -count=1 ./go/go-capture
cd e2e && go test -count=1 -run 'Replay|Strict' ./go
```

### SDK harnesses (shared fragment replay happy-path)

Four parallel harnesses exercise the same **`findInCase` + `mockOutbound`** authoring flow (see [docs/design.md](../docs/design.md) §3.2) from TypeScript, Python, Java, and Go against the same stack. Each drives `/hello` on **`APP_URL`** with `x-softprobe-session-id` and expects the `/fragment` dependency to come from the **replayed** capture rather than the live upstream.

#### TypeScript / Jest — `e2e/jest-replay/`

```bash
cd e2e/jest-replay && npm install && npm test
```

#### TypeScript / Jest with hooks + `suite.yaml` — `e2e/jest-hooks/`

Drives `runSuite()` from `@softprobe/softprobe-js/suite` against a real
`suite.yaml` that references a `MockResponseHook` by name. Proves the
hook actually runs, the transformed response reaches the runtime, the
proxy serves it as a mock, and the SUT's response carries the
hook-mutated payload (the `upstream` container sees zero hits).

```bash
cd e2e/jest-hooks && npm install && npm test
```

#### Python / pytest — `e2e/pytest-replay/`

No install needed for the harness itself; `conftest.py` puts `softprobe-python/` on `sys.path`. Only `pytest` is required:

```bash
pip install pytest   # one-time
python3 -m pytest e2e/pytest-replay/ -v
```

#### Java / JUnit 5 — `e2e/junit-replay/`

The harness depends on the locally-built `softprobe-java` artifact:

```bash
( cd softprobe-java && mvn -q install -DskipTests )   # one-time per SDK change
cd e2e/junit-replay && mvn test
```

#### Go — `e2e/go/go-replay/`

Same **`e2e`** module as the rest of `e2e/go/`; `softprobe-go` comes from **`replace` in `e2e/go.mod`**.

```bash
cd e2e && go test -count=1 ./go/go-replay/...
```

All four use the same default environment (`SOFTPROBE_RUNTIME_URL=http://127.0.0.1:8080`, `APP_URL=http://127.0.0.1:8081`); override via env vars when running outside the compose defaults.

**Capture file:** `e2e/captured.case.json` is **generated** by `TestCaptureFlowProducesValidCaseFile` in **`e2e/go/go-capture/`** (gitignored). It must contain **both** an ingress `/hello` extract span and an egress `/fragment` extract span, with valid OTLP **`traceId`** (16 bytes), **`spanId`** (8 bytes), and **`parentSpanId`** (8 bytes) on each hop where W3C context is present. Incomplete or stale case files are removed and regenerated by **`e2e/go/e2etestutil.EnsureCapturedCase`** (used by the replay and strict-policy tests).

The four SDK harnesses instead load **`spec/examples/cases/fragment-happy-path.case.json`**, a checked-in golden case, so they do not depend on a prior capture run.

See [docs/design.md](../docs/design.md) §3.2, §3.4 and §5.1.

---

## Running against a target

### Local compose stack (default)

Start the stack once, then run any harness:

```bash
docker compose -f e2e/docker-compose.yaml up --build --wait

# Pick a harness:
cd e2e && go test -count=1 ./go/go-replay/...
cd e2e/jest-replay && npm test
python3 -m pytest e2e/pytest-replay/ -v
cd e2e/junit-replay && mvn test
```

No extra env vars needed — all defaults point at `127.0.0.1:8080` (runtime) and `127.0.0.1:8081` (app).

#### Local compose stack with bearer auth

`e2e/docker-compose.auth.yaml` is a compose override that sets `SOFTPROBE_API_TOKEN=sp_test` on the runtime, exercising the same static-token auth path used by the hosted runtime. Pass `SOFTPROBE_API_KEY=sp_test` to each harness so the clients send the matching bearer token:

```bash
docker compose -f e2e/docker-compose.yaml -f e2e/docker-compose.auth.yaml up --build --wait

export SOFTPROBE_API_KEY=sp_test

cd e2e && RUNTIME_URL=http://127.0.0.1:8080 PROXY_URL=http://127.0.0.1:8082 \
  APP_URL=http://127.0.0.1:8081 UPSTREAM_URL=http://127.0.0.1:8083 \
  go test -count=1 ./go/...
cd e2e/jest-replay && npm test
python3 -m pytest e2e/pytest-replay/ -v
cd e2e/junit-replay && mvn test
```

### Hosted Softprobe runtime (`runtime.softprobe.dev`)

The four SDK replay harnesses and the `hosted/` smoke tests all work against the hosted runtime without any code changes. You only need a runtime URL and an API key.

```bash
export SOFTPROBE_API_TOKEN=<your-api-key>
```

Then run the smoke tests first to verify connectivity:

```bash
cd e2e && go test ./hosted/ -v -count=1
```

Then the SDK replay harnesses (the `app` and `upstream` services must also be accessible — see note below):

```bash
cd e2e && go test -count=1 ./go/go-replay/...
cd e2e/jest-replay && npm test
python3 -m pytest e2e/pytest-replay/ -v
cd e2e/junit-replay && mvn test
```

> **Note on `APP_URL`:** The SDK replay harnesses drive traffic through the `app` service (which must be reachable at `APP_URL`). When running purely against the hosted runtime without a local compose stack, set `APP_URL` to your own deployed test app or leave it unset — the harnesses skip gracefully if the app is unreachable.

The `jest-hooks` harness also requires a running Envoy ingress proxy (`INGRESS_URL`). It is compose-only and skips automatically when the ingress proxy is unreachable.

#### Cloud runtime through local proxy (no local runtime container)

To verify the real proxy path against the cloud deployment, run only `softprobe-proxy`, `app`, and `upstream`; do not start the local `softprobe-runtime` container.

```bash
export SOFTPROBE_RUNTIME_URL=https://softprobe-runtime-1076343766237.us-central1.run.app
export SOFTPROBE_API_KEY=<your-api-key>
export SOFTPROBE_API_TOKEN=$SOFTPROBE_API_KEY

cd e2e
python3 ./scripts/render-envoy-cloud.py
docker compose -f docker-compose.yaml -f docker-compose.cloud.yaml up --build --wait softprobe-proxy upstream app

cd e2e/jest-hooks && npm test
```

`render-envoy-cloud.py` generates `e2e/envoy.cloud.yaml` from `e2e/envoy.cloud.tmpl.yaml` so Envoy/WASM calls `SOFTPROBE_RUNTIME_URL` directly with `SOFTPROBE_API_KEY`.

**What skips vs. what requires the proxy stack:**

| Harness | Requires compose proxy | Works hosted-only |
|---|---|---|
| `hosted/` smoke tests | No | Yes — this is its primary target |
| `go/go-replay/` | No | Yes |
| `jest-replay/` | No | Yes |
| `pytest-replay/` | No | Yes |
| `junit-replay/` | No | Yes |
| `go/` (egress inject, strict policy) | Yes | Skips without proxy |
| `jest-hooks/` | Yes | Skips without ingress proxy |

### Self-hosted runtime (custom URL)

Point at any runtime by setting `SOFTPROBE_RUNTIME_URL`. If your runtime requires a static bearer token (OSS `SOFTPROBE_API_TOKEN` env var on the server side), set the matching client token:

```bash
export SOFTPROBE_RUNTIME_URL=http://my-runtime:8080
export SOFTPROBE_API_TOKEN=my-static-token
cd e2e && go test -count=1 ./go/go-replay/...
```
