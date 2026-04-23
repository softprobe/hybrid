# Local deployment (Docker Compose)

The fastest way to run the full Softprobe stack on a developer laptop or a CI runner. Based on the reference [`e2e/docker-compose.yaml`](https://github.com/softprobe/softprobe/blob/main/e2e/docker-compose.yaml).

## What you get

Five services on one Docker network:

| Service | Image | Port | Role |
|---|---|---|---|
| `softprobe-runtime` | `ghcr.io/softprobe/softprobe-runtime` | 8080 | Control + OTLP |
| `softprobe-proxy` | `envoyproxy/envoy:v1.27` + WASM | 8082 (ingress), 8084 (egress) | Data plane |
| `app` | your SUT | 8081 | Application |
| `upstream` | your dependency | 8083 | HTTP dependency |
| `test-runner` | your test image | — | Sanity check |

## `docker-compose.yaml`

```yaml
services:
  softprobe-runtime:
    image: ghcr.io/softprobe/softprobe-runtime:v0.5
    environment:
      SOFTPROBE_LISTEN_ADDR: 0.0.0.0:8080
      SOFTPROBE_CAPTURE_CASE_PATH: /cases/captured.case.json
    volumes:
      - ./cases:/cases
    ports:
      - "8080:8080"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 5s
      retries: 20

  softprobe-proxy:
    image: envoyproxy/envoy:v1.27-latest
    command: ["envoy", "-c", "/etc/envoy/envoy.yaml"]
    volumes:
      - ./envoy.yaml:/etc/envoy/envoy.yaml
      - ./sp_istio_agent.wasm:/etc/envoy/sp_istio_agent.wasm
    ports:
      - "8082:8082"   # ingress listener
      - "8084:8084"   # egress listener
      - "18001:18001" # Envoy admin
    depends_on:
      softprobe-runtime:
        condition: service_healthy

  app:
    image: your-org/your-sut:latest
    environment:
      EGRESS_PROXY_URL: http://softprobe-proxy:8084
    ports:
      - "8081:8081"
    depends_on:
      softprobe-proxy:
        condition: service_started
      upstream:
        condition: service_healthy

  upstream:
    image: your-org/your-dependency:latest
    ports:
      - "8083:8083"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8083/health"]
      interval: 5s
      retries: 20
```

## `envoy.yaml`

One Envoy config with two HTTP listeners — ingress toward the app, egress toward dependencies. The Softprobe WASM filter sits on both chains.

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
                            {"sp_backend_url":"http://softprobe-runtime:8080","direction":"ingress"}
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
                            {"sp_backend_url":"http://softprobe-runtime:8080","direction":"egress"}
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
# Download the WASM binary
curl -L -o sp_istio_agent.wasm \
  https://github.com/softprobe/softprobe-proxy/releases/download/v0.5.0/sp_istio_agent.wasm

# Start the stack
docker compose up -d --wait

# Verify
softprobe doctor --runtime-url http://127.0.0.1:8080
```

Expected:
```
✓ runtime reachable at http://127.0.0.1:8080 (v0.5.0)
✓ proxy healthy at http://127.0.0.1:8082
✓ app reachable via proxy
```

## Running your tests against it

```bash
SOFTPROBE_RUNTIME_URL=http://127.0.0.1:8080 \
APP_URL=http://127.0.0.1:8082 \
npm test
```

The test client sends `x-softprobe-session-id` to `127.0.0.1:8082` (the ingress listener), not to `:8081` directly.

## Tearing down

```bash
docker compose down
```

To reset state between runs without restarting the stack:

```bash
curl -X POST http://127.0.0.1:8080/v1/admin/reset
```

(Admin endpoint; requires `SOFTPROBE_ADMIN_TOKEN` in v0.6+.)

## Watching logs

```bash
docker compose logs -f softprobe-runtime softprobe-proxy
```

## Persisting captures between runs

Case files are written to the runtime's mounted `./cases/` directory. By default this is preserved across `docker compose restart` because it's a bind mount.

For a per-session path:

```bash
softprobe session close --session $SOFTPROBE_SESSION_ID --out cases/checkout-$(date +%Y%m%d).case.json
```

## CI variant

For GitHub Actions, run the runtime as a service container and skip the proxy (if your app can hit a stubbed upstream directly):

```yaml
services:
  softprobe-runtime:
    image: ghcr.io/softprobe/softprobe-runtime:v0.5
    ports:
      - 8080:8080
    options: >-
      --health-cmd "wget -qO- http://127.0.0.1:8080/health"
      --health-interval 5s
```

For tests that require the proxy (end-to-end capture/replay with a real SUT), use `docker compose` inside the workflow step:

```yaml
- run: docker compose -f e2e/docker-compose.yaml up -d --wait
- run: npm test
```

See [CI integration](/guides/ci-integration) for full workflows.

## Customizing

- **Different upstream host:** update the `upstream` cluster in `envoy.yaml` and the `app` service's env.
- **Two upstreams:** add a second cluster and a second route match (`/api/stripe/*` → stripe, `/api/auth/*` → auth).
- **Auth headers on ingress:** add an `envoy.filters.http.lua` filter before the WASM filter.
- **TLS:** bind the proxy to a TLS listener instead of plain HTTP; point the app at the HTTPS URL.

## Next

- [Kubernetes deployment](/deployment/kubernetes) — production-grade with Istio.
- [Hosted deployment](/deployment/hosted) — `runtime.softprobe.dev`.
- [Troubleshooting](/guides/troubleshooting) — when `docker compose up` doesn't go smoothly.
