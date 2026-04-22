# Roadmap

This page tracks what's **shipped**, what's **in progress**, and what's **planned** for the Softprobe platform. Internal milestones (`P0.4`, `P1.0a`, etc.) are hidden — this is the user-visible view.

We follow a time-boxed release cadence: a minor release (`v0.N`) every **4–6 weeks**, patch releases as needed. See [Versioning](/versioning) for what "breaking" means for each surface.

::: info Scope status
We've accepted that every feature currently documented on this site is a commitment. Items we haven't built yet are in _In progress_ below, each one tied to a delivery task in [`tasks.md`](https://github.com/softprobe/hybrid/blob/main/tasks.md) (prefix `PD`). The _Planned_ section is intentionally short — only work we've explicitly scoped past the current delivery wave belongs there.
:::

---

## Shipped

### v0.5 — Unified runtime, four-SDK coverage _(current stable)_

- **`softprobe-runtime`** unified service serving the JSON control API and the proxy OTLP API from one process with a shared in-memory session store.
- **SDKs** in TypeScript, Python, Java, and Go with feature parity on `loadCaseFromFile` / `loadCase` / `findInCase` / `findAllInCase` / `mockOutbound` / `clearRules` / `setPolicy` / `setAuthFixtures` and typed errors (`*RuntimeError`, `*RuntimeUnreachableError`, `*UnknownSessionError`, `*CaseLoadError`, `*CaseLookupAmbiguityError`).
- **Jest codegen** (`softprobe generate jest-session`) for a zero-boilerplate default path.
- **Envoy + WASM** proxy with ingress / egress listener pair; Docker Compose for local dev.
- **Strict policy** with observable per-session miss counter and the [debug-strict-miss](/guides/debug-strict-miss) workflow.
- **CLI**: `doctor`, `session {start,load-case,rules apply,policy set --strict,stats,close}`, `inspect {case,session}`, `validate {case,rules,suite}`, `generate {jest-session,test}`, `export otlp`, `scrub`, `completion`, `capture run`, `replay run`, and **`suite {run,validate,diff}`** with Node hook sidecar + JUnit/HTML reporters — see the [`e2e/cli-suite-run/`](https://github.com/softprobe/hybrid/tree/main/e2e/cli-suite-run) end-to-end harness.
- **Docs site** at `docs.softprobe.dev` (this site).
- **End-to-end acceptance tests** covering capture, replay, and strict-miss paths across Go, Jest, pytest, and JUnit harnesses.

### v0.4 and earlier

See the [Changelog](/changelog) for the full history back to v0.1.

---

## In progress

Each item below is actively scoped in [`tasks.md`](https://github.com/softprobe/softprobe/blob/main/tasks.md) with a `PD*` prefix. Targets are intent, not promises; scope may shift within the delivery wave.

### Delivering what's already documented (closing the doc-vs-shipped gap)

- **CLI surface parity with `reference/cli.md`** — `suite {run,validate,diff}`, `validate {case,rules,suite}`, `inspect session`, `generate test`, `export otlp`, `scrub`, `completion`, `capture run`, and `replay run` are **shipped** in the current CLI build. Remaining gaps are mostly **contract polish**: stable exit codes on every path ([PD1.1a](https://github.com/softprobe/hybrid/blob/main/tasks.md)), universal `--json` envelope on every mutating command ([PD1.1c](https://github.com/softprobe/hybrid/blob/main/tasks.md)), and a few session conveniences ([PD1.3](https://github.com/softprobe/hybrid/blob/main/tasks.md)). Tracks [tasks.md PD1](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd1--cli-contract-completeness).
- **Auth plumbing** — CLI and all four SDKs attach `Authorization: Bearer` when `SOFTPROBE_API_TOKEN` is set ([PD2.1a–e](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd2--runtime-auth-plumbing-in-sdks-and-cli) shipped). Remaining: **e2e auth** wiring across harnesses ([PD2.1f](https://github.com/softprobe/hybrid/blob/main/tasks.md)).
- **Runtime observability** — Prometheus `/metrics`, `SOFTPROBE_LOG_LEVEL`, `{sessionId}` template substitution in `SOFTPROBE_CAPTURE_CASE_PATH`. Target: **v0.6**. Tracks [tasks.md PD4](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd4--runtime-observability-and-capture-operations).
- **Object-storage capture writers** — `s3://`, `gs://`, `azblob://` destinations for capture output (default remains `file://`). Target: **v0.6–v0.7**. Tracks [tasks.md PD4.4a](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd4--runtime-observability-and-capture-operations).
- **TS SDK reference alignment** — **shipped** in current build (hooks + suite subpaths, error aliases, `setLogger` / `SOFTPROBE_LOG`). Tracks [tasks.md PD3](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd3--typescript-sdk-reference-reality-alignment).
- **Release hygiene** — dual-license `LICENSE` coverage, runtime + WASM images on GHCR, and build-time CLI version string are **landed**; automated **npm / PyPI / Maven / Go module** publishes remain. See [`LICENSING.md`](https://github.com/softprobe/hybrid/blob/main/LICENSING.md). Tracks [tasks.md PD5](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd5--release-hygiene).
- **Doc truth sync** — CLI reference banners for shipped commands were cleared in [Phase PD6](https://github.com/softprobe/hybrid/blob/main/tasks.md#phase-pd6--doc-truth-sync-after-each-code-phase-lands). Deployment pages still carry `::: warning Not shipped yet` only where the runtime feature is genuinely pending (for example PD4 metrics / capture templates).

### Scaling and hosted-service track

- **Redis-backed session store** so `softprobe-runtime` can run multi-replica in Kubernetes. See [Kubernetes deployment — HA and scaling](/deployment/kubernetes). Target: **v0.6–v0.7**.
- **Hosted service GA** on `o.softprobe.ai` with documented [SLA](/deployment/hosted#sla) and regional availability. Target: **v0.6–v0.7**.
- **Multi-process runtime split** — separate the control API and OTLP backend into two deployables for clouds that want to scale them independently. Target: **v0.7**. Depends on the Redis store landing first.
- **Hook runtime v1** — TypeScript/JavaScript hooks executed in a Node sidecar from the Go CLI for data transformations and custom assertions. Shipped in the current build: `RequestHook`, `MockResponseHook`, `BodyAssertHook`, `HeadersAssertHook` are resolved from `--hooks *.ts` files via the embedded sidecar; end-to-end harness at [`e2e/cli-suite-run/`](https://github.com/softprobe/softprobe/tree/main/e2e/cli-suite-run). Remaining hook runtime work (Python/Java sidecars for those CLIs, hook sandboxing options) is tracked in PD3+.

### Ecosystem track

- **Ruby and .NET SDKs** — feature parity with the existing four. Target: **v0.7–v0.8**.
- **Case diffing in the browser** — web UI to visually diff two case files or two replays. Target: **v0.8**.
- **Cloud-managed rules** — version-controlled, shareable rule bundles in the hosted service. Target: **v0.8**.
- **OpenTelemetry Collector exporter** — ship captured traces directly into your existing observability pipeline without the `softprobe export otlp` shim. Target: **v0.8**.

---

## Planned

Reserved for work we've explicitly scoped past the current delivery wave and that isn't yet documented as a shipped surface on this site. Empty today — everything on the public doc site is tracked above under _In progress_.

---

## Non-goals

To keep scope honest, here's what we are **not** planning to build:

- **Load testing** — Softprobe replays deterministically, once per case. Use a dedicated load tool for perf testing.
- **A general-purpose mock server** — rules and mocks are scoped to a session; there is no per-environment "stub" mode outside a session.
- **Protocol-level fuzzing** — we capture what your system actually did, not what it *could* do.
- **gRPC / WebSockets / long-running SSE** in the WASM filter at v1 — we ship HTTP/1.1 and HTTP/2 request-response only; streaming protocols are on a later track.

---

## Contribute

All roadmap items live in the [hybrid](https://github.com/softprobe/hybrid) monorepo. Open a **discussion** first if you want to propose a significant new capability — we'd rather agree on the shape early.
