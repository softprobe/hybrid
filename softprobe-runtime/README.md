# softprobe-runtime

`softprobe-runtime` is the OSS **unified runtime** for the Softprobe hybrid
platform.

It serves both API surfaces from one process and one in-memory session store:

- **JSON control API** for SDKs, tests, and the CLI
- **OTLP proxy API** for the Envoy/WASM data plane

In local and self-hosted setups, both `SOFTPROBE_RUNTIME_URL` and the proxy's
`sp_backend_url` should point at this same base URL.

## Default address

- `127.0.0.1:8080`

## Endpoints

### Control API

- `GET /health`
- `POST /v1/sessions`
- `POST /v1/sessions/{sessionId}/load-case`
- `POST /v1/sessions/{sessionId}/policy`
- `POST /v1/sessions/{sessionId}/rules`
- `POST /v1/sessions/{sessionId}/fixtures/auth`
- `POST /v1/sessions/{sessionId}/close`

### Proxy OTLP API

- `POST /v1/inject`
- `POST /v1/traces`

## Canonical local setup

For the OSS reference layout:

- CLI / SDKs use `SOFTPROBE_RUNTIME_URL=http://127.0.0.1:8080`
- the proxy uses `sp_backend_url=http://softprobe-runtime:8080` inside Docker
  Compose or the same runtime base URL in other local/self-hosted deployments

No second local "backend" service is required.

## Current CLI surface

This repo also contains the canonical `softprobe` CLI source under
[`cmd/softprobe`](./cmd/softprobe).

The currently wired commands are:

- `softprobe --version`
- `softprobe doctor --runtime-url http://127.0.0.1:8080`
- `softprobe inspect case <file>`
- `softprobe session start`
- `softprobe session load-case`
- `softprobe session rules apply`
- `softprobe session policy set`

Exit codes:

- `0` on success
- `1` on runtime/API failures
- `2` on usage or flag parsing errors

Later CLI orchestration work should stay aligned with [`tasks.md`](../tasks.md)
and the design in [`docs/design.md`](../docs/design.md).

## Related docs

- [Hybrid platform design](../docs/design.md)
- [Platform architecture](../docs/platform-architecture.md)
- [Repo layout](../docs/repo-layout.md)
- [HTTP control API](../spec/protocol/http-control-api.md)
- [Proxy OTLP API](../spec/protocol/proxy-otel-api.md)
- [Proxy deployment guide](../softprobe-proxy/docs/deployment.md)

An informative Kubernetes example lives in
[`deploy/kubernetes.yaml`](./deploy/kubernetes.yaml).
