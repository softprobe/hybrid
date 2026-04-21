# Standalone Envoy (no Istio)

If you don't run a service mesh — or you want to isolate the Softprobe proxy in a single-binary sidecar for testing — you can run **Envoy on its own** with the Softprobe WASM filter. This is the same topology Softprobe's own `e2e/` harness uses.

Use this deployment pattern when:

- You want to evaluate Softprobe before introducing Istio / Linkerd.
- You deploy to VMs or bare metal and the "sidecar" is just another process on the host.
- You want to run the proxy locally outside Docker Compose.

For Kubernetes + Istio, see [Kubernetes deployment](/deployment/kubernetes). For Compose, see [Local deployment](/deployment/local).

## Topology

```text
 test client ─► Envoy :8082 (ingress) ─► app :8081 ─► Envoy :8084 (egress) ─► upstream :8083
                     │                                       │
                     └──────── POST /v1/inject ───────────────┴──► softprobe-runtime :8080
                                                                   POST /v1/traces
```

One Envoy process, two listeners:

| Listener | Port | Role | WASM `traffic_direction` |
|---|---|---|---|
| Ingress | `8082` | Test client → app | `outbound` (from proxy's POV, to app) |
| Egress | `8084` | App → real upstream | `outbound` (to real upstream) |

Both listeners carry the **same** WASM filter configuration, pointing at the same runtime.

## Prerequisites

- Envoy `v1.30` or newer (`envoy --version`).
- The Softprobe WASM binary (`sp_istio_agent.wasm`). Download from [GitHub releases](https://github.com/softprobe/softprobe/releases) or build from source.
- A running `softprobe-runtime` reachable by Envoy — see [Installation](/installation).

## Configuration

Save as `envoy.yaml`:

```yaml
static_resources:
  listeners:
    # ─── Ingress: test client → app ─────────────────────────────────
    - name: ingress
      address:
        socket_address: { address: 0.0.0.0, port_value: 8082 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: ingress_http
                codec_type: AUTO
                route_config:
                  name: ingress_route
                  virtual_hosts:
                    - name: app
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: app }
                http_filters:
                  - name: envoy.filters.http.wasm
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      config:
                        name: softprobe_ingress
                        vm_config:
                          vm_id: softprobe_ingress
                          runtime: envoy.wasm.runtime.v8
                          code:
                            local: { filename: /etc/envoy/sp_istio_agent.wasm }
                        configuration:
                          "@type": type.googleapis.com/google.protobuf.StringValue
                          value: |
                            {
                              "traffic_direction": "inbound",
                              "service_name": "my-app",
                              "sp_backend_url": "http://softprobe-runtime:8080"
                            }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

    # ─── Egress: app → real upstream ───────────────────────────────
    - name: egress
      address:
        socket_address: { address: 0.0.0.0, port_value: 8084 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                stat_prefix: egress_http
                codec_type: AUTO
                route_config:
                  name: egress_route
                  virtual_hosts:
                    - name: upstream
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: upstream }
                http_filters:
                  - name: envoy.filters.http.wasm
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
                      config:
                        name: softprobe_egress
                        vm_config:
                          vm_id: softprobe_egress
                          runtime: envoy.wasm.runtime.v8
                          code:
                            local: { filename: /etc/envoy/sp_istio_agent.wasm }
                        configuration:
                          "@type": type.googleapis.com/google.protobuf.StringValue
                          value: |
                            {
                              "traffic_direction": "outbound",
                              "service_name": "my-app",
                              "sp_backend_url": "http://softprobe-runtime:8080"
                            }
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
    - name: app
      type: LOGICAL_DNS
      connect_timeout: 5s
      load_assignment:
        cluster_name: app
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 8081 }

    - name: upstream
      type: LOGICAL_DNS
      connect_timeout: 5s
      load_assignment:
        cluster_name: upstream
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: upstream.example.com, port_value: 443 }

    - name: softprobe-runtime
      type: LOGICAL_DNS
      connect_timeout: 5s
      load_assignment:
        cluster_name: softprobe-runtime
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 8080 }

admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 18001 }
```

## WASM `pluginConfig` reference

| Key | Type | Purpose |
|---|---|---|
| `traffic_direction` | `"inbound"` \| `"outbound"` | Which leg this listener intercepts. |
| `service_name` | string | Logical service name, stamped on OTLP as `sp.service.name`. |
| `sp_backend_url` | URL | Where the WASM sends `/v1/inject` and `/v1/traces`. Point at your runtime. |
| `public_key` | string | Optional; reserved for hosted-deployment authentication. |
| `collectionRules` | object | Which paths to intercept. `{"http":{"client":[{"host":".*","paths":[".*"]}]}}` captures everything. |
| `exemptionRules` | array | Paths to skip (e.g. `/health`, `/ready`). |

The minimal config (`traffic_direction` + `service_name` + `sp_backend_url`) is enough for capture-and-replay; the other keys are refinements.

## Validate the config

Envoy can self-validate before startup:

```bash
envoy --mode validate -c envoy.yaml
# "configuration 'envoy.yaml' OK"
```

Also lint with `yamllint` to catch tab/spacing errors that Envoy silently tolerates.

## Run it

```bash
envoy -c envoy.yaml --log-level info
```

You should see two listeners bind on `:8082` and `:8084`, and the admin interface on `:18001`.

## Smoke test

Start a runtime, then drive traffic through the ingress listener with a capture session:

```bash
# 1. Start runtime + app somewhere Envoy can reach.

# 2. Create a capture session.
SESSION=$(curl -s -XPOST http://127.0.0.1:8080/v1/sessions \
  -d '{"mode":"capture"}' | jq -r .sessionId)

# 3. Drive traffic through ingress (:8082), carrying the session header.
curl -s -H "x-softprobe-session-id: $SESSION" \
  http://127.0.0.1:8082/hello

# 4. Close and inspect.
curl -s -XPOST "http://127.0.0.1:8080/v1/sessions/$SESSION/close"
softprobe inspect case e2e/captured.case.json
```

If the case file shows ingress + egress hops, the config is correct.

## Routing ingress traffic via iptables (optional)

Without a mesh, you route your test client to the Envoy listener explicitly (send requests to `:8082`, not `:8081`). That's fine for tests.

For "transparent" routing — where an unmodified client talks to `:8081` but actually hits Envoy first — use `iptables` on Linux:

```bash
# Redirect localhost TCP :8081 → :8082 (ingress)
iptables -t nat -A OUTPUT -p tcp -d 127.0.0.1 --dport 8081 -j REDIRECT --to-port 8082

# Redirect outbound HTTP from app → :8084 (egress)
# (use a uid/gid match to avoid redirecting Envoy itself)
iptables -t nat -A OUTPUT -p tcp --dport 443 -m owner --uid-owner appuser -j REDIRECT --to-port 8084
```

This is the same principle Istio uses in-cluster. The complexity is real — prefer explicit routing unless transparency is a product requirement.

## Health and readiness

| Endpoint | Port | Purpose |
|---|---|---|
| `/ready` on admin | `:18001` | Envoy own-health |
| `/health` on the runtime | `:8080` | Runtime own-health |

Make your deployment system depend on both. Envoy should not accept traffic before the runtime is up.

## Uninstall

Stop Envoy (`systemctl stop envoy` or Ctrl+C). Application traffic returns to normal immediately; no lingering state.

## See also

- [Local deployment](/deployment/local) — the Docker Compose variant of this topology.
- [Kubernetes deployment](/deployment/kubernetes) — the meshed-sidecar variant.
- [Proxy OTLP API](/reference/proxy-otel-api) — what the WASM sends to the runtime.
- [Installation](/installation) — getting Envoy and the Softprobe binaries.
