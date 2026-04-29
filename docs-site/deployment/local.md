# Local proxy stack (Docker Compose)

This page shows how to run your app, a sample upstream, and the Softprobe Envoy
proxy on a laptop or CI runner while using the hosted runtime at
`https://runtime.softprobe.dev`. There is no local runtime container in the
official setup.

## What you get

Four services on one Docker network:

| Service | Image | Port | Role |
|---|---|---|---|
| `softprobe-proxy` | `envoyproxy/envoy:v1.27` + WASM | 8082 (ingress), 8084 (egress) | Data plane |
| `app` | your SUT | 8081 | Application |
| `upstream` | your dependency | 8083 | HTTP dependency |
| `test-runner` | your test image | — | Sanity check |

The proxy calls the hosted runtime for `/v1/inject` and `/v1/traces`. CLI and
SDK calls also use the hosted runtime.

## Environment

```bash
export SOFTPROBE_API_TOKEN=...     # from https://dashboard.softprobe.ai
unset SOFTPROBE_RUNTIME_URL        # default is https://runtime.softprobe.dev
```

## `docker-compose.yaml`

```yaml
services:
  softprobe-proxy:
    image: envoyproxy/envoy:v1.27-latest
    command: ["envoy", "-c", "/etc/envoy/envoy.yaml"]
    environment:
      SOFTPROBE_API_TOKEN: ${SOFTPROBE_API_TOKEN}
    volumes:
      - ./envoy.yaml:/etc/envoy/envoy.yaml
      - ./sp_istio_agent.wasm:/etc/envoy/sp_istio_agent.wasm
    ports:
      - "8082:8082"   # ingress listener
      - "8084:8084"   # egress listener
      - "18001:18001" # Envoy admin

  app:
    image: your-org/your-sut:latest
    environment:
      EGRESS_PROXY_URL: http://softprobe-proxy:8084
    ports:
      - "8081:8081"
    depends_on:
      - softprobe-proxy
      - upstream

  upstream:
    image: your-org/your-dependency:latest
    ports:
      - "8083:8083"
```

## `envoy.yaml`

One Envoy config with two HTTP listeners: ingress toward the app, egress toward
dependencies. The Softprobe WASM filter sits on both chains and sends OTLP calls
to `https://runtime.softprobe.dev`.

```yaml
static_resources:
  listeners:
    - name: ingress
      address:
        socket_address: { address: 0.0.0.0, port_value: 8082 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress
                route_config:
                  virtual_hosts:
                    - name: default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: app }
                http_filters:
                  - name: envoy.filters.http.wasm
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      config:
                        name: softprobe
                        configuration:
                          "@type": type.googleapis.com/google.protobuf.StringValue
                          value: |
                            {
                              "sp_backend_url":"https://runtime.softprobe.dev",
                              "public_key":"<your-api-token>",
                              "direction":"ingress"
                            }
                        vm_config:
                          runtime: envoy.wasm.runtime.v8
                          code: { local: { filename: /etc/envoy/sp_istio_agent.wasm } }
                  - name: envoy.filters.http.router

    - name: egress
      address:
        socket_address: { address: 0.0.0.0, port_value: 8084 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: egress
                route_config:
                  virtual_hosts:
                    - name: default
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: upstream }
                http_filters:
                  - name: envoy.filters.http.wasm
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      config:
                        name: softprobe
                        configuration:
                          "@type": type.googleapis.com/google.protobuf.StringValue
                          value: |
                            {
                              "sp_backend_url":"https://runtime.softprobe.dev",
                              "public_key":"<your-api-token>",
                              "direction":"egress"
                            }
                        vm_config:
                          runtime: envoy.wasm.runtime.v8
                          code: { local: { filename: /etc/envoy/sp_istio_agent.wasm } }
                  - name: envoy.filters.http.router

  clusters:
    - name: app
      type: STRICT_DNS
      load_assignment:
        cluster_name: app
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: app, port_value: 8081 } } }

    - name: upstream
      type: STRICT_DNS
      load_assignment:
        cluster_name: upstream
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: upstream, port_value: 8083 } } }

admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 18001 }
```

## Bring it up

```bash
WASM_VERSION=$(curl -fsSL https://storage.googleapis.com/softprobe-published-files/agent/proxy-wasm/version)
curl -fsSL "https://storage.googleapis.com/softprobe-published-files/agent/proxy-wasm/${WASM_VERSION}/sp_istio_agent.wasm" \
  -o sp_istio_agent.wasm

docker compose up -d --wait
softprobe doctor
```

Expected:

```text
✓ runtime reachable at https://runtime.softprobe.dev
✓ authenticated as <your-org>
```

## Running tests

```bash
APP_URL=http://127.0.0.1:8082 npm test
```

The test client sends `x-softprobe-session-id` to `127.0.0.1:8082` (the ingress
listener), not to `:8081` directly.

## Tearing down

```bash
docker compose down
```

Session state is hosted. Close sessions with the CLI or SDK during test cleanup:

```bash
softprobe session close --session "$SOFTPROBE_SESSION_ID" --out cases/captured.case.json
```

## Next

- [Kubernetes deployment](/deployment/kubernetes) — run the proxy with Istio.
- [Hosted runtime](/deployment/hosted) — account, token, and hosted runtime behavior.
- [Troubleshooting](/guides/troubleshooting) — when `docker compose up` doesn't go smoothly.
