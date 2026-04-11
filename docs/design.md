# Softprobe Hybrid Platform: Design Document

**Status:** Draft for implementation planning  
**Audience:** Engineers implementing runtime, proxy extension, SDKs, and CLI  
**Related contracts:** [platform-architecture.md](./platform-architecture.md), [http-control-api.md](../spec/protocol/http-control-api.md), [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md), [session-headers.md](../spec/protocol/session-headers.md)

---

## 1. Executive summary

Softprobe Hybrid unifies **HTTP capture, replay, and rule-based dependency injection** behind a **proxy-first** data plane (Envoy + Softprobe WASM) and a **language-neutral control plane** (session, case, rules, policy). Language-level framework patching (Express, Fastify, `fetch`, database drivers, and so on) is **optional** and out of the default product path, to reduce implementation cost for a small team.

Recorded behavior is stored as **one JSON file per test case**, containing an ordered list of **OpenTelemetry–compatible trace payloads** (not NDJSON streams). Test code in **Jest, pytest, or JUnit** controls mocks by **creating a session**, **loading cases and rules**, and ensuring the application’s traffic is tagged with a **stable session identifier** so the proxy can consult the runtime on the inject path.

The **primary product surface for humans, CI, and AI agents** is the **`softprobe` CLI** (and thin language SDKs that wrap the same control operations). Documentation and examples should lead with CLI workflows; the HTTP control API remains the **contract** those tools **call** on the control runtime. **Proxy OTLP** ([proxy-otel-api.md](../spec/protocol/proxy-otel-api.md)) is an integration detail for proxy and **proxy backend** authors, not the default path for test authors.

This document specifies background, goals, concrete APIs, CLI shape, data artifacts, cross-language ergonomics, and **acceptance criteria** suitable for turning into an engineering task list.

---

## 2. Background

### 2.1 Prior state

Two implementations evolved in parallel:

- **JavaScript runtime (`softprobe-js`):** Strong deterministic replay and cassette concepts, but high maintenance cost: many framework and library patches (HTTP servers, clients, Postgres, Redis, and so on).
- **Proxy (`softprobe` / Envoy WASM):** Efficient transparent HTTP interception, OTEL-shaped inject/extract wire protocol, but injection lookup was not fully connected to a shared policy engine.

### 2.2 Problem

Instrumenting every framework is not viable as the **default** product for a startup. At the same time, **authentication and other control flows** are often HTTP-based and must be **mocked or replayed** during tests, not only outbound API calls. The platform needs:

- One **canonical** model for HTTP interactions and decisions.
- One **rule system** that applies to both capture and replay.
- A **simple way** for tests written in any mainstream language to **steer** injection without re-implementing matchers in each runtime.

### 2.3 Product thesis

| Layer | Responsibility |
|--------|-----------------|
| **Proxy (data plane)** | Intercept inbound and outbound HTTP; normalize request identity; call **proxy backend** for inject/extract per [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md); enforce passthrough / mock / error. |
| **Control runtime** | **HTTP control API** only: sessions, case load, rules, policy, fixtures for CLI/SDKs. **Does not** serve `/v1/inject` in the OSS `softprobe-runtime` layout. |
| **Proxy backend** | Serves **inject** and standard OTLP collector-style **extract** endpoints for the mesh (hosted example **`https://o.softprobe.ai`**); owns replay/match semantics on the data path. |
| **Language SDKs** | Thin clients: create session, set policy, load case, register rules, attach headers; optional helpers for codegen and assertions. |
| **CLI** | **Canonical** human, CI, and agent interface: doctor, sessions, capture, replay, inspect, export, codegen; **calls** the same HTTP control API as SDKs (it does not replace the runtime). |

### 2.4 Where the APIs run (two services)

