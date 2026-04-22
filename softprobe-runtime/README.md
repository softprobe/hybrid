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

## Container image (GHCR)

[`.github/workflows/softprobe-runtime-image.yml`](../.github/workflows/softprobe-runtime-image.yml)
builds this directory into a single image containing **`softprobe-runtime`**
(the HTTP server) and **`softprobe`** (the CLI). **With no command-line
arguments**, the entrypoint starts the server on `SOFTPROBE_LISTEN_ADDR`
(`0.0.0.0:8080`). **With any arguments**, it runs the CLI — e.g.
`docker run … --version`, `docker run … doctor`, `docker run … suite run …`.

Published refs (replace `ORG` with the GitHub org or user that owns the
package, e.g. `softprobe`):

| Ref | When |
|-----|------|
| `ghcr.io/ORG/softprobe-runtime:latest` | tip of `main` |
| `ghcr.io/ORG/softprobe-runtime:sha-<short>` | every pushed commit on `main` / tags |
| `ghcr.io/ORG/softprobe-runtime:<git-tag>` | annotated semver tags matching `v*` |

```bash
docker pull ghcr.io/ORG/softprobe-runtime:latest
docker run --rm ghcr.io/ORG/softprobe-runtime:latest --version
```

## License

Softprobe Source License 1.0 (SPDX `LicenseRef-Softprobe-Source-License-1.0`) — a source-available license derived from the [Functional Source License 1.1](https://fsl.software) with a broader Competing Use clause. You may use, modify, and redistribute this code for your own internal, research, and consulting purposes at any scale. You may **not** use it in any product or service — hosted, on-premises, bundled, or rebranded — that competes with Softprobe's commercial offerings (replay testing, traffic capture, service virtualization, etc.). Each release automatically converts to Apache-2.0 two years after publication, at which point the Competing Use restriction lifts. See [`LICENSE`](./LICENSE), the repo's [`LICENSING.md`](../LICENSING.md), and the [FAQ licensing section](https://softprobe.dev/faq#licensing).

For commercial licensing outside the Softprobe Source License grant: `sales@softprobe.io`.
