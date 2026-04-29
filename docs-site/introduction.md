# Introduction

Softprobe is a **record-and-replay platform for HTTP integration tests**. It captures real traffic through a sidecar proxy and lets you replay it — deterministically and cross-language — as part of your normal test suite.

If you write HTTP services that call other HTTP services, you have probably felt the integration-test dilemma: **real upstreams are flaky and expensive, hand-written mocks drift from reality**. Softprobe closes that gap by making the mock *be* the real response — one you recorded on a specific day, from a specific version of the upstream, with the exact bytes you saw in production.

## What problem does Softprobe solve?

| Pain | What teams usually do | What Softprobe does |
|---|---|---|
| Integration tests hit live APIs and flake | Avoid writing them | Record once, replay deterministically |
| Hand-written mocks drift from reality | Accept the drift | Mock payload = captured payload |
| Different languages, different mock libraries | Duplicate fixtures in each repo | One case file, four SDKs consume it |
| Can't test the full request path (sidecar, headers, auth) | Skip it in CI | Run the real sidecar against the replayed upstream |
| Recording PII in production is scary | Give up on prod capture | Capture behind a header; mask in the hook |

## The mental model in 60 seconds

Think of Softprobe as **three moving parts** and **one artifact**:

```d2
direction: down

local: {
  label: "User local components"
  style: {
    fill: "#e8f1ff"
    stroke: "#2563eb"
  }
  direction: right

  test_cli: "Test / CLI\nJest / pytest / JUnit / go test"
  proxy: "Envoy sidecar\nSoftprobe WASM"
  app: "Your application\nunder test"

  test_cli -> proxy: "test traffic"
  proxy <-> app: "app HTTP"
}

row: {
  direction: right

  hosted: {
    label: "Softprobe hosted services"
    style: {
      fill: "#ede9fe"
      stroke: "#7c3aed"
    }
    runtime: "runtime.softprobe.dev\ncontrol API + OTLP"
    storage: "Object storage\n*.case.json + OTLP JSON"
    runtime -> storage: "case artifacts"
  }

  external: {
    label: "User external HTTP dependencies"
    style: {
      fill: "#fff4e6"
      stroke: "#ea580c"
    }
    deps: "Payment gateway / CRM / APIs\ne.g. api.stripe.com"
  }
}

local.proxy <-> row.external.deps: "upstream HTTP on miss"
local.test_cli <-> row.hosted.runtime: "control API"
local.proxy <-> row.hosted.runtime: "OTLP inject + traces"
```

1. **Proxy** — Envoy with the Softprobe WASM filter. Sits beside your app as a sidecar and sees every inbound and outbound HTTP hop.
2. **Hosted runtime** — the Softprobe service at `https://runtime.softprobe.dev`. It speaks an HTTP control API (used by tests and the CLI) and an OTLP trace API (used by the proxy). Both handlers share the same session state, so a rule set by a test is visible to the proxy immediately.
3. **SDKs** — thin clients for TypeScript, Python, Java, and Go that let your tests express intent in terms like `findInCase`, `mockOutbound`, `loadCaseFromFile`.

The **artifact** is `*.case.json` — a plain JSON document containing an array of OTLP-shaped traces. It's diffable, git-friendly, and can be edited by hand or by an LLM.

## What you can do with it

- **Capture** any HTTP traffic that passes through your sidecar into a case file.
- **Replay** a captured case as a deterministic mock for every dependency the app talked to.
- **Override selectively** — mock only `/fragment` while `/payments` still goes live.
- **Mutate before replay** — bump a timestamp, rotate a token, swap a masked credit card for a test value.
- **Run suites at scale** — `softprobe suite run suites/*.yaml --hooks hooks/*.ts` drives thousands of captures deterministically with a shared Node sidecar for hooks; emits JUnit XML for CI.
- **Keep one mental model** — even as more tooling lands, the core remains sessions, case files, and explicit SDK-authored mock rules.

## What it is not

- **Not a service mesh or routing control plane.** Istio / Linkerd stay responsible for service discovery, mTLS, and traffic policy. Softprobe is a test-time filter.
- **Not a framework-patching library.** Unlike snapshot mockers that monkey-patch `fetch` or `HttpClient`, Softprobe never touches your application code. It intercepts at the network layer.
- **Not a load generator.** It replays recorded sessions; it does not synthesize new traffic patterns.
- **Not a UI test tool.** The unit of capture is an HTTP request/response pair. Browser automation is out of scope.

## Who is this documentation for?

| Reader | Start here |
|---|---|
| "I have 10 minutes, convince me" | [Quick start](/quickstart) |
| "I want to capture a real session" | [Capture your first session](/guides/capture-your-first-session) |
| "I want to replay in my test file today" | [Replay in Jest](/guides/replay-in-jest), [pytest](/guides/replay-in-pytest), [JUnit](/guides/replay-in-junit), or [Go](/guides/replay-in-go) |
| "I need to run 10k cases nightly in CI" | [Run a suite at scale](/guides/run-a-suite-at-scale) |
| "I want to understand how it works first" | [Architecture](/concepts/architecture) |
| "I need to deploy the proxy" | [Deployment](/deployment/local) |

## How this platform evolved

Softprobe began as a per-framework mocker for Node.js. Patching every HTTP client (Express, Fastify, Axios, `fetch`, Postgres, Redis) turned out to be an endless maintenance tax — and it only ever worked for one language. The current design moves interception **below** the application, into the proxy, so the same capture artifact can be replayed against a Java service, a Python service, or a Go service with identical semantics.

The tradeoff is a modest increase in **operational surface** (you run an Envoy proxy next to the app) in exchange for **dramatic savings in test maintenance** and **real cross-language parity**.

---

**Next:** [Quick start →](/quickstart) or [Architecture →](/concepts/architecture)