- **`softprobe-runtime` (OSS):** implements the [HTTP control API](../spec/protocol/http-control-api.md) for CLI, SDKs, and tests. **v1 needs no database** if a single process and in-memory sessions are acceptable; add a datastore only for HA or restart survival (see [platform-architecture.md](./platform-architecture.md) §10.2).
- **Proxy backend:** implements [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md) for Envoy/WASM. Production default may be **`https://o.softprobe.ai`**; proxy Wasm config (`sp_backend_url` or equivalent) points **here**, not at the control runtime. The hosted backend exposes standard OTLP collector ingestion plus **`POST /v1/inject`** on the same OTLP trace schema.
- **Consistency:** Session and case data from the control API must drive correct replay at the proxy; **how** the proxy backend learns that state is **product integration** (see §14). Neither service is an Istio-style mesh control plane for Envoy routing—see [platform-architecture.md](./platform-architecture.md).

---

## 3. Goals

1. **Proxy-first HTTP:** Capture and inject **inbound and outbound** HTTP without requiring application code changes beyond routing traffic through the mesh and propagating session headers.
2. **Case artifacts:** Persist each scenario as **one JSON case file** with a **`traces` array** in **OTLP-compatible JSON** form (see §6), suitable for tooling, diffing, and optional export to collectors.
3. **Cross-language tests:** **Jest, pytest, and JUnit** (and similar) can **control injection** via a **small HTTP API** to the runtime and **session headers** on requests; no requirement to use a specific JS-only API in the application under test.
4. **Flexible rules:** Support **composable rules** (priority, scope, consume behavior, overrides) that combine with recorded traces, not a single flat mock table.
5. **Two execution modes:** **Replay** (deterministic playback + rules) and **Generate** (emit tests + fixtures using the same session and rule model).
6. **Contract-first:** Schemas and protocols in `spec`; proxy and language repos implement **versioned** contracts.
7. **CLI-first simplicity:** One obvious command-line entrypoint; stable **machine-readable** output for automation and AI agents; onboarding docs that favor **copy-paste CLI flows** over raw HTTP for most users.
8. **Progressive disclosure:** The **default happy path** is *create session → load case → run tests with session header*; composable **rules**, **strict policy**, and **case-embedded rules** are documented as advanced layers on top of that path.

### 3.1 Non-goals (v1)

- Mandatory patching of Express, Fastify, Axios, `fetch`, `pg`, Redis clients, and so on.
- Defining a second, parallel mock DSL unrelated to rules + OTEL-shaped spans.
- Replacing Istio/Envoy as the deployment model for the data plane.

### 3.2 Default happy path (document and implement first)

For **replay**, the **minimal** flow—what tutorials, golden examples, and agent prompts should assume unless the user asks for more:

1. Runtime reachable (local sidecar, testcontainer, or cluster service).
2. **`softprobe doctor`** (or equivalent) confirms runtime and schema compatibility.
3. **`softprobe session start --mode replay`** → obtain `sessionId` (prefer **env-export** or **`--json`** for scripts).
4. **`softprobe session load-case`** with the scenario’s **one JSON case file**.
5. Tests (or manual requests) hit the app with **`x-softprobe-session-id`** on **inbound** traffic; the mesh **propagates** it for **outbound** calls per deployment docs.

**Rules**, **policy toggles**, and **fixtures** are added only when the scenario needs overrides, strict blocking, or non-HTTP material. This ordering reduces cognitive load and support burden.

### 3.3 Operational complexity and mitigations

Proxy + mesh + runtime is **more moving parts** than an in-process mock library. The product mitigates that with:

- **`softprobe doctor`**: runtime URL, reachability, expected headers, **spec/schema version** alignment.
- **Opinionated local setups**: documented “one command” or compose profiles where feasible.
- **Single canonical CLI** (see §9): same verbs in CI and on a laptop, so docs and agents do not fork per language.

**Header propagation** remains the main integration risk: maintain **one golden-path diagram** (test → app with header → mesh → outbound) and a single **troubleshooting** section for when propagation must be explicit.

---

## 4. Core concepts

