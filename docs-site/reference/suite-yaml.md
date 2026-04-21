# Suite YAML reference

This is the complete schema for `*.suite.yaml` files consumed by `softprobe suite run` and the SDK `runSuite` adapters.

For a walkthrough with examples, see [Run a suite at scale](/guides/run-a-suite-at-scale).

## Top-level fields

```yaml
name: checkout-nightly          # (required) human-readable suite name
version: 1                      # (optional) schema version, defaults to 1
cases: cases/**/*.case.json     # (required) glob or list

defaults:                       # (optional) applied to every case
  request:    { ... }
  mocks:      [ ... ]
  assertions: { ... }

cases:                          # alternative to the glob form
  - path: cases/happy.case.json
  - path: cases/declined.case.json
    overrides: { ... }          # per-case overrides (shallow merge)

env:                            # (optional) default env vars
  TEST_TOKEN: default-token
```

### `cases`

Either a glob string/list or an array of objects with `path` + optional `overrides`.

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
  - path: cases/checkout/declined.case.json
    overrides:
      mocks:
        - name: stripe
          response: { status: 402, body: '...' }
      assertions:
        status: 402
```

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

Overrides shallow-merge into `defaults`: top-level keys replace wholesale, except `mocks` which merges by `name`.

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
