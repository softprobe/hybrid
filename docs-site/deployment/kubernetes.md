# Kubernetes deployment

Attach the Softprobe WASM filter to your workload's Istio sidecar and send
inject/extract traffic to the hosted runtime at `https://runtime.softprobe.dev`.
The official Kubernetes path does not install a runtime in your cluster.

This page assumes you already run Istio or another mesh that accepts Envoy WASM
filters. If you want a laptop setup first, see [Local proxy stack](/deployment/local).

## Components

1. **Hosted runtime** — `https://runtime.softprobe.dev`, authenticated with your
   Softprobe API token.
2. **Kubernetes Secret** — stores the API token for the WASM plugin.
3. **WasmPlugin resource** — attaches the Softprobe WASM binary to opted-in
   workloads.

## 1. Store the API token

```bash
kubectl create secret generic softprobe-api-token \
  --namespace default \
  --from-literal=SOFTPROBE_API_TOKEN="$SOFTPROBE_API_TOKEN"
```

Use your secret manager or GitOps controller if you do not create secrets
imperatively.

## 2. The WASM filter (Istio)

```yaml
# softprobe-wasmplugin.yaml
apiVersion: extensions.istio.io/v1alpha1
kind: WasmPlugin
metadata:
  name: softprobe
  namespace: default
spec:
  selector:
    matchLabels:
      softprobe.dev/capture: "enabled"
  url: oci://ghcr.io/softprobe/softprobe-proxy:v0.5
  imagePullPolicy: IfNotPresent
  phase: AUTHZ
  pluginConfig:
    sp_backend_url: https://runtime.softprobe.dev
    public_key: "<your-api-token>"
```

If your Istio version supports environment expansion or secret references in
`pluginConfig`, wire `public_key` from the Kubernetes Secret above. Otherwise,
render the manifest in CI/CD from a secret value.

::: tip Out-of-band capture (`sp_backend_url`)
The WASM filter POSTs inject/extract OTLP to `sp_backend_url`, separate from
your production OpenTelemetry exporter. Full HTTP bodies are sent only to the
Softprobe hosted runtime, not to Datadog, Honeycomb, New Relic, or similar by
default.
:::

Opt a workload in:

```yaml
metadata:
  labels:
    softprobe.dev/capture: "enabled"
```

This keeps the WASM filter out of workloads that do not need it.

## 3. Give the app the session header routes

Your tests or the CLI send `x-softprobe-session-id` on ingress. In production
canaries, restrict who can send that header:

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: checkout
  namespace: default
spec:
  hosts: [checkout.default.svc.cluster.local]
  http:
    - match:
        - headers:
            x-softprobe-session-id:
              exact: ""
      headers:
        request:
          remove: [x-softprobe-session-id]
      route:
        - destination:
            host: checkout.default.svc.cluster.local
    - route:
        - destination:
            host: checkout.default.svc.cluster.local
```

This pattern prevents external callers from injecting session ids; only internal
test drivers can carry the header.

## 4. Running tests

Tests running in a `Job`, `Pod`, or external CI runner use the hosted default:

```bash
export SOFTPROBE_API_TOKEN=...
unset SOFTPROBE_RUNTIME_URL
softprobe doctor
```

Then run your tests against the mesh ingress URL and include
`x-softprobe-session-id` on inbound traffic.

## 5. Capture output

The hosted runtime stores captured traces remotely while the session is open.
Close the session with `--out` to download a case file into your repository:

```bash
softprobe session close --session "$SOFTPROBE_SESSION_ID" --out cases/checkout.case.json
```

## Observability

Use `softprobe doctor --verbose`, `softprobe inspect session`, proxy logs, and
your app logs for troubleshooting. Hosted runtime health and API errors are
reported through the CLI and SDKs.

## Uninstall

```bash
kubectl delete wasmplugin softprobe -n default
kubectl delete secret softprobe-api-token -n default
```

Traffic returns to normal immediately.

## Security checklist

- **Protect the API token.** Store it in Kubernetes Secrets or your secret
  manager; do not commit rendered manifests with the token.
- **Session ids are capabilities.** Treat them as secrets in shared environments.
- **Capture PII with intent.** Apply redaction rules before driving production
  traffic into capture mode.
- **Restrict egress deliberately.** Workloads using Softprobe need HTTPS egress
  to `runtime.softprobe.dev`.

## Next

- [CI integration](/guides/ci-integration) — running suites against the hosted runtime.
- [Hosted runtime](/deployment/hosted) — account, token, and runtime behavior.
- [Troubleshooting](/guides/troubleshooting) — mesh-specific failures.
