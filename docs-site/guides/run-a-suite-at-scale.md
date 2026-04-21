# Run a suite at scale

::: tip Ships in this build
`softprobe suite run` is implemented end-to-end: scheduler, Node hook sidecar, JUnit XML, HTML report. The sibling Jest adapter (`runSuite()` from `@softprobe/softprobe-js/suite`) shares the same YAML schema and hook contract, so you can author hooks once and run them from CI (CLI) or your IDE (Jest). A full end-to-end harness lives at [`e2e/cli-suite-run/`](https://github.com/softprobe/softprobe/tree/main/e2e/cli-suite-run) — docker compose + `softprobe suite run` + MockResponseHook + BodyAssertHook wired against the same `app` / `upstream` services the rest of the e2e fleet uses.
:::

Hand-written tests are great for a handful of scenarios. But when you capture production sessions in bulk — hundreds, thousands, maybe tens of thousands of `*.case.json` files — writing one Jest test per case doesn't scale.

`softprobe suite run` reads one YAML file and executes every case deterministically, in parallel, emitting JUnit XML for your CI. It's the recommended path for **regression-sweeping captured production traffic**.

## When should I use a suite?

| You have… | Use |
|---|---|
| <10 scenarios, lots of custom assertions | Write Jest / pytest / JUnit / Go tests |
| 10–100 captures with shared behavior | Write a suite; keep a few hand-written tests for edge cases |
| Thousands of production captures, nightly regression | **Suite is the only reasonable option** |

Suite execution is roughly 10–20× faster per case than launching Jest/pytest worker processes, because there's no test-framework overhead per case — just HTTP.

## The one file you write: `suite.yaml`

```yaml
# suites/checkout.suite.yaml
name: checkout-nightly
cases: cases/checkout/*.case.json   # glob — can be a list

defaults:
  request:
    source: case.ingress    # replay the ingress request from the case
    transform: unmaskCard   # optional hook name (see "Hooks")

  mocks:
    - name: fragment
      match: { direction: outbound, host: fragment, pathPrefix: /shipping }
      source: case
    - name: stripe
      match: { direction: outbound, hostSuffix: stripe.com, pathPrefix: /v1/payment_intents }
      source: case

  assertions:
    status: 200
    headers:
      include:
        content-type: application/json
    body:
      mode: json-subset
      ignore:
        - "$.paymentId"
        - "$.createdAt"
      custom: assertTotalsMatchItems   # optional hook name
```

Three sections:

1. **`cases`** — which case files to run. Supports globs and lists.
2. **`defaults`** — how to build the request, what to mock, and how to assert. Per-case overrides live in `cases: [{ path, overrides: {...} }]` if you need them.
3. **`hooks` references** — optional named functions resolved by the executor (see [Write a hook](/guides/write-a-hook)).

Full reference: [Suite YAML](/reference/suite-yaml).

## Run it

```bash
softprobe suite run suites/checkout.suite.yaml \
  --parallel 32 \
  --junit out/checkout.xml \
  --report out/checkout.html
```

| Flag | Purpose |
|---|---|
| `--parallel N` | Run N cases concurrently. Defaults to `min(32, cpu * 4)`. |
| `--junit PATH` | Emit JUnit XML (consumed by most CI systems). |
| `--report PATH` | Emit a standalone HTML report with per-case diffs. |
| `--hooks PATH[,PATH]` | TypeScript hook files (see below). |
| `--filter GLOB` | Run only cases matching the substring/glob. |
| `--fail-fast` | Stop on first failure (default: run everything). |
| `--env-file FILE` | Load environment variables (e.g. `TEST_CARD`). |

## What the CLI does per case

```text
for each case in cases:
  1. start a replay session on the runtime
  2. POST /v1/sessions/$ID/load-case with the case file
  3. for each mock in defaults.mocks (and case-specific overrides):
       findInCase on the loaded case
       if transform hook declared → call it with the captured response
       POST /v1/sessions/$ID/rules (accumulated)
  4. build the request from case.ingress
     if transform hook declared → call it with the request
  5. send the request to APP_URL with x-softprobe-session-id: $ID
  6. compare the actual response with case.response
     using assertions (status, headers, body, ignores, redactions)
     if assert-body hook declared → call it with (actual, captured)
  7. close the session
  8. emit one <testcase> into the JUnit XML
```

Cases run in parallel at step granularity — the runtime handles hundreds of concurrent sessions.

## Hooks — when declarative isn't enough

Suites cover 80% of cases with pure YAML. The other 20% need *code*: PII masking, signature recomputation, custom invariants. For that, suites reference **named hooks** that the executor resolves at runtime.

In v1, the CLI executor supports **TypeScript/JavaScript** hooks via a Node sidecar:

```yaml
mocks:
  - name: stripe
    match: { hostSuffix: stripe.com }
    source: case
    hook: stripe.unmaskCard          # → hooks/stripe.ts export
```

```ts
// hooks/stripe.ts
import type { MockResponseHook, BodyAssertHook } from '@softprobe/softprobe-js/hooks';

export const unmaskCard: MockResponseHook = ({ capturedResponse, env }) => {
  const body = JSON.parse(capturedResponse.body);
  body.source.card.number = env.TEST_CARD ?? '4111111111111111';
  body.source.card.exp_year = 2030;
  return { ...capturedResponse, body: JSON.stringify(body) };
};

export const assertTotalsMatchItems: BodyAssertHook = ({ actual, captured }) => {
  const items = JSON.parse(captured.ingressBody).items;
  const expected = items.reduce((s: number, i: any) => s + i.price, 0)
                 + actual.shipping;
  if (Math.abs(actual.total - expected) > 0.01) {
    return [{ path: '$.total', expected, actual: actual.total }];
  }
  return [];
};
```

Pass them in at run time:

```bash
softprobe suite run suites/checkout.suite.yaml --hooks hooks/stripe.ts
```

The CLI spawns a Node sidecar once per suite run and streams JSON requests over stdin/stdout for each hook invocation. See [Write a hook](/guides/write-a-hook) for the full contract.

## Sharing hooks with framework tests

The same `hooks/*.ts` file can be imported from a Jest `runSuite` helper, so the custom logic is reused in both CI nightly (CLI) and dev-loop IDE debugging:

```ts
// __tests__/checkout.replay.test.ts
import { runSuite } from '@softprobe/softprobe-js/suite';
import * as hooks from '../hooks/stripe';

runSuite('suites/checkout.suite.yaml', { hooks });
// → registers one describe()/it() per case
```

The pytest, JUnit, and Go adapters also load the same YAML, but their hooks are in the framework's native language (Python/Java/Go). Suites stay the source of truth; hooks localize by executor. See the [design note](https://github.com/softprobe/softprobe/blob/main/docs/design.md#9-cli-design-revised) for the rationale.

## Overriding per case

When most cases share defaults but a few need tweaks:

```yaml
cases:
  - path: cases/checkout/happy.case.json
    name: happy-path
  - path: cases/checkout/card-declined.case.json
    name: card-declined
    overrides:
      mocks:
        - name: stripe
          source: inline
          response:
            status: 402
            body: '{"error":{"type":"card_error","code":"card_declined"}}'
      assertions:
        status: 402
```

Overrides shallow-merge into `defaults`: `request`, `assertions`, and `policy` replace wholesale; `mocks` merge by `name:` so override entries with matching names swap the default in place. Unmatched override mocks append. Full rules: [Suite YAML → Per-case overrides](/reference/suite-yaml#per-case-overrides).

**One capture, many scenarios.** A powerful pattern when you only have one real recording: point two `cases:` entries at the *same* `path:` and have one use the hook-driven default mock while the other uses `overrides.mocks` with `source: inline` to simulate a failure mode. You now have end-to-end coverage of two distinct SUT outcomes from a single capture. The [`e2e/cli-suite-run/`](https://github.com/softprobe/softprobe/tree/main/e2e/cli-suite-run) harness does exactly this — a `happy-path` case lets a `rewriteDep` `MockResponseHook` mutate the captured `/fragment` body, and a `fragment-down` case overrides the same mock with an inline 503 to test the SUT's degraded-dependency path. Same `/hello` endpoint, same YAML, two verified outcomes.

### The `name:` field matters at scale

With 2,000 cases in one suite, log lines like `FAIL cases/checkout/happy.case.json` aren't enough — especially when multiple cases reference the same capture. Add `name:` to each entry:

- Human output shows `path [name]`
- JSON/JUnit carries it as `displayName`/`caseId`
- `--filter SUBSTR` matches against it, so `--filter card-declined` does the obvious thing

## Using environment variables

Any `${VAR}` in the YAML is expanded at runtime:

```yaml
request:
  headers:
    authorization: "Bearer ${TEST_TOKEN}"

assertions:
  body:
    ignore:
      - "$.expiresAt"
      - "$.clientSecret"
```

Pass them via `--env-file` or the shell:

```bash
TEST_TOKEN=eyJ... softprobe suite run suites/checkout.suite.yaml
```

## Output formats

### JUnit XML (`--junit out/report.xml`)

```xml
<testsuites name="checkout-nightly" tests="120" failures="2" time="38.41">
  <testsuite name="checkout-nightly" tests="120" failures="2">
    <testcase classname="checkout" name="cases/checkout/happy.case.json" time="0.31"/>
    <testcase classname="checkout" name="cases/checkout/declined.case.json" time="0.29">
      <failure message="body mismatch at $.total">...</failure>
    </testcase>
    ...
  </testsuite>
</testsuites>
```

Consumable by GitHub Actions, CircleCI, Jenkins, GitLab CI, etc.

### HTML report (`--report out/report.html`)

Self-contained page with per-case pass/fail, durations, diffs of expected vs. actual body. No server needed — open it in a browser or upload as a CI artifact.

### JSON (`--json`)

```bash
softprobe suite run suites/checkout.suite.yaml --json > result.json
```

Stream of JSON objects, one per case, for further processing.

## Performance tips

**Crank up `--parallel` for read-heavy cases.** Replay is mostly I/O. `--parallel 64` on a laptop typically runs 2× as fast as `--parallel 16` until you saturate the runtime (one CPU is usually enough).

**Put the case files on local SSD.** If your cases live on a network mount, suite startup can stall at glob expansion. Copy to `/tmp/cases/` in CI.

**Scope your captures.** A 50-MB case is probably three scenarios pretending to be one. Split them.

**Warm up the runtime.** The first session's `load-case` pays for JSON parsing of the case. For very large cases, suite runners pool a case-content cache across parallel workers automatically.

## Next

- [Write a hook](/guides/write-a-hook) — TypeScript hooks for unmasking PII, custom asserts, dynamic request bodies.
- [Suite YAML reference](/reference/suite-yaml) — every field.
- [CI integration](/guides/ci-integration) — running suites in GitHub Actions / GitLab CI.
