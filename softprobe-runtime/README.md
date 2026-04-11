# softprobe-runtime

`softprobe-runtime` is the OSS control runtime for Softprobe.

It serves the JSON HTTP control API only:

- `POST /v1/sessions`
- `POST /v1/sessions/{sessionId}/load-case`
- `POST /v1/sessions/{sessionId}/policy`
- `POST /v1/sessions/{sessionId}/rules`
- `POST /v1/sessions/{sessionId}/fixtures/auth`
- `POST /v1/sessions/{sessionId}/close`

Default listen address:

- `127.0.0.1:8080`

This runtime does not serve proxy inject/extract endpoints. Those live in the proxy backend, for example `https://o.softprobe.ai`.

See:

- [HTTP control API](../spec/protocol/http-control-api.md)
- [Hybrid platform design](../docs/design.md)
- [Repo layout](../docs/repo-layout.md)
- [Kubernetes deployment note](../docs/platform-architecture.md#105-kubernetes-informative)
- [Proxy deployment guide](../softprobe-proxy/docs/deployment.md)

An informative Kubernetes example lives in [`deploy/kubernetes.yaml`](./deploy/kubernetes.yaml). The proxy backend URL belongs on the proxy WasmPlugin as `sp_backend_url`; do not point it at the control runtime.

## Placeholder CLI

The repo also includes a minimal `softprobe` CLI in [`cmd/softprobe`](./cmd/softprobe).

- `softprobe --version` prints the binary version
- `softprobe doctor --runtime-url http://127.0.0.1:8080` checks `GET /health`
- `softprobe session start --runtime-url http://127.0.0.1:8080 --json` prints `sessionId`, `sessionRevision`, `specVersion`, and `schemaVersion`
- `softprobe session start --runtime-url http://127.0.0.1:8080` also prints `export SOFTPROBE_SESSION_ID=...` for shell use
- `softprobe session load-case --runtime-url http://127.0.0.1:8080 --session $ID --file cases/example.case.json` uploads a case file

This is a placeholder for the later canonical CLI work in `tasks.md`.

Exit codes:

- `0` on success
- `1` on runtime/API failures
- `2` on usage or flag parsing errors
