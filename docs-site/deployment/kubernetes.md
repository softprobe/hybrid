# Kubernetes deployment

Run the Softprobe runtime as a regular `Deployment` and attach the Softprobe WASM filter to your workload's Istio sidecar (or directly to Envoy).

This page assumes you already run Istio (or another mesh that accepts Envoy WASM filters). If you don't, see [Local deployment](/deployment/local) or use [Hosted](/deployment/hosted).

## Components

1. **Runtime Deployment + Service** — one `Deployment` with 1–3 replicas; `ClusterIP` Service on port 8080.
2. **WasmPlugin resource** — attaches the Softprobe WASM binary to your workload's sidecar filter chain.
3. **(Optional) Captures PersistentVolumeClaim** — if you want captured case files to live on cluster storage.

## 1. The runtime

```yaml
# softprobe-runtime.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: softprobe-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: softprobe-runtime
  namespace: softprobe-system
spec:
  replicas: 1
  selector:
    matchLabels: { app: softprobe-runtime }
  template:
    metadata:
      labels: { app: softprobe-runtime }
    spec:
      containers:
        - name: runtime
          image: ghcr.io/softprobe/softprobe-runtime:v0.5
          ports:
            - containerPort: 8080
              name: http
          env:
            - name: SOFTPROBE_LISTEN_ADDR
              value: "0.0.0.0:8080"
            - name: SOFTPROBE_LOG_LEVEL
              value: "info"
            # Uncomment to persist captures
            # - name: SOFTPROBE_CAPTURE_CASE_PATH
            #   value: "/cases/session-{sessionId}.case.json"
          # volumeMounts:
          #   - name: cases
          #     mountPath: /cases
          resources:
            requests: { cpu: 100m, memory: 128Mi }
            limits:   { cpu: 1000m, memory: 1Gi }
          readinessProbe:
            httpGet: { path: /health, port: 8080 }
            periodSeconds: 5
          livenessProbe:
            httpGet: { path: /health, port: 8080 }
            periodSeconds: 15
      # volumes:
      #   - name: cases
      #     persistentVolumeClaim:
      #       claimName: softprobe-cases
---
apiVersion: v1
kind: Service
metadata:
  name: softprobe-runtime
  namespace: softprobe-system
spec:
  selector: { app: softprobe-runtime }
  ports:
    - name: http
      port: 8080
      targetPort: 8080
```

Apply:

```bash
kubectl apply -f softprobe-runtime.yaml
kubectl -n softprobe-system rollout status deploy/softprobe-runtime
```

## 2. The WASM filter (Istio)

```yaml
# softprobe-wasmplugin.yaml
apiVersion: extensions.istio.io/v1alpha1
kind: WasmPlugin
metadata:
  name: softprobe
  namespace: default          # put it in every namespace you want to capture in
spec:
  selector:
    matchLabels:
      softprobe.dev/capture: "enabled"   # only workloads opted in
  url: oci://ghcr.io/softprobe/softprobe-proxy:v0.5
  imagePullPolicy: IfNotPresent
  phase: AUTHZ                # runs after auth, before routing
  pluginConfig:
    sp_backend_url: http://softprobe-runtime.softprobe-system:8080
```

Opt a workload in:

```yaml
# pod template
metadata:
  labels:
    softprobe.dev/capture: "enabled"
```

This keeps the WASM filter out of workloads that don't need it.

## 3. Give the app the session header routes

Your tests (or the CLI) send `x-softprobe-session-id` on ingress. In production canaries, you may want a VirtualService that only accepts the header from an internal source:

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
              exact: ""       # empty: drop the header from outside traffic
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

This pattern prevents external callers from injecting session ids; only internal test drivers can carry the header.

## 4. Exposing the runtime to tests

### From inside the cluster

Tests running in a `Job` or `Pod` in the cluster use the service DNS:

```bash
SOFTPROBE_RUNTIME_URL=http://softprobe-runtime.softprobe-system:8080
```

### From outside the cluster

Port-forward for development:

```bash
kubectl -n softprobe-system port-forward svc/softprobe-runtime 8080:8080
```

Or expose via an `Ingress` / `Gateway` — protect it with auth.

## 5. Capture to object storage

For large-scale captures (thousands of sessions per night), write to S3/GCS instead of a local volume:

```yaml
env:
  - name: SOFTPROBE_CAPTURE_CASE_PATH
    value: "s3://my-bucket/captures/{sessionId}.case.json"
  - name: AWS_REGION
    value: "us-west-2"
  # Credentials via IRSA / workload identity preferred.
```

The runtime streams each closed session's case file directly to object storage. Supported schemes: `s3://`, `gs://`, `azblob://`, `file://`.

