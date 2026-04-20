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

### Runtime integration tests — `e2e/` (Go)

These tests exercise the **runtime + proxy** end-to-end: capture, the egress SDK-authoring flow against a live stack, and strict-policy enforcement. They are the reference suite for runtime behavior, not an SDK-authoring example.

```bash
cd e2e && RUNTIME_URL=http://127.0.0.1:8080 PROXY_URL=http://127.0.0.1:8082 \
  APP_URL=http://127.0.0.1:8081 UPSTREAM_URL=http://127.0.0.1:8083 go test -count=1 .
```

- `TestCaptureFlowProducesValidCaseFile` — drives the proxy in **capture** mode and validates the OTLP-shaped case artifact (valid `traceId` / `spanId` bytes, ingress + egress extract spans).
- `TestReplayEgressInjectMocksUpstream` — uses `softprobe-go` (`StartSession` → `LoadCaseFromFile` → `FindInCase` → `MockOutbound`) to register a captured `/fragment` response as a mock rule, then hits the **egress** listener with that session and asserts the live upstream is never contacted.
- `TestStrictPolicyBlocksUnmockedTraffic` — asserts `externalHttp: strict` rejects unmocked outbound traffic with a 5xx.

### SDK harnesses (shared fragment replay happy-path)

Four parallel harnesses exercise the same **`findInCase` + `mockOutbound`** authoring flow (see [docs/design.md](../docs/design.md) §3.2) from TypeScript, Python, Java, and Go against the same stack. Each drives `/hello` on **`APP_URL`** with `x-softprobe-session-id` and expects the `/fragment` dependency to come from the **replayed** capture rather than the live upstream.

#### TypeScript / Jest — `e2e/jest-replay/`

```bash
cd e2e/jest-replay && npm install && npm test
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

#### Go — `e2e/go-replay/`

Consumes `softprobe-go` via a local `replace` directive — nothing to pre-build:

```bash
cd e2e/go-replay && go test -count=1 ./...
```

All four use the same default environment (`SOFTPROBE_RUNTIME_URL=http://127.0.0.1:8080`, `APP_URL=http://127.0.0.1:8081`); override via env vars when running outside the compose defaults.

**Capture file:** `e2e/captured.case.json` is **generated** by `TestCaptureFlowProducesValidCaseFile` (gitignored). It must contain **both** an ingress `/hello` extract span and an egress `/fragment` extract span, with valid OTLP **`traceId`** (16 bytes), **`spanId`** (8 bytes), and **`parentSpanId`** (8 bytes) on each hop where W3C context is present. Incomplete or stale case files are removed and regenerated by `ensureCapturedCase`.

The four SDK harnesses instead load **`spec/examples/cases/fragment-happy-path.case.json`**, a checked-in golden case, so they do not depend on a prior capture run.

See [docs/design.md](../docs/design.md) §3.2, §3.4 and §5.1.
