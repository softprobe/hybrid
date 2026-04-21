# FAQ

Common questions we hear from developers evaluating and adopting Softprobe.

## Product

### How is Softprobe different from VCR / nock / WireMock / Mockserver?

Those tools intercept HTTP in the **application process** (or in a standalone mock server the app talks to). Softprobe intercepts in the **sidecar proxy**, one layer below. That means:

- **Language-neutral:** one capture file replays identically from Jest, pytest, JUnit, and `go test`.
- **No framework patching:** your app code doesn't import a mock library or a new HTTP client.
- **Real network semantics:** retries, timeouts, mTLS, proxy headers, trace propagation — all exercised as in production.

The tradeoff is that you run an Envoy sidecar (locally via Docker, in production via Istio) instead of nothing. For teams that already run a mesh, this is free.

### How does Softprobe compare to Hoverfly / Diffblue / Speedscale?

| Aspect | Softprobe | Hoverfly | Speedscale |
|---|---|---|---|
| License | Apache 2.0 OSS | Apache 2.0 OSS | Commercial |
| Interception | Envoy + WASM sidecar | Dedicated reverse proxy | Sidecar or agent |
| Case format | OTLP JSON (standard) | Custom JSON | Custom |
| SDKs | 4 languages | 1 (Go client) | No first-party SDK |
| Strict policy | Built-in | Limited | Built-in |
| CLI suite runner | Yes | No | Yes |

Softprobe differentiates on OTLP-native case files (use any OTEL tool to inspect or export) and first-party cross-language SDKs with identical APIs.

### Do I need Kubernetes to use Softprobe?

No. The local Docker Compose stack runs everywhere Docker runs. Kubernetes is useful for production canaries, shared runtime infrastructure in CI, and Istio-integrated deployments. For writing and running tests on a laptop, `docker compose up` is enough.

### Does Softprobe support gRPC / websockets / GraphQL?

- **GraphQL:** yes — it's HTTP. Capture and replay work as-is.
- **gRPC:** **roadmap.** The WASM filter has some initial support; v1 targets HTTP/1.1 and HTTP/2 request-response semantics.
- **Websockets:** not in v1. The unit of capture is a request/response pair.
- **Server-sent events:** partial — single-connection SSE captures as one response; long-running streams are roadmap.

### Can I capture production traffic?

Yes, with care:

1. Use a dedicated capture-only runtime, not your test runtime.
2. Sample at 0.1–1% of traffic with the `x-softprobe-session-id` header.
3. Apply redaction rules before data hits storage.
4. Review captures before committing to git.

The WASM filter mirrors bytes asynchronously — the request path itself is unaffected by capture.

### How large can a case file be?

Technical limit: 512 MB. Practical limit: **under 1 MB per scenario**. If your cases grow past that, you're probably capturing multiple scenarios as one — split them.

### Does replay touch the real internet?

By default, only **unmocked** outbound calls do. Set `externalHttp: strict` to block all unmocked outbound — then any test that would've called the real internet fails explicitly.

## Architecture

### Why did the runtime stop doing replay selection?

Earlier versions let the runtime walk captured traces to pick a replay response. Four problems:

1. Ambiguity surfaced at test runtime, not at authoring time.
2. You couldn't mutate the response (timestamp, token).
3. The runtime grew a query engine that differed subtly across languages.
4. Cross-language matcher parity required duplicated work.

Moving selection into the SDK via `findInCase` fixes all four. The runtime is now a simple `when`/`then` matcher.

### Why does the SDK need a local copy of the case?

`findInCase` is synchronous and in-memory. A round-trip to the runtime would break that guarantee, and it would make test-authoring feedback loops slow. The runtime also keeps a copy so case-embedded rules can apply, but the SDK's copy is what powers `findInCase`.

### How do you keep the OSS runtime and hosted runtime in sync?

They are **the same binary**. Hosted adds a durable datastore, per-org auth, and capture-to-object-storage — all built on top of the same control API and OTLP handlers. Every commit to `softprobe-runtime` ships to both.

### Why Envoy + WASM instead of a standalone reverse proxy?

