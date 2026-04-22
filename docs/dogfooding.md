# Dogfooding `softprobe`

> **Status:** design note. Delivery plan lives in [`tasks.md` Phase PD7](../tasks.md). Normative behavior for the CLI, control API, and case files stays in [`design.md`](./design.md).

## 1. Executive summary

`softprobe` is a capture-and-replay framework for HTTP dependencies. Most of
our own stack is exactly the thing it was built to test: the CLI and every
SDK make HTTP calls to a server (`softprobe-runtime`) over a documented JSON
contract. This note describes how we turn that happy coincidence into a
first-class test strategy without letting the tool silently mark its own
homework.

The core idea: record **one canonical control-plane conversation** against a
known-good runtime, commit the resulting case file into `spec/examples/`,
and replay it from the CLI tests and from each SDK's parity test. The case
file becomes both a regression fixture and living documentation of our wire
protocol.

## 2. Motivation

Today, every layer of the stack hand-rolls fake HTTP servers to avoid
booting a real runtime in unit tests:

- `cmd/softprobe/*_test.go` stands up an `httptest.Server` that impersonates
  the control API (see `cmd/softprobe/auth_test.go` for a representative
  example — it reimplements the session lifecycle to assert one header).
- `softprobe-js/src/__tests__/*.ts` uses in-process fakes per test file.
- `softprobe-python/tests/test_client.py`, `softprobe-java`'s `ClientTest`,
  and `softprobe-go/softprobe/auth_test.go` all do the same.

Each fake is a local copy of the control-plane contract. They drift the
moment we change `softprobe-runtime/internal/controlapi/` and nobody
notices until an end-to-end job fails in CI minutes later — or worse, until
a user upgrades the SDK and hits a 404.

Dogfooding collapses those N local copies into **one** recorded artifact
that the real runtime produced. When the contract changes, the case file
changes, and every SDK that hasn't kept up fails loudly.

## 3. Scope: what to dogfood (and what not to)

### In scope

1. **CLI control-plane flow** — `softprobe` binary driving the runtime over
   the JSON control API ([`design.md` §4.3 Two API surfaces](./design.md)
   and [§4.4 Control API reference](./design.md)).
2. **SDK control-plane parity** — each of `softprobe-js`, `softprobe-python`,
   `softprobe-java`, `softprobe-go` replays the same case to prove they all
   speak the same contract. This directly supports the cross-language
   acceptance criterion in [`design.md` §12.5](./design.md).
3. **CLI docs snippets** — optional, deferred layer: every copy-paste CLI
   flow in `docs-site/guides/` becomes a suite case so docs drift fails CI
   (ties into [`design.md` §3.2 Default happy path](./design.md) and the
   CLI-first principle called out in `AGENTS.md` section 12).

### Out of scope

- **Proxy → runtime OTLP traffic.** The proxy makes real outbound HTTP
  calls — `POST /v1/inject` on the request hot path and `POST /v1/traces`
  asynchronously for extract ([`design.md` §4.3](./design.md),
  [`spec/protocol/proxy-otel-api.md`](../spec/protocol/proxy-otel-api.md),
  implementation in [`softprobe-proxy/src/otel.rs`](../softprobe-proxy/src/otel.rs)).
  They're out of scope for dogfood not because they don't exist, but
  because they use OTLP **protobuf** while our rule matchers key on
  URL/method/body **JSON shape**. Recording and replaying protobuf span
  signatures would be a new matcher dimension, not a dogfood. Revisit if
  and when the case format grows protobuf-aware rules.
- **Runtime internals.** `softprobe-runtime/internal/store/` has no
  outbound HTTP worth recording; cover it with native Go tests.
- **Hook sidecar protocol.** The Node sidecar talks to the Go CLI over
  newline-delimited JSON on stdio (not HTTP) and is already covered by
  `e2e/cli-suite-run/`.

## 4. Do we need a stable released build first?

**No — but we need a *frozen reference*, which is a weaker requirement than
a release.**

The invariant the dogfood tests depend on is: "the case was recorded against
a known-good version of the runtime that we trust." Ways to satisfy this,
strongest to weakest:

| Reference source | When appropriate |
|---|---|
| Published release tag (e.g. `ghcr.io/softprobe/softprobe-runtime:v0.5.0`) | Once [`tasks.md` PD5.3a](../tasks.md) ships the runtime image. Use this for SDK parity tests so every SDK CI can pull the same pinned image. |
| Protected `main@<sha>` that all non-dogfood CI jobs proved green | Starting state. Sufficient for in-repo CLI replay tests while PD5 is in flight. |
| Locally built HEAD | **Never.** Guarantees a race between code changes and case refreshes. |

Recommendation: start with protected-main and graduate to release tags as
PD5 lands. The delivery plan below is structured so neither PD5.3 nor a
published SDK are prerequisites for landing the first dogfood test.

The chicken-and-egg concern ("can't test capture by using capture") is
handled by keeping the native test suites in place — the dogfood tests are
**additive**. A dogfood test can only be trusted once the native tests on
the recording commit are green. Make that ordering explicit in CI.

## 5. Best practices

These are the invariants any implementation must preserve. Violating any
of them silently turns dogfooding into theater.

