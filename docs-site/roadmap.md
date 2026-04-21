# Roadmap

This page tracks what's **shipped**, what's **in progress**, and what's **planned** for the Softprobe platform. Internal milestones (`P0.4`, `P1.0a`, etc.) are hidden — this is the user-visible view.

We follow a time-boxed release cadence: a minor release (`v0.N`) every **4–6 weeks**, patch releases as needed. See [Versioning](/versioning) for what "breaking" means for each surface.

---

## Shipped

### v0.5 — Unified runtime, four-SDK coverage _(current stable)_

- **`softprobe-runtime`** unified service serving the JSON control API and the proxy OTLP API from one process.
- **SDKs** in TypeScript, Python, Java, and **Go** with feature parity on `loadCaseFromFile` / `findInCase` / `mockOutbound`.
- **Jest codegen** (`softprobe generate jest-session`) for a zero-boilerplate default path.
- **Declarative `suite.yaml`** runner via `softprobe suite run`.
- **Envoy + WASM** proxy with ingress/egress topology; Docker Compose for local dev.
- **Strict policy** with observable per-session miss counter and the dedicated [debug-strict-miss](/guides/debug-strict-miss) workflow.
- **Docs site** at `docs.softprobe.dev` (this site).

### v0.4 and earlier

See the [Changelog](/changelog) for the full history back to v0.1.

---

## In progress

These are being actively worked on and expected in the next 1–2 releases. Scope may shift; track each item on GitHub.

- **Redis-backed session store** so `softprobe-runtime` can run multi-replica in Kubernetes. See [Kubernetes deployment — HA and scaling](/deployment/kubernetes). Target: **v0.6**.
- **Hosted service GA** on `o.softprobe.ai` with documented [SLA](/deployment/hosted#sla) and regional availability. Target: **v0.6**.
- **`softprobe generate test`** codegen for **pytest**, **JUnit**, and **Go** (the same ergonomics as `generate jest-session`). Target: **v0.6–v0.7**.
- **Hook runtime v1** (TypeScript/JavaScript hooks executed in a Node sidecar from the Go CLI), for data transformations and custom assertions. Target: **v0.6**.
- **Suite parallelism** — run thousands of cases concurrently with per-case session isolation. Target: **v0.6**.

---

## Planned

Items we've committed to but haven't started. Ordering reflects rough priority, not strict sequencing.

- **Multi-process runtime split** — separate the control API and OTLP backend into two deployables for clouds that want to scale them independently. Target: **v0.7**. See [Kubernetes deployment — HA and scaling](/deployment/kubernetes).
- **Ruby and .NET SDKs** — feature parity with the existing four. Target: after **v0.7**.
- **Case diffing in the browser** — a web UI to visually diff two case files or two replays. Target: **v0.8**.
- **Cloud-managed rules** — version-controlled, shareable rule bundles in the hosted service. Target: **v0.8**.
- **OpenTelemetry Collector exporter** — ship captured traces directly into your existing observability pipeline without the `softprobe export otlp` shim. Target: **v0.8**.

---

## Non-goals

To keep scope honest, here's what we are **not** planning to build:

- **Load testing** — Softprobe replays deterministically, once per case. Use a dedicated load tool for perf testing.
- **A general-purpose mock server** — rules and mocks are scoped to a session; there is no per-environment "stub" mode outside a session.
- **Protocol-level fuzzing** — we capture what your system actually did, not what it *could* do.

---

## Contribute

All roadmap items live as issues on [github.com/softprobe/softprobe](https://github.com/softprobe/softprobe). Open a **discussion** first if you want to propose a significant new capability — we'd rather agree on the shape early.
