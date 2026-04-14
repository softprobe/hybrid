# Softprobe Repo Layout

This document defines the target multi-repo strategy.

---

## 1) Target repos

The target platform layout is:

1. `spec`
2. `softprobe-proxy`
3. `softprobe-runtime` (OSS **control API** service only — [http-control-api.md](../spec/protocol/http-control-api.md); see [platform architecture — section 10](./platform-architecture.md#10-softprobe-runtime-implementation-and-deployment))
4. `softprobe-js`
5. `softprobe-python`
6. `softprobe-java`

`softprobe-runtime` may initially be a **directory in this monorepo** or a **separate repository**; the name refers to the **component**, not only a Git remote.

---

## 2) Responsibilities

### `spec`

- canonical schemas
- protocol definitions
- compatibility fixtures

### `softprobe-proxy`

- HTTP interception and enforcement (Envoy/WASM); **client** of the **proxy backend** for inject/extract per [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md) (default hosted URL is product-specific, e.g. `https://o.softprobe.ai`)

### `softprobe-runtime`

- HTTP server implementing **only** [HTTP control API](../spec/protocol/http-control-api.md) (JSON): sessions, `load-case`, rules, policy, fixtures, close
- **Does not** implement [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md); that service is the **proxy backend** (hosted or self-hosted)
- **v1 datastore:** in-memory session state is enough; **no database required** for OSS (add Redis/Postgres only for HA or durability — see [platform architecture](./platform-architecture.md) §10.2)
- **Recommended implementation languages:** **Go** or **Rust** (JSON HTTP server + static CLI). **Node** acceptable only as a **short-term reference** if it lives in `softprobe-js` until extraction.
- ships or co-releases the canonical **`softprobe` CLI** unless a separate `softprobe-cli` repo is explicitly chosen (see §6)

### `softprobe-js`

- **First-stage SDK (v1):** TypeScript/JavaScript client + **Jest** reference tests and ergonomic session helpers that **materialize** `load-case` / `rules` / `policy` via the control API (see `docs/design.md` §5.3 and §7.0).
- JS generator
- optional **thin** launcher shim only (for example `npx` delegating to the canonical **`softprobe` binary**—see §6)
- optional **temporary** reference runtime implementation (prefer moving server code to `softprobe-runtime` over time)

### `softprobe-python`

- Python SDK
- Pytest integration
- generator
- runtime client (HTTP control API)

### `softprobe-java`

- Java SDK
- JUnit integration
- generator
- runtime client (HTTP control API)

---

## 3) Dependency rules

Allowed:

- all implementation repos depend on `spec`

Disallowed:

- language repos depending on each other
- proxy depending on any language repo or on `softprobe-runtime` **as a library** (proxy talks to runtime **only over HTTP** per `spec`)
- spec depending on any implementation repo

Allowed:

- `softprobe-runtime` depends only on `spec` (plus stdlib / small generic deps)
- `softprobe` CLI (wherever it lives) depends only on `spec` for contract constants and **HTTP** to the runtime

---

## 4) Short-term transition

Current repos in the workspace are:

- `proxy`
- `softprobe-js`

Current transition plan:

- treat `proxy` as the future `softprobe-proxy`
- use `spec` as the new canonical home for shared contracts
- keep `softprobe-js` focused on JS implementation concerns

---

## 5) Runtime placement

- **Canonical home:** `softprobe-runtime` (repo or monorepo package) implements the **HTTP control API** only. The **proxy OTEL API** is implemented by the **proxy backend** (e.g. `https://o.softprobe.ai`), not this repo.
- **Transition:** A reference runtime may stay in `softprobe-js` **temporarily**; **schemas and protocol definitions** must remain in `spec`, not in any language repo, as the permanent source of truth.

---

## 6) Canonical CLI (`softprobe`)

There is **one** authoritative command-line interface for Softprobe: the **`softprobe`** binary.

- **Responsibility:** **Call** the [HTTP control API](../spec/protocol/http-control-api.md) on the Softprobe Runtime (sessions, load-case, rules, policy, doctor, inspect, capture/replay orchestration, and so on). The **runtime** implements that API; the CLI is a **client**. **Language-agnostic** CLI implementation (for example **Go** or **Rust**) is recommended so CI and agents install one artifact.
- **Not allowed:** Each language repo shipping a **different** set of verbs or flags for the same operations (that fragments docs and AI prompts).
- **Allowed:** Language-specific **SDKs** and **optional shims** that shell out to or bundle the same binary.

**Placement:** The CLI source should live in **`softprobe-runtime`** (same release train as the server) or a **dedicated** `softprobe-cli` repository. A **temporary** copy alongside a Node reference in `softprobe-js` is allowed only until extraction. Either way, **releases** should publish a **single** `softprobe` artifact per platform.