| Term | Definition |
|------|------------|
| **Session** | A bounded test run context: holds mode, policy, loaded case, active rules, and optional auth fixtures. Identified by `sessionId`. |
| **Case** | One JSON file: metadata + **`traces[]`** (OTLP-compatible) + optional embedded **`rules[]`** and **`fixtures[]`**. |
| **Rule** | A **when** matcher + **then** action (mock, replay-from-case, passthrough, error, patch recording). |
| **Policy** | Session defaults: e.g. strict external blocking, allowlist hosts, default action on miss. |
| **Inject lookup** | Proxy sends OTLP trace payloads to **proxy backend** (`POST /v1/inject` per [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md)); backend returns hit (response attributes) or miss (`404`). |
| **Extract** | Proxy asynchronously uploads observed traffic to OTLP collector-style endpoints such as `POST /v1/traces` on the **proxy backend**. |

---

## 5. Architecture

### 5.1 Control flow (replay mode)

```mermaid
sequenceDiagram
  participant T as Test (Jest/pytest/JUnit)
  participant R as Control runtime JSON API
  participant A as App workload
  participant P as Envoy + Softprobe WASM
  participant B as Proxy backend inject or extract API
  participant U as Upstream

  T->>R: POST /v1/sessions (mode=replay)
  T->>R: POST /v1/sessions/{sessionId}/load-case
  T->>R: POST /v1/sessions/{sessionId}/rules (optional)
  Note over R,B: Session or case sync to proxy backend is product-specific
  T->>A: Exercise SUT (headers include x-softprobe-session-id)
  A->>P: HTTP request
  P->>B: POST /v1/inject (OTLP trace payload)
  alt Hit: mock or replay
    B-->>P: 200 + response attributes
    P-->>A: Injected HTTP response
  else Miss + strict
    B-->>P: 404 or policy error
    P-->>A: Configured error response
  else Passthrough
    P->>U: Forward
    U-->>P: Live response
    P-->>A: Response
    P->>B: POST /v1/traces (extract)
  end
```

### 5.2 Split of responsibilities

- **Proxy:** Delegates inject/extract to the **proxy backend** over OTLP trace payloads; does not call the JSON control API on the request path.
- **Proxy backend:** Owns request-path **inject** resolution, extract handling, and replay/match semantics for mesh traffic (per [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md)).
- **Control runtime:** Owns the **HTTP control API** and in-memory (or durable) **session/case** state for tests and CLI; must stay **consistent** with what the proxy backend uses for replay (integration path in §14).
- **SDKs:** Serialize **session + headers**; optional ergonomics only.
- **CLI:** **Calls** the HTTP control API on the control runtime; preferred way for agents and operators to create sessions, load cases, and apply rule packs without hand-crafting JSON requests.

---

## 6. Case file format (replaces NDJSON cassettes)

### 6.1 File model

- **One file per case**, e.g. `cases/login-with-oauth.case.json`.
- Top-level shape aligns with [case.schema.json](../spec/schemas/case.schema.json): `version`, `caseId`, `traces`, optional `suite`, `mode`, `rules`, `fixtures`, `createdAt`.
- Each entry in `traces[]` follows the OTLP JSON profile in [case-otlp-json.md](../spec/protocol/case-otlp-json.md).

### 6.2 Traces array

- **`traces`** is an **array of OTLP-compatible trace documents**.
- **Recommended encoding:** JSON equivalent of OTLP **`ExportTraceServiceRequest`** / **`TracesData`** resource-spans structure as produced by standard OTEL SDKs or the Softprobe recorder, so the same payload can be:
  - written to disk,
  - sent to an OTEL backend,
  - re-used for inject span construction.

**Illustrative shape** (logical, not normative field-for-field):

```json
{
  "version": "1.0.0",
  "caseId": "checkout-happy-path",
  "suite": "payments",
  "mode": "replay",
  "createdAt": "2026-04-05T12:00:00Z",
  "traces": [
    {
      "resourceSpans": [
        {
          "resource": { "attributes": [{ "key": "service.name", "value": { "stringValue": "api" } }] },
          "scopeSpans": [
            {
              "spans": [
                {
                  "traceId": "…",
                  "spanId": "…",
                  "name": "HTTP POST",
                  "attributes": [
                    { "key": "sp.session.id", "value": { "stringValue": "sess_abc" } },
                    { "key": "sp.traffic.direction", "value": { "stringValue": "outbound" } },
                    { "key": "url.full", "value": { "stringValue": "https://api.stripe.com/v1/payment_intents" } }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ],
  "rules": []
}
```

