# Installation

Softprobe has four installable pieces. You pick the ones you need:

| Piece | Install for… | Ships as |
|---|---|---|
| **Runtime** (`softprobe-runtime`) | Hosting the control plane yourself | Docker image + static binary |
| **CLI** (`softprobe`) | Scripting capture, running suites, CI | curl-install, direct download |
| **Proxy** (Envoy + Softprobe WASM) | Intercepting HTTP in your environment | WASM binary + Envoy config |
| **SDK** (one per language) | Authoring tests | npm, PyPI, Maven Central, Go modules |

A typical laptop-dev setup uses all four via `docker compose` and a local `npm install`. A typical CI setup uses the Docker image + the CLI binary + the SDK for the test language.

## Runtime

::: tip Use the hosted runtime (recommended)
Skip this section entirely. Point your CLI and SDKs at `https://runtime.softprobe.dev` — no Docker, no binary to manage. See [Hosted deployment](/deployment/hosted) for a five-minute setup guide.
:::

**Self-hosting** the runtime makes sense when you need no internet dependency, a fully air-gapped environment, or want to run the runtime inside your own Kubernetes cluster. The three options below are for that case.

### Docker

```bash
docker run -p 8080:8080 ghcr.io/softprobe/softprobe-runtime:v0.5
```

The image is ~30 MB (`distroless` base) and starts in under a second. The only required environment variable is `SOFTPROBE_LISTEN_ADDR` (defaults to `0.0.0.0:8080`).

| Variable | Default | Purpose |
|---|---|---|
| `SOFTPROBE_LISTEN_ADDR` | `0.0.0.0:8080` | HTTP bind address for both control API and OTLP |
| `SOFTPROBE_CAPTURE_CASE_PATH` | *(unset)* | Where to flush captured cases on session close |
| `SOFTPROBE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

### Source (Go 1.22+)

```bash
git clone https://github.com/softprobe/softprobe-runtime
cd softprobe-runtime && go build -o softprobe-runtime .
./softprobe-runtime
```

## CLI

The CLI is the primary interface for humans, CI pipelines, and AI agents. It speaks only HTTP to the runtime — no local state.

### Direct download

Download the binary for your platform from the [GitHub Releases](https://github.com/softprobe/softprobe/releases) page and place it on your PATH.

### Curl installer (Linux / macOS)

```bash
curl -fsSL https://docs.softprobe.dev/install/cli.sh | sh
```

Installs `/usr/local/bin/softprobe`.

### Verify

```bash
softprobe --version
# softprobe v0.5.0 (spec http-control-api@v1)

softprobe doctor
# ✓ runtime reachable at https://runtime.softprobe.dev
# ✓ schema version matches CLI (spec v1)
# ✓ proxy WASM binary at expected path
```

If `doctor` reports red, fix the flagged item before continuing.

## Proxy

The proxy is an **Envoy** binary with the **Softprobe WASM filter** loaded. You can run it directly or through a service mesh.

### Local Docker Compose (easiest)

Copy [`e2e/docker-compose.yaml`](https://github.com/softprobe/softprobe/blob/main/e2e/docker-compose.yaml) and [`e2e/envoy.yaml`](https://github.com/softprobe/softprobe/blob/main/e2e/envoy.yaml) from the main repository. They wire a single Envoy with one ingress listener and one egress listener, both pointing at the runtime.

```bash
docker compose -f e2e/docker-compose.yaml up --wait
```

### Standalone Envoy

If you already run Envoy, add the Softprobe WASM filter to your HTTP filter chain:

```yaml
http_filters:
  - name: envoy.filters.http.wasm
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
      config:
        name: softprobe
        configuration:
          "@type": type.googleapis.com/google.protobuf.StringValue
          value: |
            {"public_key":"<your-api-token>"}
        vm_config:
          runtime: envoy.wasm.runtime.v8
          code:
            local: { filename: /etc/envoy/sp_istio_agent.wasm }
```

`public_key` is your API token. `sp_backend_url` defaults to `https://runtime.softprobe.dev` — omit it unless you are self-hosting the runtime.

Download the WASM binary:

```bash
curl -fsSL https://storage.googleapis.com/softprobe-published-files/agent/proxy-wasm/$(curl -fsSL https://storage.googleapis.com/softprobe-published-files/agent/proxy-wasm/version)/sp_istio_agent.wasm \
  -o sp_istio_agent.wasm
```

### Istio

In a mesh, attach the filter to your workload with a `WasmPlugin` resource. See [Kubernetes deployment](/deployment/kubernetes) for the full manifest.

## SDKs

Install the SDK for the language your tests are written in. You can install more than one if your services span languages.

::: warning Publication status
Only the TypeScript SDK (`@softprobe/softprobe-js`) has historical releases on a public registry. **Python, Java, and Go SDKs are not yet released** from this repository — the `pip install`, `go get`, and Maven coordinates below refer to **planned** releases. Today, consume Python / Java / Go SDKs directly from source in the [softprobe monorepo](https://github.com/softprobe/softprobe) (see each package's `README.md`).
:::

### TypeScript / JavaScript

```bash
npm install --save-dev @softprobe/softprobe-js
```

```ts
import { Softprobe } from '@softprobe/softprobe-js';

const softprobe = new Softprobe();  // reads SOFTPROBE_API_TOKEN; defaults to https://runtime.softprobe.dev
```

Reference: [TypeScript SDK](/reference/sdk-typescript).

### Python

```bash
# Planned PyPI release — not yet published from this repo.
pip install softprobe
```

```python
from softprobe import Softprobe

softprobe = Softprobe()  # reads SOFTPROBE_RUNTIME_URL; defaults to https://runtime.softprobe.dev
```

Reference: [Python SDK](/reference/sdk-python).

### Java (Maven)

```xml
<!-- Planned Maven Central release — not yet published from this repo. -->
<dependency>
  <groupId>dev.softprobe</groupId>
  <artifactId>softprobe-java</artifactId>
  <version>0.5.0</version>
  <scope>test</scope>
</dependency>
```

Reference: [Java SDK](/reference/sdk-java).

### Go

```bash
# Planned Go module release — not yet published from this repo.
go get github.com/softprobe/softprobe-go@v0.5.0
```

```go
import "github.com/softprobe/softprobe-go/softprobe"
```

Reference: [Go SDK](/reference/sdk-go).

## One-liner: hosted setup (recommended)

```bash
curl -fsSL https://docs.softprobe.dev/install/cli.sh | sh
export SOFTPROBE_API_TOKEN=...   # from https://dashboard.softprobe.ai
softprobe doctor
```

After that, follow the [Quick start](/quickstart).

## Version matrix

| Component | Current release | Minimum compatible CLI | Minimum compatible SDK |
|---|---|---|---|
| Runtime | v0.5.0 | v0.5.0 | v0.5.0 |
| CLI | v0.5.0 | — | v0.4.0+ |
| Proxy WASM | v0.5.0 | v0.5.0 | — |
| Spec | v1 | v0.5.0 | v0.4.0+ |

`softprobe doctor` warns when versions drift out of range.

## Uninstall

```bash
# curl install
sudo rm /usr/local/bin/softprobe
```

Case files, if any, remain — they are plain JSON you own.

---

**Next:** [Quick start →](/quickstart) or [Architecture →](/concepts/architecture)
