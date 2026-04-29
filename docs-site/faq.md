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
| License | Softprobe Source License 1.0 (server) + Apache-2.0 (SDKs) | Apache 2.0 OSS | Commercial |
| Interception | Envoy + WASM sidecar | Dedicated reverse proxy | Sidecar or agent |
| Case format | OTLP JSON (standard) | Custom JSON | Custom |
| SDKs | 4 languages | 1 (Go client) | No first-party SDK |
| Strict policy | Built-in | Limited | Built-in |
| CLI suite runner | Yes (YAML + Node hook sidecar, JUnit/HTML) | No | Yes |

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

### What runtime should I use?

Use the hosted runtime at `https://runtime.softprobe.dev`. The official docs do
not require installing or operating a runtime process.

### Why Envoy + WASM instead of a standalone reverse proxy?

Envoy already handles TLS, HTTP/2, retries, load balancing, and trace propagation correctly. Reimplementing any of that is a long tail of bugs. The WASM extension lets us add Softprobe-specific logic without forking Envoy and without running a separate HTTP proxy.

For teams that don't use Envoy, a standalone "Softprobe Proxy" binary is on the roadmap — essentially Envoy preconfigured.

## SDKs

### Do all four SDKs have feature parity?

They are converging on the same core session/case/replay model, with the
TypeScript SDK as the current reference. Exact parity for every helper and test
integration should be checked against each SDK reference page rather than
assumed.

Framework-specific extras and test-helper packages may lead or lag across
languages.

### What happens on a network partition between SDK and runtime?

SDK calls throw `RuntimeError`. Your test fails cleanly. Hosted sessions remain
available until they are closed or expire.

### Can I run tests without internet access?

Not with the official hosted-runtime setup. Your test environment needs HTTPS
egress to `runtime.softprobe.dev`.

### Which Node versions are supported?

Node 20 and 22 LTS. Node 18 works but is not tested.

### Which Python versions are supported?

Python 3.9 through 3.12. 3.13 is validated pre-release.

### Which Java versions are supported?

Java 17 and 21. Java 11 and 8 are not supported (we use records, pattern-matching, and `java.net.http.HttpClient`).

### Which Go versions are supported?

Go 1.22 and 1.23.

## Operations

### Is the hosted runtime production-grade?

The hosted runtime is the official target for tests, CI, and controlled canary
capture. It provides authenticated, tenant-scoped session state and case storage.

### What's the memory footprint?

Runtime capacity is managed by Softprobe. For customer workloads, size your app
and proxy normally and ensure they can reach `runtime.softprobe.dev`.

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

Softprobe uses a **dual-license split** — see [`LICENSING.md`](https://github.com/softprobe/softprobe/blob/main/LICENSING.md) for the full path map.

- **Server-side** (`softprobe-runtime`, `softprobe-proxy`, the `softprobe` CLI): [**Softprobe Source License 1.0**](https://github.com/softprobe/softprobe/blob/main/LICENSE) (SPDX: `LicenseRef-Softprobe-Source-License-1.0`). This is a source-available license derived from the [Functional Source License 1.1](https://fsl.software) with a broader non-compete clause — it restricts any competing use (hosted, on-premises, bundled, or rebranded), not just hosted-service redistribution. Free to use for your own internal business, research, and consulting. Every release auto-re-licenses to Apache-2.0 two years after its publication date and the non-compete restriction lifts completely at that point.
- **Client SDKs and protocol schemas** (`softprobe-js`, `-python`, `-java`, `-go`, `spec/`): plain [**Apache License, Version 2.0**](https://www.apache.org/licenses/LICENSE-2.0). Embed them in proprietary commercial products with no additional restrictions.

No CLA required for contributions up to 500 lines; larger contributions require a DCO sign-off.

### Can I use Softprobe in a commercial product?

**Yes, for the SDKs** — they're Apache-2.0, so embedding them in commercial products is unrestricted.

**For the server-side, it depends on what kind of commercial product.** The Softprobe Source License permits commercial internal use, commercial consulting, and redistribution — but it prohibits using the server components in a product or service offered to third parties that competes with Softprobe (HTTP/RPC capture, replay, session-based mocking, service virtualization, or record-and-replay regression testing). That restriction applies whether the competing product is a hosted service, an on-premises appliance, a bundled component inside a larger product, or a rebranded fork. If your use case falls in that category, contact `sales@softprobe.io` for commercial licensing.

### Can I run Softprobe on my own infrastructure in production?

Yes, unreservedly. Running Softprobe internally — including in production CI, for your own company's workloads, at any scale, across any commercial domain — is a Permitted Purpose under the Softprobe Source License. The non-compete restriction only engages when the server components are redistributed or offered to third parties as a replay-testing product or service.

### Can I build a commercial product that uses captured traffic?

Yes, in general — as long as the product isn't *itself* a replay-testing / traffic-mocking / service-virtualization competitor. Concretely:

- Building an observability product that consumes Softprobe case files for analytics: **fine** (not a Competing Use).
- Building a CI tool that embeds the `softprobe` CLI to run your customers' tests: **also generally fine** if your product's primary value is CI orchestration rather than replay testing itself; the CLI binary is distributed only as a supporting component. Contact us if you want certainty.
- Building a "managed replay testing service" where customers upload cases and you replay them: **Competing Use** — you need a commercial license.
- Building an on-premises appliance that wraps Softprobe and sells replay testing as its headline feature: **Competing Use**.

When in doubt, email `legal@softprobe.io` before shipping.

### Why not stock FSL 1.1 or Apache-2.0 for the server?

Apache-2.0 on the server side means a larger cloud provider or an on-prem vendor can take the code, rebrand it, out-spend us on marketing, and never contribute back. Stock FSL 1.1 only plugs the *hosted-service* version of that loophole — it still permits on-premises rebranding and product bundling, which is also a problem for a company whose product is installable software.

The Softprobe Source License plugs both loopholes with a broader Competing Use definition while preserving FSL's other features verbatim: the 2-year Apache-2.0 conversion, the explicit internal-use / research / consulting permissions, and the patent grant structure. Older releases always grow more permissive over time — nothing is retroactively clawed back.

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