**Normative mapping** for specific attributes remains aligned with [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md) (e.g. `sp.session.id`, `sp.traffic.direction`, `http.request.*`, `http.response.*`).

### 6.3 Optional embedded rules

Cases may ship **default rules** (e.g. redact tokens) that the runtime applies unless the session **explicitly disables** `case.rules` or supplies higher-priority overlays.

---

## 7. How tests control HTTP injection (Jest, pytest, JUnit)

### 7.1 Mechanism

Tests do **not** call Envoy directly. They:

1. **Start or attach to** a **Softprobe Runtime** process (local sidecar, testcontainer, or cluster service for advanced setups).
2. **Create a session** with desired `mode` (`capture` | `replay` | `generate`) and **policy**.
3. **Load a case** and/or **register rules** through the [HTTP control API](../spec/protocol/http-control-api.md).
4. Ensure **every HTTP request** that should participate carries:

   - `x-softprobe-session-id: <sessionId>` (required per [session-headers.md](../spec/protocol/session-headers.md))
   - Optional: `x-softprobe-case-id`, `x-softprobe-mode`, `x-softprobe-test-name`

5. Run the scenario. The **proxy** includes the session id in OTEL inject/extract spans so the runtime can correlate.

### 7.2 Jest / Node example (illustrative)

This example uses **strict external HTTP** and **session rule overlays** (see section 3.2 for the minimal happy path without those layers).

```typescript
import { Softprobe } from '@softprobe/sdk';

describe('checkout', () => {
  let sp: Softprobe;

  beforeAll(async () => {
    sp = await Softprobe.connect({ baseUrl: process.env.SOFTPROBE_RUNTIME_URL });
  });

  beforeEach(async () => {
    const session = await sp.sessions.create({
      mode: 'replay',
      policy: { externalHttp: 'strict', defaultOnMiss: 'error' },
    });
    await session.loadCase({ path: 'cases/checkout.case.json' });
    await session.rules.upsert([
      {
        id: 'stripe-override',
        priority: 10,
        consume: 'many',
        when: { direction: 'outbound', host: 'api.stripe.com', pathPrefix: '/v1/payment_intents' },
        then: { action: 'mock', response: { status: 200, json: { id: 'pi_test', status: 'succeeded' } } },
      },
    ]);
    sp.currentSession = session;
  });

  it('charges successfully', async () => {
    await request(app)
      .post('/checkout')
      .set('x-softprobe-session-id', sp.currentSession.id)
      .send({ amount: 1000 })
      .expect(200);
  });
});
```

**Key point:** The **application under test** must receive the session header on **inbound** calls (test client → app) and the mesh must **propagate** it for **outbound** calls (app → dependency) according to deployment rules. Where automatic propagation is impossible, the SDK documents **explicit header forwarding** for the test harness only (not patching every HTTP client library).

### 7.3 pytest example (illustrative)

```python
import os
import pytest
import requests
from softprobe import Client

@pytest.fixture
def softprobe_session():
    client = Client(base_url=os.environ["SOFTPROBE_RUNTIME_URL"])
    session = client.sessions.create(
        mode="replay",
        policy={"externalHttp": "strict", "defaultOnMiss": "error"},
    )
    session.load_case(path="cases/checkout.case.json")
    yield session
    session.close()

def test_checkout(softprobe_session):
    headers = {"x-softprobe-session-id": softprobe_session.id}
    r = requests.post("http://app-under-test/checkout", json={"amount": 1000}, headers=headers)
    assert r.status_code == 200
```

### 7.4 JUnit 5 example (illustrative)