## 6. HA and scaling — staged rollout

The runtime evolves through three deployment stages as your load grows. Pick the earliest stage that meets your SLO.

### Stage 1 — In-memory, single-replica (v0.5 OSS default)

- One `Deployment` with `replicas: 1`.
- Session state lives entirely in process memory.
- Case bytes and rule documents held in a `sync.Map`-backed store.
- **Good for:** up to ~hundreds of concurrent sessions; CI suites; dev environments.
- **Failure mode:** a runtime pod restart drops all active sessions — tests in flight fail fast with 404. Acceptable for CI because the outer retry catches it.

Manifest delta — none; this is the default from the manifest above.

### Stage 2 — Redis-backed, multi-replica-ready (planned v0.6)

- Session state (rules, loaded cases, revision, fixtures) moves to **Redis** or **PostgreSQL**.
- Runtime becomes stateless — `replicas: 3+` with a rolling update strategy.
- Proxy continues to POST `/v1/inject` to a stable `ClusterIP` Service; any replica answers correctly.
- **Good for:** low-to-mid thousands of concurrent sessions; blue/green runtime upgrades without dropping sessions.

Manifest delta:

```yaml
# softprobe-runtime deployment
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: runtime
          env:
            - name: SOFTPROBE_STORE_BACKEND
              value: "redis"
            - name: SOFTPROBE_STORE_URL
              valueFrom:
                secretKeyRef:
                  name: softprobe-redis
                  key: url
            # sessions shard by sessionId; no sticky routing required.
```

Plus a Redis StatefulSet (or a managed Redis) sized for ~1 KB × active sessions.

### Stage 3 — Multi-process split (planned v0.7)

- Split the unified runtime into two `Deployment`s:
  - **`softprobe-control`** — serves the JSON control API (tests, CLI, SDKs); low CPU.
  - **`softprobe-otlp`** — serves the OTLP `/v1/inject` + `/v1/traces` (proxy only); high CPU, scales horizontally.
- Both read/write the same Redis/Postgres store.
- **Good for:** peak throughput beyond ~20k inject RPS; independent scaling of control and data planes.

Manifest delta:

```yaml
# Two deployments, one Service each
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: softprobe-control, namespace: softprobe-system }
spec:
  replicas: 2
  # identical container, + env:
  #   SOFTPROBE_SERVE=control
  #   SOFTPROBE_STORE_URL=...
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: softprobe-otlp, namespace: softprobe-system }
spec:
  replicas: 8                     # scale with ingress RPS
  # identical container, + env:
  #   SOFTPROBE_SERVE=otlp
  #   SOFTPROBE_STORE_URL=...
```

The WasmPlugin's `sp_backend_url` points at `softprobe-otlp` (data plane), while your tests hit `softprobe-control` (control plane). Both are in-cluster ClusterIPs.

### Sizing reference

| Stage | Sessions / runtime pod | CPU / pod | Memory / pod |
|---|---|---|---|
| 1 (in-memory) | ~500 idle, ~2000 light | 200m avg, 1000m peak | 200–500 MB |
| 2 (Redis-backed) | ~2000 idle, ~5000 light | 400m avg, 1500m peak | 100–200 MB (state offloaded) |
| 3 (split, `otlp`) | scales linearly | 500m per ~2k RPS | 100–150 MB |

Tune `resources.requests` and `HorizontalPodAutoscaler` thresholds based on these numbers.

## 7. Observability

### Metrics (Prometheus)

```yaml
ports:
  - name: metrics
    port: 9090
    targetPort: 9090
```

Scrape `/metrics`. Exposed metrics:

- `softprobe_sessions_total{mode=…}`
- `softprobe_inject_requests_total{result=hit|miss}`
- `softprobe_inject_latency_seconds_bucket`
- `softprobe_extract_spans_total`

### Logs

JSON-formatted to stdout. Aggregate with Fluent Bit / Vector / Loki as usual.

## Uninstall

```bash
kubectl delete wasmplugin softprobe -n default
kubectl delete -f softprobe-runtime.yaml
```

Traffic returns to normal immediately.

## Security checklist

- **Runtime is unauthenticated by default today.** Network-restrict it inside the cluster; bearer-token auth for the OSS runtime is planned follow-up work.
- **Session ids are capabilities.** Treat them as secrets in shared environments.
- **Capture PII with intent.** Apply redaction rules before driving production traffic into capture mode.
- **Network-restrict the runtime** to the namespaces that need it via `NetworkPolicy`.

## Next

- [CI integration](/guides/ci-integration) — running suites against a cluster-resident runtime.
- [Hosted deployment](/deployment/hosted) — the managed alternative.
- [Troubleshooting](/guides/troubleshooting) — mesh-specific failures.
