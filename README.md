# softprobe/hybrid

Monorepo workspace for the **Softprobe Hybrid** platform: **proxy-first HTTP** capture and replay, **OTEL-shaped** case artifacts, and a **language-neutral control plane** for rules and sessions.

## Contents

| Path | Description |
|------|-------------|
| [`softprobe-spec/`](./softprobe-spec/) | Canonical contracts: schemas, protocols, architecture docs, and the [hybrid platform design](./softprobe-spec/docs/hybrid-platform-design.md). |
| [`proxy/`](./proxy/) | Envoy/WASM data plane (Rust): HTTP interception, inject/extract OTEL API toward the runtime. |
| [`softprobe-js/`](./softprobe-js/) | JavaScript SDK, CLI, codegen, and related tooling. |

## Design source of truth

- **Hybrid product and engineering design:** [`softprobe-spec/docs/hybrid-platform-design.md`](./softprobe-spec/docs/hybrid-platform-design.md)
- **Platform overview:** [`softprobe-spec/docs/platform-architecture.md`](./softprobe-spec/docs/platform-architecture.md)
- **Repo topology:** [`softprobe-spec/docs/repo-layout.md`](./softprobe-spec/docs/repo-layout.md)

## Contracts quick links

- [HTTP control API](softprobe-spec/protocol/http-control-api.md) — sessions, cases, rules (JSON)
- [Proxy OTEL API](softprobe-spec/protocol/proxy-otel-api.md) — inject lookup and trace extract (protobuf)
- [Session headers](softprobe-spec/protocol/session-headers.md)

## Nested Git history (local backup)

This monorepo previously used separate Git repositories under `proxy/` and `softprobe-js/`. Their `.git` directories were moved to **`.nested-git-backup/`** (ignored by Git) so one repository could track the whole workspace. To work with the old remotes again, move the backup back, for example:

`mv .nested-git-backup/proxy.git proxy/.git`

## Contributing

Implementations should validate behavior against `softprobe-spec` schemas and compatibility fixtures as they are added. Prefer extending the spec before changing proxy or SDK behavior in incompatible ways.