```java
@ExtendWith(SoftprobeExtension.class)
class CheckoutTest {
  @SoftprobeSession(mode = "replay", casePath = "cases/checkout.case.json")
  SoftprobeSession session;

  @Test
  void chargesSuccessfully() {
    var client = HttpClient.newHttpClient();
    var req = HttpRequest.newBuilder(URI.create("http://app-under-test/checkout"))
        .header("x-softprobe-session-id", session.id())
        .POST(HttpRequest.BodyPublishers.ofString("{\"amount\":1000}"))
        .build();
    var res = client.send(req, HttpResponse.BodyHandlers.ofString());
    assertEquals(200, res.statusCode());
  }
}
```

### 7.5 Auth and non-HTTP setup

- **HTTP-based OAuth/OIDC/SSO:** Handled by **case traces + rules** on the relevant inbound/outbound HTTP interactions.
- **Non-HTTP secrets or session material:** Use **`POST /v1/sessions/{id}/fixtures/auth`** (see control API) to register **tokens, cookies, or metadata** the runtime can surface to matchers or codegen, without patching frameworks.

---

## 8. Dependency injection model (rules + policy)

### 8.1 Decision space

Runtime evaluation for each candidate HTTP exchange returns a **decision**:

| Decision | Meaning |
|----------|---------|
| `MOCK` | Return a constructed response (from rule or synthesized from case). |
| `REPLAY` | Return the **next matching** recorded response from the loaded case according to matcher + ordering. |
| `PASSTHROUGH` | Allow live upstream (explicit rule or allowlist). |
| `ERROR` | Fail the request (strict policy or rule). |
| `CAPTURE_ONLY` | Used in capture mode: record, always forward (policy-dependent). |

The proxy maps these to: **inject attributes** (mock/replay), **forward** (passthrough), or **local error response** (error).

### 8.2 Rule structure

Rules align with [rule.schema.json](../spec/schemas/rule.schema.json):

- **`id`:** Stable identifier for diffs and codegen.
- **`priority`:** Higher wins on conflict (explicit numeric total ordering).
- **`consume`:** `once` | `many` — controls whether a matching **replay** interaction is **dequeued** from the case.
- **`when`:** Matcher object (direction, service, host, method, path, pathPrefix, header predicates, body JSONPath subset, trace tags).
- **`then`:** Action + payload (response spec, status template, latency, fault injection).

**Example rule pack (YAML):**

```yaml
version: 1
rules:
  - id: block-unknown-external
    priority: 1000
    consume: many
    when:
      direction: outbound
      notHostSuffix: [.internal, localhost]
    then:
      action: error
      error:
        status: 599
        body: { "error": "external call blocked in strict mode" }

  - id: stripe-replay
    priority: 100
    consume: once
    when:
      direction: outbound
      host: api.stripe.com
      method: POST
      pathPrefix: /v1/payment_intents
    then:
      action: replay
```

### 8.3 Composition order

1. **Session policy defaults** (strictness, allowlists).
2. **Case-embedded rules** (shipped with recording).
3. **Session rules** (test-local overlays, highest priority wins on ties by `priority` field).

If two candidates share the same `priority`, the later composition layer wins
(`session rules` > `case-embedded rules` > `session policy defaults`); within a
single layer, later entries in the layer's document win.

### 8.4 Session revision and caching

Every mutating call (`load-case`, `rules`, `policy`, `fixtures`) bumps a **`sessionRevision`**. The proxy may cache inject results **only** when keyed by `(sessionId, sessionRevision, requestFingerprint)`.

---

## 9. CLI design (revised)

Design principles:

1. **CLI verbs map 1:1 to control-plane concepts** (session, case, rules, policy, export), not to one-off hacks.
2. **One canonical `softprobe` binary** (language-agnostic: speaks only HTTP to the runtime per [http-control-api.md](../spec/protocol/http-control-api.md)). Language repos ship **SDKs**, test helpers, and optional **thin shims** (for example `npx softprobe` delegating to the installed binary)—they **must not** introduce a second, divergent command vocabulary for the same operations.
3. **Onboarding spine:** `doctor` → `session start` → `session load-case` → run tests with session header; see §3.2.

