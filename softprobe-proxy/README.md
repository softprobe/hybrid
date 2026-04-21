# softprobe-proxy

`softprobe-proxy` is the **proxy data plane** for the Softprobe hybrid platform.
It is an Envoy WebAssembly (WASM) extension written in Rust.

Its job is narrowly defined:

- intercept ingress and egress HTTP
- propagate session correlation through W3C trace context
- call the runtime's OTLP endpoints (`/v1/inject`, `/v1/traces`)
- enforce inject hit/miss behavior on the request path

It is **not** a separate product control plane and it is **not** the source of
truth for replay policy. Policy and rules live in `softprobe-runtime`.

## How it fits into the platform

```text
client -> proxy -> app -> proxy -> upstream
              \
               \-> softprobe-runtime (/v1/inject, /v1/traces)
```

In the OSS reference layout:

- the runtime serves both the JSON control API and the OTLP proxy API
- `sp_backend_url` points at the same base URL as `SOFTPROBE_RUNTIME_URL`

See:

- [`docs/design.md`](../docs/design.md)
- [`spec/protocol/proxy-otel-api.md`](../spec/protocol/proxy-otel-api.md)
- [`spec/protocol/session-headers.md`](../spec/protocol/session-headers.md)
- [`docs/platform-architecture.md`](../docs/platform-architecture.md)

## Local and development workflows

### Build the WASM module

Prerequisites:

- Rust toolchain with `wasm32-unknown-unknown`
- Protocol Buffers compiler

```bash
rustup target add wasm32-unknown-unknown
brew install protobuf
make build
```

### Local compose / Envoy validation

```bash
make integration-test
```

This validates the module and exercises the local Envoy path against the runtime
contract used by the repo's end-to-end harness.

### End-to-end compose stack

From the repo root:

```bash
docker compose -f e2e/docker-compose.yaml up --build --wait
```

That stack runs:

- `softprobe-runtime`
- `softprobe-proxy`
- a sample app
- a sample upstream dependency
- a smoke-test runner

The reference topology and environment are documented in [`e2e/README.md`](../e2e/README.md).

## Istio / Kubernetes workflows

Use the deployment manifests and docs in this repo:

- [`docs/deployment.md`](./docs/deployment.md)
- [`deploy/`](./deploy/)
- [`config/development.yaml`](./config/development.yaml)

Quick local iteration with Kind/Istio:

```bash
make dev-quickstart
make forward
make status
```

Hot reload without tearing down the cluster:

```bash
make dev-reload
```

## Session propagation

For service-to-service HTTP, the proxy writes session correlation into
`tracestate`. Applications should propagate standard W3C `traceparent` and
`tracestate` through OpenTelemetry instrumentation. They should **not** manually
copy `x-softprobe-session-id` onto outbound requests.

See [`spec/protocol/session-headers.md`](../spec/protocol/session-headers.md).

## Legacy note

Older Softprobe materials sometimes described this proxy as an analytics or
dashboard-oriented "agent". That is no longer the canonical product story for
this repo. The proxy's current role is test-time HTTP interception for the
hybrid capture/replay platform.