1. **Dogfood the public contract, never internals.** Replay against
   [`spec/protocol/http-control-api.md`](../spec/protocol/http-control-api.md)
   and the JSON Schemas under [`spec/schemas/`](../spec/schemas/), not
   against private Go types. Otherwise the case file becomes a mirror of
   our current implementation rather than the contract.

2. **Two parallel test stacks — additive, not replacement.** Keep the
   existing `httptest`/real-runtime tests *and* add the dogfood replay
   tests. Dogfood catches contract drift; the native tests catch capture
   or replay pipeline regressions. If you delete the native tests, a
   capture bug becomes invisible because the bug is baked into the case.

3. **Hermetic replay, deterministic capture.** Cases must not embed
   wall-clock timestamps, random session ids, process ids, or host-specific
   URLs. The capture driver has to canonicalize these at record time, or
   the replay will only work on the machine that captured it.

4. **One canonical scenario, not many narrow ones.** A single session
   lifecycle case exercises ~80% of the control-API surface and stays
   diff-reviewable. Prefer one 400-line case file over twenty 20-line ones.

5. **Explicit, human-approved refresh.** `make capture-refresh`
   regenerates the case against the reference build and surfaces a git
   diff. A human decides whether the diff is a real protocol change or a
   regression. **Never auto-commit a refreshed case from CI.**

6. **Clear failure taxonomy.** When a dogfood test fails, the error must
   distinguish between:
   - *code regression* ("CLI sent header `X` but case expected `Y`"),
   - *case staleness* ("response rule has no match because the server no
     longer accepts `Z`"),
   - *transport issue* ("runtime unreachable on `:8080`").

7. **Case files live in the repo next to the schema.** Under
   [`spec/examples/cases/`](../spec/examples/) the case is double-duty —
   test fixture plus copy-paste example of the wire format for users.

8. **Dogfood what your users do.** Users drive the CLI and the SDKs.
   That's exactly what we replay. Don't try to dogfood the proxy's OTEL
   pipeline — it's not the primary user surface and the case format wasn't
   designed for protobuf.

9. **Refresh PRs are never code PRs.** A PR that regenerates the case
   must not also modify runtime code. This keeps the "why did the case
   change?" question answerable by `git blame` alone.

## 6. Architecture of the dogfood run

```
                     +-------------------------+
                     | capture driver          |
                     | cmd/softprobe-dogfood-  |
                     | capture (Go)            |
                     +-----------+-------------+
                                 | drives CLI
                                 v
  +----------+     +-------------+-------------+     +--------------------+
  | softprobe |--->| egress proxy :8084        |---->| softprobe-runtime  |
  | CLI       |    | (captures the conversation)|    | (reference build)  |
  +----------+     +-------------+-------------+     +--------------------+
                                 |
                                 v
           spec/examples/cases/control-plane-v1.case.json  (committed)

-----------------------------  REPLAY TIME  -----------------------------

  +----------+     +-------------------------+     +--------------------+
  | softprobe |--->| any softprobe-runtime   |     | no external deps   |
  | CLI       |    | with case loaded        |     |                    |
  +----------+     +-------------------------+     +--------------------+
```

Capture time uses the real runtime. Replay time uses any runtime with the
case loaded — no network, no external services. Every SDK parity test
follows the same shape with its own SDK swapped in for the CLI.

## 7. Canonical dogfood scenario (v1)

The first case covers the full session lifecycle. It exercises every
mutating endpoint documented in [`design.md` §4.4](./design.md) and the
auth header from [`tasks.md` PD2](../tasks.md):

1. `softprobe doctor` — `GET /v1/meta` + `GET /health`
2. `softprobe session start --mode replay` — `POST /v1/sessions`
3. `softprobe session load-case --file fragment-happy-path.case.json` —
   `POST /v1/sessions/{id}/load-case`
4. `softprobe session rules apply --file rules.json` —
   `POST /v1/sessions/{id}/rules`
5. `softprobe session policy set --strict` —
   `POST /v1/sessions/{id}/policy`
6. `softprobe session stats` — `GET /v1/sessions/{id}/stats`
7. `softprobe session close` — `POST /v1/sessions/{id}/close`

All requests carry `Authorization: Bearer $SOFTPROBE_API_TOKEN` (per
[`tasks.md` PD2.1a–e](../tasks.md)).

Additional scenarios (capture-mode session close writing a case, fixture
upload, etc.) can be added as separate cases once v1 is stable.

## 8. Delivery

See [`tasks.md` Phase PD7](../tasks.md). Minimum viable slice is PD7.1b +
PD7.2a: the capture driver and one CLI replay test. Everything else
(SDK parity, refresh CI, docs-as-suite) is additive and lands
opportunistically.

## 9. Links

- Normative design: [`docs/design.md`](./design.md), especially
  [§3.2 Default happy path](./design.md),
  [§4.3 Two API surfaces](./design.md),
  [§4.4 Control API reference](./design.md),
  [§9 CLI design](./design.md),
  [§12 Acceptance criteria](./design.md).
- Repo layout and component responsibilities:
  [`docs/repo-layout.md`](./repo-layout.md).
- Control-API contract: [`spec/protocol/http-control-api.md`](../spec/protocol/http-control-api.md).
- CLI surface: [`docs-site/reference/cli.md`](../docs-site/reference/cli.md).
- Coding rules: [`AGENTS.md`](../AGENTS.md) (TDD, scope, CLI-first).