### 9.1 Command reference

| Command | Purpose |
|---------|---------|
| `softprobe doctor` | Check runtime reachability, proxy headers, schema/spec versions; primary **first run** and CI preflight. |
| `softprobe session start --mode replay` | Create session; emit `sessionId` for use in env or scripts (see §9.2). |
| `softprobe session load-case --session $ID --file cases/x.case.json` | Load traces/rules from disk. |
| `softprobe session rules apply --session $ID --file rules/stripe.yaml` | Apply rule pack (**advanced**; optional on the default path). |
| `softprobe session policy set --session $ID --strict` | Toggle strict external HTTP (**advanced**). |
| `softprobe capture run --target http://app:3000 --out cases/new.case.json` | Orchestrated capture (wraps session `capture` + extract aggregation into one case file). |
| `softprobe replay run --session $ID` | Validate session + print inject statistics (optional dry-run). |
| `softprobe inspect case cases/x.case.json` | Summarize spans, hosts, directions, diff-friendly view. |
| `softprobe export otlp --case cases/*.case.json --endpoint $OTLP` | Push case traces to OTLP HTTP/gRPC endpoint. |
| `softprobe generate test --case cases/x.case.json --framework vitest|jest|pytest|junit` | Emit test skeleton + fixture references using the **same** session API. |

### 9.2 Flags, machine output, and agent ergonomics

**Common flags:** `--runtime-url`, `--json` (structured stdout for agents and CI), `--trace` (verbose diagnostic logging).

**`--json` output** (normative intent for v1 CLI):

- Single JSON document per invocation where practical (or documented stream of JSON objects if a command must emit multiple events).
- Include **`specVersion`** or **`schemaVersion`** (aligned with `spec/`) so agents detect drift.
- Use **stable field names**; breaking changes require a **version bump** and `doctor` visibility.

**Shell integration:** `session start` (and similar) should support emitting a line suitable for **`eval`** or **`source`**, for example `export SOFTPROBE_SESSION_ID=…`, in addition to `--json`, so humans and scripts share one command.

**Exit codes:** Document **stable** meanings (for example `0` success, non-zero for doctor failures, unknown session, runtime unreachable); agents rely on exit codes as much as on JSON bodies.

---

## 10. API summary (control vs data plane)

| API | Transport | Typical server |
|-----|-----------|----------------|
| Session / rules / policy / fixtures | JSON over HTTP ([http-control-api.md](../spec/protocol/http-control-api.md)) | **`softprobe-runtime`** (OSS control service) |
| Inject lookup / extract upload | OTLP traces ([proxy-otel-api.md](../spec/protocol/proxy-otel-api.md)) | **Proxy backend** (e.g. **`https://o.softprobe.ai`**) |

**Documented primary path for test authors and tooling:** **`softprobe` CLI** and language **SDKs**, both targeting the **control API** on the control runtime. Direct HTTP calls to the control API are for integrators, debugging, and contract tests.

The proxy uses **only** the OTLP inject/extract API toward the **proxy backend**. Test code never speaks OTLP to the proxy; that boundary stays **proxy ↔ proxy backend**.

---

## 11. Security and safety

- **Session ids** are capabilities: treat as secrets in shared environments; support **short TTL** and **explicit close**.
- **Strict mode** must default to **fail closed** for unexpected outbound traffic in CI.
- **Redaction rules** should run on **extract** path before persistence or export.

---

## 12. Acceptance criteria

### 12.1 Artifacts

- [ ] A valid **case file** validates against `case.schema.json` and contains **`traces` as an array** of OTLP-compatible JSON documents.
- [ ] Documented **golden example** case files in `spec/examples/cases/` match the schema.

### 12.2 Control runtime

- [ ] `POST /v1/sessions` returns a `sessionId` and initial `sessionRevision`.
- [ ] `load-case`, `rules`, and `policy` endpoints bump `sessionRevision` monotonically.

### 12.3 Proxy backend

