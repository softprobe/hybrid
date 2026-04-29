---
layout: home

hero:
  name: Softprobe
  text: Record real traffic. Replay deterministic tests.
  tagline: A proxy-first capture/replay platform for TypeScript, Python, Java, and Go — no framework patching, no hand-written mocks.
  image:
    src: /hero.svg
    alt: Softprobe
  actions:
    - theme: brand
      text: Quick start
      link: /quickstart
    - theme: alt
      text: What is Softprobe?
      link: /introduction
    - theme: alt
      text: GitHub
      link: https://github.com/softprobe

features:
  - icon: 🎯
    title: Capture once, replay everywhere
    details: A short session recorded from staging becomes a committed <code>.case.json</code> file that four SDKs (TS/Py/Java/Go) can replay identically in CI, on a laptop, or in a Kubernetes test cluster.
    link: /concepts/capture-and-replay
    linkText: How it works

  - icon: 🧩
    title: No framework patching
    details: A single Envoy + Softprobe WASM sidecar intercepts ingress and egress HTTP. Your app code does not change — outbound calls just need standard OpenTelemetry headers.
    link: /concepts/architecture
    linkText: Architecture

  - icon: 🔒
    title: Deterministic by default
    details: Replay returns the same response bytes every run. Strict policy fails the test if any unexpected external call would leave the sandbox — no flaky "forgot to mock" failures.
    link: /concepts/rules-and-policy
    linkText: Rules & policy

  - icon: ⚡
    title: Scale beyond unit tests
    details: Orchestration layers such as <code>softprobe suite run</code> build on the same hosted runtime, proxy, and case format.
    link: /guides/run-a-suite-at-scale
    linkText: Run a suite

  - icon: 🗣
    title: One control plane, many languages
    details: The hosted runtime speaks one HTTP control API to every SDK and the CLI. Write tests in your team's language; the behavior is identical across Jest, pytest, JUnit, and <code>go test</code>.
    link: /reference/http-control-api
    linkText: Control API

  - icon: 🧠
    title: AI-agent friendly
    details: All CLI commands support <code>--json</code> output and stable exit codes. Case files are plain JSON (OTLP-compatible) — diffable, greppable, and safe for LLMs to rewrite.
    link: /reference/cli
    linkText: CLI reference
---

<div class="vp-doc" style="max-width: 960px; margin: 3rem auto 0; padding: 0 24px;">

## Who uses Softprobe?

Teams that ship HTTP APIs and depend on other HTTP APIs — payment processors, auth providers, search services, internal microservices — and want **integration-level confidence without integration-level flake**.

## The one-paragraph version

You run an Envoy sidecar with the Softprobe WASM filter next to your app. The sidecar captures every HTTP request and response on both ingress and egress and streams them to the **Softprobe Runtime** as OTLP traces, which writes one JSON **case file** per session. Later, your tests start a **replay session**, load that case, and use the **SDK** to register concrete mock responses. The sidecar serves those mocks back in place of the real upstreams — deterministically, with zero changes to the application code under test.

## Ready to try?

<div class="cta-row">

- **2 minutes:** [Quick start →](/quickstart)
- **10 minutes:** [Capture your first session →](/guides/capture-your-first-session)
- **Reading:** [Architecture overview →](/concepts/architecture)

</div>

</div>

<style>
.cta-row ul { list-style: none; padding-left: 0; }
.cta-row li { margin: 0.5rem 0; font-size: 1.05rem; }
</style>
