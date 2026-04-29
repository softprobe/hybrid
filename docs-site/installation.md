# Installation

Softprobe has three installable pieces. The runtime is hosted at
`https://runtime.softprobe.dev`; there is no runtime installation step in the
official path.

| Piece | Install for… | Ships as |
|---|---|---|
| **CLI** (`softprobe`) | Scripting capture, running suites, CI | curl-install, direct download |
| **Proxy** (Envoy + Softprobe WASM) | Intercepting HTTP in your environment | WASM binary + Envoy config |
| **SDK** (one per language) | Authoring tests | npm, PyPI, Maven Central, Go modules |

A typical laptop-dev setup uses the CLI, one SDK, and a local Envoy proxy next
to your app. A typical CI setup uses the same hosted runtime, with
`SOFTPROBE_API_TOKEN` stored as a secret.

## Hosted runtime

Create an API token in [dashboard.softprobe.ai](https://dashboard.softprobe.ai)
and export it before using the CLI, SDKs, or proxy:

```bash
export SOFTPROBE_API_TOKEN=...
```

`SOFTPROBE_RUNTIME_URL` defaults to `https://runtime.softprobe.dev`; leave it
unset unless Softprobe support asks you to point at a different hosted endpoint.

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

`public_key` is your API token. `sp_backend_url` defaults to `https://runtime.softprobe.dev`; omit it in the official hosted setup.

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
TypeScript (`@softprobe/softprobe-js`) and Go (`github.com/softprobe/softprobe-go`) are published. Python and Java coordinates below are still planned releases.
:::

### TypeScript / JavaScript

```bash
npm install --save-dev @softprobe/softprobe-js
```

```ts
import { Softprobe } from '@softprobe/softprobe-js';

const softprobe = new Softprobe();  // reads SOFTPROBE_API_TOKEN; uses https://runtime.softprobe.dev
```

Reference: [TypeScript SDK](/reference/sdk-typescript).

### Python

```bash
# Planned PyPI release — not yet published from this repo.
pip install softprobe
```

```python
from softprobe import Softprobe

softprobe = Softprobe()  # reads SOFTPROBE_API_TOKEN; uses https://runtime.softprobe.dev
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
| Hosted runtime | v0.5.0 | v0.5.0 | v0.5.0 |
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