- [ ] Inject lookup resolves **rules + case replay** in the documented composition order.
- [ ] `consume: once` replay entries are **dequeued** exactly once per matching request.

### 12.4 Proxy integration

- [ ] For tagged traffic, proxy calls **`POST /v1/inject`** on the **configured proxy backend** and honors **hit vs 404** per [proxy-otel-api.md](../spec/protocol/proxy-otel-api.md).
- [ ] Extract path sends **`POST /v1/traces`** without blocking the request path beyond configured timeouts.

### 12.5 Cross-language

- [ ] Reference tests demonstrate **Jest**, **pytest**, and **JUnit** creating a session and passing **`x-softprobe-session-id`** through to the SUT.
- [ ] Failure modes (runtime down, unknown session, strict miss) produce **actionable errors** in each SDK.

### 12.6 CLI

- [ ] `softprobe doctor` and `softprobe session start` work against a local runtime with **`--json` output** suitable for automation (includes **spec/schema version** field).
- [ ] `softprobe session start` supports **shell-friendly** emission of `sessionId` (for example `export SOFTPROBE_SESSION_ID=…`) in addition to JSON.
- [ ] **Exit codes** for common failures are documented and stable enough for CI and agents.

### 12.7 Codegen

- [ ] Generated tests compile and use **only** public SDK + session APIs (no private protobuf fields in user code).

---

## 13. Implementation phases (task-list friendly)

| Phase | Scope |
|-------|--------|
| **P0** | Finalize JSON schemas for case traces OTLP profile; golden fixtures; **control** runtime session store (HTTP API only). |
| **P1** | Proxy ↔ **proxy backend** integration; inject resolver and extract on backend; strict policy; extract to case file writer (one file per case); control↔backend consistency. |
| **P2** | JS + Python + Java SDK thin clients; Jest/pytest/JUnit examples; **canonical language-agnostic `softprobe` CLI** (language repos provide SDKs and optional shims only). |
| **P3** | Codegen; OTLP export from case files; performance caching with sessionRevision. |
| **P4** | Optional deep instrumentation packages (`@softprobe/js-http-hooks`, and so on) **behind** feature flags. |

---

## 14. Open questions

- **OTLP JSON profile for case files:** Case artifacts need a **minimal, documented subset** of OTLP JSON (field naming, required attributes for HTTP identity, and **size limits**) so validation, diffing, and AI-generated cases stay unambiguous. **Resolved in `spec/protocol/case-otlp-json.md`** as the canonical on-disk shape; keep “full OTLP” export as an optional path. The proxy backend accepts OTLP protobuf and JSON for `/v1/inject`, but the on-disk case profile should still be narrowed and documented.
- Whether **inbound** replay should support **full duplex** streaming in v1 or scope to **request/response** HTTP only.
- Multi-tenant runtime: **namespace** session ids per org vs global UUIDs.
- **Control runtime ↔ proxy backend:** How `load-case` / rules / policy updates reach **`https://o.softprobe.ai`** (or self-hosted proxy backend) so inject uses the same session state—push API, pull, shared account store, or combined deployment. Document the **supported** integration for each product tier.

---

## 15. Document history

| Version | Date | Notes |
|---------|------|--------|
| 0.1 | 2026-04-05 | Initial hybrid design: case JSON + OTLP traces, proxy-first, cross-language control, rules/CLI revision |
| 0.2 | 2026-04-11 | CLI-first and simplicity: canonical `softprobe` binary, default happy path, agent/CI JSON + exit codes, OTLP JSON profile direction |
| 0.3 | 2026-04-11 | Softprobe Runtime service: single process for control + proxy APIs; Go/Rust recommendation; CLI as client; K8s Deployment pattern (see `docs/platform-architecture.md`, `docs/repo-layout.md`) |
| 0.4 | 2026-04-11 | Split **control runtime** (OSS JSON API, in-memory OK) vs **proxy backend** (inject/extract, e.g. `https://o.softprobe.ai`); datastore guidance; open integration question |
