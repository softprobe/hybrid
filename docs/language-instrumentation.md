# Language-level instrumentation (hybrid plane)

**Status:** design note (roadmap and product positioning)  
**Audience:** Engineers working on `softprobe-js`, future language runtimes, and docs  
**Canonical hybrid design:** [design.md](./design.md)  
**Proxy OOB posture (do not fan bodies into customer APM):** [proxy-integration-posture.md](./proxy-integration-posture.md)

---

## 1. Two instrumentation planes, one product

Softprobe is **hybrid**: customers may use **either** or **both** of the following.

| Plane | What it is | Strengths | Limits |
|--------|----------------|-------------|--------|
| **Proxy** | Envoy / Istio **WASM** + `softprobe-runtime` ([proxy-otel-api.md](../spec/protocol/proxy-otel-api.md)) | No application code changes for HTTP; aligns with **W3C Trace Context** and mesh routing | **HTTP request/response** only in the current product scope |
| **Language** | In-process hooks (today primarily **`softprobe-js`**) | **Non-HTTP** dependencies (Postgres, Redis, …); precise in-process bodies; works **without** a service mesh | **N×M** packages, versions, and frameworks; ongoing maintenance |

The **default** HTTP capture/replay path in the monorepo design is **proxy-first** plus the **unified runtime** and **case JSON** artifacts ([design.md](./design.md)). **Language-level** patching is **optional** and must not be mandatory for v1 onboarding (`design.md` §3.1).

---

## 2. When to recommend which plane

- **Recommend proxy** when the workload is already behind **Istio** (or similar), HTTP is enough, and the team already propagates **`traceparent` / `tracestate`** (including session correlation per [session-headers.md](../spec/protocol/session-headers.md)).
- **Recommend language** instrumentation when the team needs **Redis / Postgres / other** protocols captured or replayed in-process, or there is **no** mesh, or the language has **no** Softprobe proxy story yet.
- **Recommend proxy (or wait)** when the language is **not** supported in-tree (for example **Rust**, **C++**) until a mesh-based HTTP path is viable; language SDKs can still drive **sessions and rules** from tests.

---

## 3. Product economics and OTel

Maintaining separate integrations for **every** server framework (**Express 4/5**, **Fastify** major lines, …) does not scale for a small commercial team. The **OpenTelemetry** ecosystem spreads that cost across vendors and contributors.

Softprobe should **not** try to “out-OTel OTel” for breadth. Instead:

- Prefer **thin** language surfaces that align with **runtime sessions**, **rules**, and **case files** (same control API as other SDKs).
- Where in-process capture remains justified, prefer **lowest-level** hooks when possible (for example Node **`http` / `https`** server APIs) so **one** integration covers many frameworks built on `http.Server`, instead of one patch per framework. Treat this as an **engineering direction** to evaluate during the Node port, not a hard commitment for every edge case.

---

## 4. Current `softprobe-js` legacy path (to be removed)

The package still contains an earlier **Node-centric** stack documented in [`softprobe-js/design.md`](../softprobe-js/design.md), [`design-cassette.md`](../softprobe-js/design-cassette.md), and the **cutover plan** in [`softprobe-js/design-node-port.md`](../softprobe-js/design-node-port.md), plus related files:

- **OpenTelemetry context** carries Softprobe mode, trace id, cassette, matcher.
- **NDJSON** cassettes: one file per trace under `{cassetteDirectory}/{traceId}.ndjson`.
- **Init** reads **`SOFTPROBE_CONFIG_PATH`** / **`.softprobe/config.yml`** and applies **per-package** patches (Express, Fastify, fetch, Postgres, Redis, …).

That path is **legacy** and scheduled for removal under the Node language-plane cutover. The **canonical** TypeScript surface is the **runtime client**: `startSession`, `loadCaseFromFile`, `findInCase`, `mockOutbound`, `close` (see [`softprobe-js/README.md`](../softprobe-js/README.md)).

---

## 5. Target Node language plane (clean cutover)

Align the Node implementation with the **same** control plane and artifacts as the rest of the hybrid platform:

1. **Control plane:** **`softprobe-runtime`** HTTP API only for session lifecycle, rules, policy, fixtures — no standalone Softprobe-only YAML as the **authoritative** product configuration for new users.
2. **Artifacts:** one **`*.case.json`** per scenario per [`spec/schemas/case.schema.json`](../spec/schemas/case.schema.json) (OTLP-compatible traces in JSON), not NDJSON-on-disk as the documented happy path.
3. **Remove:** NDJSON-first workflows, cassette directory as the primary storage story, and “diff the `.ndjson` file” as the main CLI narrative. Do not keep a backward-compatibility runtime path.

**Relationship to customer APM:** language capture may still attach bodies to spans **for Softprobe case assembly** or debugging, but the same **truncation and billing** constraints apply as in [proxy-integration-posture.md](./proxy-integration-posture.md). Do not document “send full bodies to Datadog” as the default.

---

## 6. Node port checklist (for `tasks.md` / future phases)

Use this as a backlog outline; individual tasks live in [`tasks.md`](../tasks.md) parking lot.

- [ ] Session + rules + policy driven only via **runtime** client (align with [`http-control-api.md`](../spec/protocol/http-control-api.md)).
- [ ] Capture path produces **case JSON** (or streams to runtime endpoints that persist case-shaped data), and NDJSON capture sinks are removed.
- [ ] Replay path reads **loaded case** + **session rules** consistent with proxy inject semantics; NDJSON replay path removed.
- [ ] Remove **`SOFTPROBE_CONFIG_PATH`** / cassette-directory as supported product defaults; migrate examples under `softprobe-js/examples/`.
- [ ] Remove broad per-framework patch dependence from the default language path; keep only minimal hooks needed for the runtime-backed flow.

---

## 7. Related links

- [design.md §2.5](./design.md#25-instrumentation-planes-proxy-vs-language) — Instrumentation planes
- [softprobe-js/design-node-port.md](../softprobe-js/design-node-port.md) — PD6.5 Node port: control API, case JSON, **no backward compatibility**, removal criteria
- [proxy-integration-posture.md](./proxy-integration-posture.md)
- [repo-layout.md](./repo-layout.md)
- [softprobe-js/design-proxy-first.md](../softprobe-js/design-proxy-first.md)
