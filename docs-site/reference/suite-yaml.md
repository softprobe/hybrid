# Suite YAML reference

This is the complete schema for `*.suite.yaml` files consumed by `softprobe suite run` and the SDK `runSuite` adapters.

For a walkthrough with examples, see [Run a suite at scale](/guides/run-a-suite-at-scale).

## Top-level fields

```yaml
name: checkout-nightly          # (required) human-readable suite name
version: 1                      # (optional) schema version, defaults to 1

cases:                          # (required) glob, list, or explicit entries
  - path: cases/happy.case.json
    name: happy-path            # (optional) stable display name
  - path: cases/declined.case.json
    name: declined
    overrides: { ... }          # per-case overrides (shallow merge)

defaults:                       # (optional) applied to every case
  request:    { ... }
  mocks:      [ ... ]
  assertions: { ... }

# Shortcut: when every case shares the same request/mocks/assertions you
# can omit `defaults:` and declare them at the top level. The parser
# folds these into `defaults` automatically, so this…
request:    { ... }
mocks:      [ ... ]
assertions: { ... }
# …is equivalent to putting them under `defaults:`. Useful for small
# suites; switch to `defaults:` once you start using `overrides:`.

env:                            # (optional) default env vars
  TEST_TOKEN: default-token     # shell env > --env-file > suite `env:`
```