Envoy already handles TLS, HTTP/2, retries, load balancing, and trace propagation correctly. Reimplementing any of that is a long tail of bugs. The WASM extension lets us add Softprobe-specific logic without forking Envoy and without running a separate HTTP proxy.

For teams that don't use Envoy, a standalone "Softprobe Proxy" binary is on the roadmap — essentially Envoy preconfigured.

## SDKs

### Do all four SDKs have feature parity?

Yes, at the level of `Softprobe`, `SoftprobeSession`, `findInCase`, `findAllInCase`, `mockOutbound`, `clearRules`, `setPolicy`, and `close`. The TypeScript SDK is the reference; Python / Java / Go ports are validated against the same test fixtures.

Ergonomic extras (framework integrations, test-helper packages) can lead or lag across languages by a minor version. Check each SDK reference page.

### What happens on a network partition between SDK and runtime?

SDK calls throw `RuntimeError`. Your test fails cleanly. The runtime keeps the session in its in-memory store until another call re-establishes contact or the session is closed.

### Can I run tests without internet access?

Yes. Everything runs locally — runtime, proxy, app, test code. No Softprobe component calls out to the internet during test execution.

### Which Node versions are supported?

Node 20 and 22 LTS. Node 18 works but is not tested.

### Which Python versions are supported?

Python 3.9 through 3.12. 3.13 is validated pre-release.

### Which Java versions are supported?

Java 17 and 21. Java 11 and 8 are not supported (we use records, pattern-matching, and `java.net.http.HttpClient`).

### Which Go versions are supported?

Go 1.22 and 1.23.

## Operations

### Is the runtime production-grade?

The v1 OSS runtime is suitable for **test and CI workloads**. For production canary capture or multi-region HA, use the hosted service or wait for v0.6 (which adds Redis / Postgres-backed session stores).

### What's the memory footprint?

Idle: ~50 MB. Active: ~200 MB per 1000 concurrent sessions. A 1-core, 512 MB container handles hundreds of concurrent tests comfortably.

### Does Softprobe log request bodies?

Only when capturing. Regular runtime operation logs session ids and counts, not payload contents.

### Can I run multiple runtime instances?

In v1 OSS, no — sessions are in-process. In hosted, yes — the storage layer handles it.

### How do I debug "replay works locally but not in CI"?

1. Run `softprobe doctor` on the CI runner.
2. Compare the captured case file used in CI with the one used locally — they may drift.
3. Check `x-softprobe-session-id` actually reaches the proxy (see [Session headers](/reference/session-headers#debugging-header-propagation)).
4. Check the SUT is routing egress through the proxy (`EGRESS_PROXY_URL`).

## Licensing

### Under what license is Softprobe released?

**Apache-2.0** for all OSS components: runtime, SDKs, proxy WASM, CLI, and this documentation. Commercial-friendly. No CLA required for contributions up to 500 lines; larger contributions require a DCO sign-off.

### Can I use Softprobe in a commercial product?

Yes. Apache-2.0 permits commercial use, distribution, modification, and private/public use. Trademark use (logo, name) requires written permission for non-incidental use.

### Does the hosted service require attribution?

No. Hosted is a commercial service; there is no attribution requirement when using it.

## Contributing

### I found a bug. What do I do?

File an issue at [github.com/softprobe/softprobe](https://github.com/softprobe/softprobe/issues) with:

- The output of `softprobe doctor --verbose`.
- The case file (if safe to share) or a minimal synthetic reproducer.
- The exact commands you ran.

### How can I contribute?

Start with [`CONTRIBUTING.md`](https://github.com/softprobe/softprobe/blob/main/CONTRIBUTING.md). Good first issues are tagged `good-first-issue`. For larger changes, open a design discussion first.

### Is there a community?

Yes — [softprobe.dev/community](https://softprobe.dev/community). Slack + weekly office hours.

---

## Still have questions?

- **Docs:** see the sidebar.
- **GitHub discussions:** [github.com/softprobe/softprobe/discussions](https://github.com/softprobe/softprobe/discussions).
- **Email:** `hello@softprobe.dev` (response within one business day).
