# softprobe/hybrid

Monorepo workspace for the **Softprobe Hybrid** platform: **proxy-first HTTP** capture and replay, **OTEL-shaped** case artifacts, and a **language-neutral control plane** for rules and sessions.

## Contents

| Path | Description |
|------|-------------|
| [`spec/`](./spec/) | Canonical contracts: schemas, protocols, and examples; product design lives in [`docs/design.md`](./docs/design.md). |
| [`softprobe-proxy/`](./softprobe-proxy/) | Envoy/WASM data plane (Rust): HTTP client to runtime for inject/extract per [`spec/protocol/proxy-otel-api.md`](spec/protocol/proxy-otel-api.md). |
| **`softprobe-runtime`** (see [`tasks.md`](./tasks.md) P0.0) | OSS HTTP **control API** only ([`spec/protocol/http-control-api.md`](spec/protocol/http-control-api.md)). Inject/extract: **proxy backend** (e.g. `https://o.softprobe.ai`). |
| [`softprobe-js/`](./softprobe-js/) | JavaScript SDK, codegen, optional temporary reference runtime. |

## Design source of truth

- **Hybrid product and engineering design:** [`docs/design.md`](./docs/design.md)
- **Platform overview:** [`docs/platform-architecture.md`](./docs/platform-architecture.md)
- **Repo topology:** [`docs/repo-layout.md`](./docs/repo-layout.md)

## Contracts quick links

- [HTTP control API](spec/protocol/http-control-api.md) — sessions, cases, rules (JSON)
- [Proxy OTEL API](spec/protocol/proxy-otel-api.md) — inject lookup and trace extract (protobuf)
- [Session headers](spec/protocol/session-headers.md)

## Nested Git history (local backup)

This monorepo previously used separate Git repositories under `proxy/` and `softprobe-js/`. Their `.git` directories were moved to **`.nested-git-backup/`** (ignored by Git) so one repository could track the whole workspace. To work with the old remotes again, move the backup back, for example:

`mv .nested-git-backup/proxy.git proxy/.git`

## Contributing

Implementations should validate behavior against `spec` schemas and compatibility fixtures as they are added. Prefer extending the spec before changing proxy or SDK behavior in incompatible ways.