::: tip Worked example
A minimal 2-case suite — one hook-driven, one YAML-only — lives in [`e2e/cli-suite-run/`](https://github.com/softprobe/softprobe/tree/main/e2e/cli-suite-run). It reuses a single capture file and drives two different SUT outcomes from it through `overrides:` + `source: inline`. The same YAML runs under both `softprobe suite run` and the Jest `runSuite()` adapter.
:::

### `cases`

Either a glob string/list or an array of objects with `path` + optional `name` + optional `overrides`.

```yaml
# Form A: glob
cases: cases/checkout/*.case.json

# Form B: list of globs
cases:
  - cases/checkout/*.case.json
  - cases/signup/*.case.json

# Form C: explicit with overrides
cases:
  - path: cases/checkout/happy.case.json
    name: happy-path
  - path: cases/checkout/happy.case.json        # same capture!
    name: declined
    overrides:
      mocks:
        - name: stripe
          source: inline
          response: { status: 402, body: '...' }
      assertions:
        status: 402
```

The `name:` field is the **displayName**: it shows up in the human output as `path [name]`, in JSON/JUnit as `displayName`/`caseId`, and — crucially — is matchable by `softprobe suite run --filter`. Two cases can share the same `path:` (one capture → many tests), which is why `name:` is the only reliable way to disambiguate them.

`cases[i].skip: true` and `cases[i].only: true` let you scope a run without editing `filter`. `only:` on one entry auto-skips the rest.

## `defaults.request`

Describes how to build the HTTP request sent to the SUT.

```yaml
defaults:
  request:
    source: case.ingress            # reuse the captured ingress request
    transform: checkout.unmaskCard  # optional hook name
```

| Field | Type | Default | Purpose |
|---|---|---|---|
| `source` | `case.ingress` \| object | `case.ingress` | Where to pull the request from. `case.ingress` means "use the recorded inbound request verbatim"; an inline object (`method`, `path`, `headers`, `body`) overrides completely. |
| `transform` | hook name | — | Invoked before the request is sent; can modify any field |
| `url` | URL | `$APP_URL` | Override the target URL per case |

### Inline request

```yaml
defaults:
  request:
    method: POST
    path: /checkout
    headers:
      authorization: "Bearer ${TEST_TOKEN}"
    body: '{"amount": 1000}'
```

## `defaults.mocks`

Array of rules the runtime should apply to outbound calls from the SUT.

```yaml
defaults:
  mocks:
    - name: stripe                            # stable id; used in reports/overrides
      match:
        direction: outbound
        hostSuffix: stripe.com
        pathPrefix: /v1/payment_intents
        method: POST
      source: case                            # or inline `response: { ... }`
      hook: stripe.unmaskCard                 # optional MockResponseHook
      priority: 100                           # optional; higher wins
      consume: many                           # `once` | `many`
      latencyMs: 0                            # simulated latency

    - name: fragment
      match: { direction: outbound, host: fragment, pathPrefix: /shipping }
      source: case
```

### `match`

Every key is an AND'd predicate. See [Rules and policy](/concepts/rules-and-policy#predicates-in-when) for the complete list.

### `source`

- `source: case` — look up the response in the loaded case via `findInCase`. If zero or multiple match, the case **fails** (ambiguity at authoring time).
- `source: (inline response)` — hand-write the response:

  ```yaml
  - name: custom
    match: { hostSuffix: example.com }
    response:
      status: 200
      headers: { content-type: application/json }
      body: '{"ok":true}'
  ```

### `hook`

A hook name referencing an exported `MockResponseHook` function. Invoked after `source: case` resolves; the returned fields overlay the captured response.

## `defaults.assertions`

How the actual SUT response is compared to the captured one.

```yaml
defaults:
  assertions:
    status: 200
    headers:
      include:
        content-type: application/json
      ignore: [date, x-request-id]
    body:
      mode: json-subset               # or `exact` | `string` | `ignore`
      ignore:
        - "$.paymentId"
        - "$.createdAt"
      redactions:
        - path: "$.ssn"
          replacement: "***-**-****"
      custom: checkout.assertTotalsMatchItems
    custom-headers: checkout.requireRetryAfter
```

### `status`

- Integer → exact match
- Array → match any: `[200, 201]`
- Object → wildcard: `{ min: 200, max: 299 }`

### `headers.include`

Every key must be present and regex-match the value. Absent keys fail; extras are OK.

```yaml
headers:
  include:
    content-type: ^application/json
    etag: .+
```

### `headers.ignore`

Keys to strip from comparison altogether.

### `body.mode`

| Mode | Behavior |
|---|---|
| `json-subset` (default) | Actual is a superset of captured; actual may add fields. |
| `exact` | Every field, every value, identical. |
| `string` | Byte-level comparison. Use for non-JSON bodies. |
| `ignore` | Skip body comparison. |

### `body.ignore`

JSONPath list of fields to ignore in `json-subset` / `exact` mode.

### `body.redactions`

Scrub fields before comparison (useful when the actual response leaks PII you don't want in the diff output).

### `body.custom`

A hook name referencing a `BodyAssertHook`. Invoked after the built-in comparison; returned `Issue[]` adds to the failures.

## Per-case overrides

```yaml
cases:
  - path: cases/checkout/happy.case.json
  - path: cases/checkout/declined.case.json
    overrides:
      mocks:
        - name: stripe
          response:
            status: 402
            body: '{"error":{"type":"card_error","code":"card_declined"}}'
      assertions:
        status: 402
```

Overrides shallow-merge into `defaults`:

- `request`, `assertions`, `policy` — replace wholesale when present
- `mocks` — merge by `name`: an override whose `name:` matches an existing mock replaces that one in place; unmatched overrides append

This is what lets one capture file drive two distinct test shapes: the top-level `mocks:` can register a hook-driven mock, and a per-case `overrides.mocks:` entry with the same `name:` can swap it for a `source: inline` response without touching the shared hooks. See [`e2e/cli-suite-run/suites/fragment.suite.yaml`](https://github.com/softprobe/softprobe/tree/main/e2e/cli-suite-run/suites/fragment.suite.yaml) for a runnable example.

## Environment variable expansion

Any `${VAR}` in the YAML is expanded at load time:

```yaml
defaults:
  request:
    headers:
      authorization: "Bearer ${TEST_TOKEN}"
```

Default values use `${VAR:-default}`:

```yaml
defaults:
  mocks:
    - name: stripe
      match: { hostSuffix: ${STRIPE_HOST:-stripe.com} }
```

## Policy

```yaml
defaults:
  policy:
    externalHttp: strict
    externalAllowlist: [localhost, internal.svc.cluster.local]
    defaultOnMiss: error
```

Applied via `softprobe session policy set` before each case runs.

## Complete example

```yaml
name: checkout-nightly
version: 1
cases: cases/checkout/**/*.case.json

env:
  TEST_CARD: "4242424242424242"

defaults:
  request:
    source: case.ingress
    transform: checkout.unmaskCard

  policy:
    externalHttp: strict

  mocks:
    - name: stripe
      match:
        direction: outbound
        hostSuffix: stripe.com
        pathPrefix: /v1/payment_intents
      source: case
      hook: stripe.refreshTimestamps

    - name: fragment
      match: { direction: outbound, host: fragment, pathPrefix: /shipping }
      source: case

    - name: sendgrid
      match: { direction: outbound, hostSuffix: sendgrid.net }
      response:
        status: 202
        body: '{"message":"queued","id":"test_msg"}'

  assertions:
    status: 200
    headers:
      include:
        content-type: application/json
    body:
      mode: json-subset
      ignore:
        - "$.paymentId"
        - "$.timestamp"
      custom: checkout.assertTotalsMatchItems

cases:
  - path: cases/checkout/happy.case.json
  - path: cases/checkout/declined.case.json
    overrides:
      mocks:
        - name: stripe
          response:
            status: 402
            body: '{"error":{"type":"card_error","code":"card_declined"}}'
      assertions:
        status: 402
```

## Validation

```bash
softprobe suite validate suites/checkout.suite.yaml
```

Checks:

- YAML is parseable.
- Every referenced hook exists in the `--hooks` files.
- Every `cases:` glob or path resolves.
- `match` predicates are valid keys.
- `body.ignore` entries are valid JSONPath.

## Schema

The JSON schema for suite YAML is published at `spec/schemas/suite.schema.json` and versioned with the spec. `softprobe suite validate` uses it under the hood.

## See also

- [Run a suite at scale](/guides/run-a-suite-at-scale) — tutorial
- [Write a hook](/guides/write-a-hook) — hook contracts referenced here
- [Rules and policy](/concepts/rules-and-policy) — predicates in `match`
