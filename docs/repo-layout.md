# Softprobe Repo Layout

This document describes the **target component split** for the hybrid
proxy-first platform. It reflects the current design source of truth in
[`design.md`](./design.md): one unified runtime, one proxy data plane, and
language SDKs that all drive the same JSON control API.

Additional design notes (monorepo `docs/`): [proxy-integration-posture.md](./proxy-integration-posture.md) (proxy OTLP **out-of-band** from customer APM), [language-instrumentation.md](./language-instrumentation.md) (proxy vs optional language plane, Node legacy roadmap).

---

## 1) Target repos / packages

The target platform layout is:

1. `spec`
2. `softprobe-proxy`
3. `softprobe-runtime`
7. `softprobe-cli`
4. `softprobe-js`
5. `softprobe-go`
6. `softprobe-python`
7. `softprobe-java`
8. `docs-site`

`softprobe-runtime` may live as a monorepo package or a separate repository; the
name refers to the component, not only a Git remote.

---

## 2) Responsibilities

### `spec`

- canonical schemas
- protocol definitions
- golden examples and compatibility fixtures

### `softprobe-proxy`

- Envoy/WASM HTTP interception and enforcement
- OTLP client of the runtime for `POST /v1/inject` and `POST /v1/traces`
- request-path behavior only: header propagation, inject lookup, extract upload

The proxy talks to the runtime **only over HTTP** per `spec/`; it must not link
the runtime as a library.

### `softprobe-runtime`

- unified Go runtime serving:
  - the JSON control API
  - the proxy OTLP API
- shared in-memory session store for control and proxy handlers
- canonical `softprobe` CLI source, unless a later `softprobe-cli` split is
  explicitly approved
- v1 datastore: in-memory only; add Redis/Postgres later for HA/durability

The unified runtime is the canonical OSS topology for local and self-hosted use.
If scale later requires a process split, the control and OTLP surfaces may be
separated behind a shared datastore without changing the external contracts.

### `softprobe-js`

- TypeScript SDK
- Jest-oriented reference ergonomics and codegen
- migration-era Node-specific instrumentation and NDJSON surfaces, clearly marked
  as legacy until removed

The canonical TypeScript path is SDK-driven replay via `findInCase` and
`mockOutbound`, not framework patching.

### `softprobe-go`

- Go SDK
- Go reference ergonomics and test helpers when implemented

### `softprobe-python`

- Python SDK
- pytest-oriented integrations when implemented

### `softprobe-java`

- Java SDK
- JUnit-oriented integrations when implemented

### `docs-site`

- public user-facing docs
- guides and reference pages aligned to shipped OSS behavior
- preview/planned marking for features not yet implemented

---

## 3) Dependency rules

Allowed:

- all implementation repos depend on `spec`
- `softprobe-runtime` depends on `spec` plus small generic libraries
- language SDKs depend on `spec` and their own standard library/runtime deps

Disallowed:

- language repos depending on each other
- proxy depending on a language repo or on `softprobe-runtime` as a library
- `spec` depending on any implementation repo
- foundation/runtime code depending on package-specific instrumentations

---

## 4) Current monorepo reality

This workspace already contains the main hybrid components:

- `softprobe-runtime`
- `softprobe-proxy`
- `softprobe-js`
- `softprobe-go`
- `softprobe-python`
- `softprobe-java`
- `spec`
- `docs-site`

The monorepo is therefore already close to the target split. Future extraction
to separate repositories should preserve the same component boundaries.

---

## 5) Runtime placement

- **Canonical home:** `softprobe-runtime`
- **Canonical topology:** one unified service serving both control and OTLP APIs
- **Future split:** optional; only if scale/HA demands it, and only behind the
  same wire contracts

Older references to a "control-only runtime" or a separate mandatory "proxy
backend" are obsolete for the current OSS reference layout.

---

## 6) Canonical CLI (`softprobe`)

There is **one** authoritative command-line interface for Softprobe: the
`softprobe` binary.

- **Responsibility:** call the runtime over HTTP for control-plane operations
- **Placement:** `softprobe-runtime` unless a dedicated CLI repo is later chosen
- **Not allowed:** multiple language repos defining different first-class verbs
  for the same control-plane operations
- **Allowed:** thin language-specific shims or wrappers, as long as the product
  docs still lead with the canonical CLI

The CLI is a **client** of the runtime, not the implementation of the runtime.

---

## 7) Legacy note

Some repos in this workspace still contain migration-era surfaces from earlier
product directions, especially Node-specific NDJSON/framework instrumentation and
older proxy/dashboard positioning. Those can remain temporarily for compatibility,
but they should be documented as **legacy/migration-only** and must not replace
the hybrid design as the product source of truth.
