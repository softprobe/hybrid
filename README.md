# softprobe/hybrid

Monorepo workspace for the **Softprobe Hybrid** platform: a **proxy-first HTTP**
capture/replay system with a **unified runtime**, **OTLP-shaped case files**, and
language SDKs that steer replay through the same JSON control API.

## Canonical product

Softprobe's current product shape is:

- **`softprobe-proxy`**: Envoy + WASM data plane that intercepts ingress and
  egress HTTP and talks to the runtime over OTLP (`/v1/inject`, `/v1/traces`)
- **`softprobe-runtime`**: unified Go service that serves both the **JSON
  control API** and the **proxy OTLP API** from one process and one in-memory
  session store
- **Language SDKs**: TypeScript, Go, Python, and Java helpers that create
  sessions, load cases, look up captured spans in memory, and register explicit
  mock rules
- **Case artifacts**: `*.case.json` files containing OTLP-compatible trace
  payloads

In local and self-hosted setups, `SOFTPROBE_RUNTIME_URL` and the proxy's
`sp_backend_url` point at the **same** `softprobe-runtime` base URL.

## Workspace layout

| Path | Responsibility |
|------|----------------|
| [`spec/`](./spec/) | Canonical schemas, protocol definitions, and example artifacts |
| [`softprobe-runtime/`](./softprobe-runtime/) | Unified runtime and canonical `softprobe` CLI source |
| [`softprobe-proxy/`](./softprobe-proxy/) | Envoy/WASM data plane |
| [`softprobe-js/`](./softprobe-js/) | TypeScript SDK and migration-era Node package surfaces |
| [`softprobe-go/`](./softprobe-go/) | Go SDK |
| [`softprobe-python/`](./softprobe-python/) | Python SDK |
| [`softprobe-java/`](./softprobe-java/) | Java SDK |
| [`e2e/`](./e2e/) | End-to-end compose stack and cross-SDK replay harnesses |
| [`docs-site/`](./docs-site/) | User-facing documentation site sources |

## Source of truth

- Product and engineering design: [`docs/design.md`](./docs/design.md)
- Platform architecture: [`docs/platform-architecture.md`](./docs/platform-architecture.md)
- Repo topology and responsibilities: [`docs/repo-layout.md`](./docs/repo-layout.md)
- Control API contract: [`spec/protocol/http-control-api.md`](./spec/protocol/http-control-api.md)
- Proxy OTLP contract: [`spec/protocol/proxy-otel-api.md`](./spec/protocol/proxy-otel-api.md)
- Session header propagation: [`spec/protocol/session-headers.md`](./spec/protocol/session-headers.md)

## Local development quick start

Bring up the reference stack:

```bash
docker compose -f e2e/docker-compose.yaml up --build --wait
```

The compose harness publishes:

- runtime: `http://127.0.0.1:8080`
- proxy ingress: `http://127.0.0.1:8082`
- app: `http://127.0.0.1:8081`
- upstream: `http://127.0.0.1:8083`

From there:

- run the runtime CLI and control-plane tests from [`softprobe-runtime/`](./softprobe-runtime/)
- run SDK-specific tests from the language package directories
- run end-to-end replay flows from [`e2e/`](./e2e/)

## Legacy note

This workspace still contains older Node-specific NDJSON/framework-instrumentation
surfaces and earlier proxy/dashboard positioning in some packages. Those remain
for migration and compatibility work, but they are **not** the canonical product
direction. The hybrid proxy-first runtime described in [`docs/design.md`](./docs/design.md)
is the source of truth.

## Contributing

Follow [`AGENTS.md`](./AGENTS.md) and [`tasks.md`](./tasks.md). Prefer extending
or correcting the contracts in `spec/` before making incompatible runtime,
proxy, or SDK changes.

## License

Softprobe uses a **dual-license split** — see [`LICENSING.md`](./LICENSING.md)
for the full path map and plain-English summary.

- **Server-side** (`softprobe-runtime/`, `softprobe-proxy/`, the `softprobe`
  CLI): [Softprobe Source License 1.0](./LICENSE) (SPDX
  `LicenseRef-Softprobe-Source-License-1.0`), a source-available license
  derived from the [Functional Source License 1.1](https://fsl.software)
  with a broader Competing Use clause. Free for your own internal,
  research, and consulting use at any scale; restricted from being used
  in any product or service — hosted, on-premises, bundled, or rebranded
  — that competes with Softprobe. Every release auto-re-licenses to
  Apache-2.0 two years after its publication date and the Competing Use
  restriction lifts at that point.
- **Client SDKs and schemas** (`softprobe-js/`, `softprobe-python/`,
  `softprobe-java/`, `softprobe-go/`, `spec/`): plain
  [Apache License, Version 2.0](./softprobe-js/LICENSE). Embed them in
  proprietary commercial products with no additional restrictions.

For commercial licensing (hosting, OEM, redistribution outside the
Softprobe Source License grant): `sales@softprobe.io`.
