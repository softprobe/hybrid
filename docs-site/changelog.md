# Changelog

All notable changes to the Softprobe platform are listed here.

**Format.** Each entry groups changes by **Added**, **Changed**, **Deprecated**, **Removed**, **Fixed**, and **Security**. Breaking changes under the current major are called out explicitly with a ⚠ marker; see [Versioning](/versioning) for what "breaking" means per surface.

**Scope.** This changelog covers the **user-visible surface**: the CLI, SDKs, HTTP control API, case/rule schemas, proxy WASM filter, and docs site. Internal refactors appear only when they change observable behavior.

**Source.** Entries are reconciled against `docs/design.md` §16 (document history) and each release's GitHub notes.

---

## [Unreleased]

### Changed

- **Docs:** CLI reference (`reference/cli.md`) aligned with the shipped binary — removed stale “not shipped yet” callouts for `inspect session`, `validate`, `generate test`, `export otlp`, `scrub`, and `completion`; expanded the `--json` field-stability table; refreshed `roadmap.md` and SDK reference links to the `hybrid` monorepo.

---

## [v0.5] — 2026-04-12

Unified-runtime release. The platform now ships as **one binary** (`softprobe-runtime`) that serves both the JSON control API and the proxy OTLP API from a shared in-memory store.

### Added
- `softprobe-runtime` single-process runtime exposing both `POST /v1/sessions/*` (control) and `POST /v1/inject` + `POST /v1/traces` (proxy backend).
- Go SDK (`github.com/softprobe/softprobe-go`) with feature parity to the TypeScript, Python, and Java SDKs.
- AI-agent-friendly CLI surface: `doctor --json`, `session start --shell`, `inspect case --json`.
- `sessionRevision` monotonic counter surfaced in every session-modifying control response; enables the optional proxy inject cache.
- `capture_only` rule action for observe-only sessions that still need to record live traffic.
- User-facing docs site at `docs.softprobe.dev` (this site).

### Changed
- **`sp_backend_url`** now points at the runtime (same base URL as `SOFTPROBE_RUNTIME_URL`) instead of a separate service.
- SDKs merge rule payloads **client-side** before POSTing; the runtime still treats `POST …/rules` as a full replace. See [Merge on the client, replace on the wire](/reference/sdk-typescript#mockoutbound-spec).

### Deprecated
- ⚠ `then.action: "replay"` is deprecated. Old case files continue to parse; new authoring should use `mock` (proxy-side) or SDK-side `findInCase` + `mockOutbound`. See [Rule schema](/reference/rule-schema#then-action-replay-deprecated).

### Fixed
- Session correlation now honors W3C `tracestate` when `traceparent` is rewritten by an upstream hop (see [Trace context propagation](/concepts/architecture#trace-context-propagation-critical)).

---

## [v0.4] — 2026-04-11

Split-topology release that drew the line between a control runtime (OSS, in-memory OK) and a proxy backend ("e.g. `https://runtime.softprobe.dev`"). Superseded by v0.5's unified runtime, but the deployment patterns (single-replica vs HA) carry forward.

### Added
- Initial datastore guidance for HA deployments (Redis/Postgres).
- Kubernetes manifest templates split by control vs proxy-backend concerns.

### Changed
- Control API and proxy backend became independently deployable.

---

## [v0.3] — 2026-04-11

First packaged runtime service.

### Added
- `softprobe-runtime` service concept: one process for all control-plane duties.
- CLI repositioned as a **client** of the runtime (no embedded state).
- Kubernetes `Deployment` pattern documented in `docs/platform-architecture.md`.

### Changed
- `SOFTPROBE_RUNTIME_URL` became the canonical environment variable; older `SOFTPROBE_SERVER_URL` was aliased and deprecated.

---

## [v0.2] — 2026-04-11

CLI-first simplicity pass.

### Added
- `softprobe` as the **canonical** binary name.
- Default happy-path `softprobe capture run … -- <command>` and `softprobe replay run …`.
- Machine-readable `--json` output + stable exit codes for agent/CI consumption.
- OTLP JSON profile direction for case files.

### Changed
- Removed ad-hoc flag variants in favor of consistent `noun verb` grammar.

---

## [v0.1] — 2026-04-05

Initial hybrid design.

### Added
- Case JSON file format carrying OTLP traces.
- Proxy-first capture model (Envoy + WASM).
- Cross-language control via an HTTP API.
- Rule format and per-session policy revision concept.

---

## Unreleased

Items here have landed on `main` but not yet in a tagged release. They will move under the next `v0.N` heading on release day.

- _(Nothing pending right now.)_

---

## See also

- [Roadmap](/roadmap) — what's coming next.
- [Versioning](/versioning) — how we number releases and what counts as breaking.
- [GitHub releases](https://github.com/softprobe/softprobe/releases) — binaries, checksums, source tarballs.
