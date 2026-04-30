# CLI `softprobe suite run` — end-to-end

This harness proves `softprobe suite run` is fit for scale: it drives a
real `suite.yaml` against the same docker-compose stack every other
harness uses (`softprobe-runtime`, Envoy + Softprobe WASM, `app` SUT,
`upstream`), exercising both the hook-driven and YAML-only paths in a
single run.

It is the CI-side counterpart of `e2e/jest-hooks/`: the **same** suite
format, the **same** hook files can be authored once and run from
either Jest (developer loop) or the Go CLI (CI nightly). See
`docs-site/guides/run-a-suite-at-scale.md` for the user-facing story.

## What it demonstrates

The suite defines **two cases** that reuse a single capture
(`spec/examples/cases/fragment-happy-path.case.json`) and drive two
different SUT outcomes from it:

| Case | Mock source | Assert hook | Proves |
|---|---|---|---|
| `happy-path` | `source: case` + `rewriteDep` MockResponseHook | `helloShape` | Node sidecar → hook → runtime → proxy → SUT → back to assert hook, end-to-end. |
| `fragment-down` | `overrides.mocks` with `source: inline` (503) | `helloUnavailable` | YAML-only path: per-case overrides, inline mocks, no hook roundtrip. |

Both cases route `/hello` through the ingress proxy (`:8082`) so the
WASM filter translates `x-softprobe-session-id` into W3C
`tracestate` on the egress hop — the exact path the
[proxy session-id fix](../../softprobe-proxy/src/otel.rs) lives on.

## What the CLI does per case

1. `softprobe suite run` spawns the Node sidecar **once per run**
   (embedded at `softprobe-cli/cmd/softprobe/sidecar/suite-sidecar.mjs`).
2. For each case:
   - Open a replay session on the runtime.
   - `POST /v1/sessions/{id}/load-case` with the case bytes.
   - Per-case `overrides:` merged into top-level `defaults:` (mocks
     merge by `name`, others replace wholesale).
   - For each resolved mock: `findInCase` → optional `MockResponseHook`
     via the sidecar → `POST /v1/sessions/{id}/rules`.
   - Build the request (`GET /hello` through the ingress proxy); CLI
     adds `x-softprobe-session-id` automatically.
   - Compare the response against `assertions.status` / `.headers` /
     `.body`, then invoke any `body.custom` / `headers.custom`
     `BodyAssertHook` / `HeadersAssertHook` via the sidecar.
   - Close the session; emit a `<testcase>` into the JUnit XML.

Exit code `0` on all-pass, `20` on any failure.

## Why the hook matters end-to-end

The `happy-path` case is the regression guard for the full sidecar +
proxy chain. `rewriteDep` writes `env.FRAGMENT_DEP_VALUE` (default
`"mutated-by-cli"`) into the registered mock; `helloShape` asserts the
SUT's `dep` equals that same value. If the hook doesn't run, or the
proxy fabricates a synthetic session id and the runtime never matches
the rule (see
[`softprobe-proxy/src/otel.rs::session_id_tests`](../../softprobe-proxy/src/otel.rs)),
or the WASM hops lose the session id, the live upstream's
`{"dep":"ok"}` bleeds through and `helloShape` flags `$.dep`. A single
failed case means the full chain broke.

The `fragment-down` case is the regression guard for the pure-YAML
path: no hook, just `overrides.mocks` with `source: inline`. If the
override merge breaks or inline mocks stop registering, this case
flips to FAIL with `$.dep = "ok"` instead of `"unavailable"`.

## Prerequisites

Bring up the shared e2e stack and build the CLI:

```bash
docker compose -f e2e/docker-compose.yaml up -d --wait
( cd softprobe-cli && go build -o ../softprobe ./cmd/softprobe )
```

Node 18+ must be on `PATH` for the sidecar. Node 22+ is recommended so
TypeScript hook files load directly via `--experimental-strip-types`;
on older Node, compile hooks to `.js` or `.mjs` first.

## Run it

```bash
cd e2e/cli-suite-run
mkdir -p out
FRAGMENT_DEP_VALUE=mutated-by-cli \
../../softprobe suite run \
  --runtime-url http://127.0.0.1:8080 \
  --app-url    http://127.0.0.1:8082 \
  --hooks      hooks/rewrite-dep.ts \
  --hooks      hooks/assert-hello.ts \
  --junit      out/junit.xml \
  --report     out/report.html \
  --parallel   2 \
  suites/fragment.suite.yaml
```

Expected:

```text
suite: fragment-hooks-cli
  OK ../../spec/examples/cases/fragment-happy-path.case.json [happy-path] (~50ms)
  OK ../../spec/examples/cases/fragment-happy-path.case.json [fragment-down] (~50ms)
result: passed=2 failed=0 total=2
```

The bracketed `[name]` comes from each `cases[i].name` — essential
when two entries share a `path:`.

### Filter one case at a time

```bash
../../softprobe suite run --filter fragment-down \
  --runtime-url http://127.0.0.1:8080 --app-url http://127.0.0.1:8082 \
  --hooks hooks/assert-hello.ts \
  suites/fragment.suite.yaml
```

`--filter SUBSTR` matches against either the resolved case path or the
`name:` field, so `fragment-down` targets the override-only case.
Notice the command above omits `hooks/rewrite-dep.ts` — the
`fragment-down` case doesn't need it, and the CLI only loads the hooks
you pass.

### Negative proof (regression probe)

Remove `--hooks hooks/rewrite-dep.ts` and run the full suite:

```text
FAIL … [happy-path] … resolve mocks: mocks[0] "fragment": hook "rewriteDep" not found in any --hooks file
OK   … [fragment-down] (~50ms)
exit=20
```

That's the CLI telling you the `happy-path` case wants a sidecar hook
that isn't loaded; `fragment-down` keeps passing because its
`overrides:` uses `source: inline` — no sidecar needed. This is the
cleanest 5-second check that both paths are wired independently.

## Layout

```
cli-suite-run/
├── README.md
├── hooks/
│   ├── rewrite-dep.ts    # MockResponseHook — mutates captured body
│   └── assert-hello.ts   # BodyAssertHook × 2 — helloShape, helloUnavailable
├── suites/
│   └── fragment.suite.yaml  # 2 cases off one capture file
└── out/                  # gitignored: junit.xml, report.html
```

## Sharing hooks with the Jest harness

`e2e/jest-hooks/` and `e2e/cli-suite-run/` intentionally author their
hooks separately today, but the shapes are identical — the CLI's Node
sidecar and the Jest `runSuite()` adapter both invoke hooks with the
same `{ capturedResponse, capturedSpan, mockName, ctx, env }` /
`{ actual, captured, ctx, env }` payloads. You can point the CLI at
the Jest hook files, or vice-versa, as long as the files stay in the
pure-types-only subset of TypeScript (no `enum`, no `namespace` — both
drivers rely on Node's `--experimental-strip-types`).
