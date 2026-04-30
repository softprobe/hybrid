# Ship rules (and fixtures) with a case file

Some behavior belongs to the **case**, not the test: PII redaction on capture, default mocks for endpoints every scenario shares, auth tokens that every replay needs. Rather than copy-pasting those rules into every test file, ship them **inside the case file** as `case.rules[]` and `case.fixtures[]`.

This guide shows when to use embedded rules, how they interact with session rules from your test, and how to validate them.

## When to embed vs. apply from a test

| Scenario | Where rules belong |
|---|---|
| Redact authorization headers on capture | **Case-embedded** (`capture_only` with redaction config) |
| Default "401" on any unknown outbound host | **Case-embedded** — a sensible floor for every test using this case |
| Override the Stripe response for one specific test | **Session rule** via `mockOutbound` |
| Per-test-run auth token | **Session fixture** via SDK — specific to the test process |
| Cross-service defaults (e.g., "always return `{ ok: true }` for `/healthz`") | **Case-embedded** — all tests benefit |

Rule of thumb: **case-embedded rules set defaults, session rules override per-test**. The [precedence rules](/concepts/rules-and-policy#how-precedence-works) guarantee the session layer wins on ties.

## Case file shape

Both `case.rules[]` and `case.fixtures[]` are top-level arrays in the case JSON. `rules[]` entries follow the same [rule schema](/reference/rule-schema) as session rules; `fixtures[]` entries are free-form objects.

```json
{
  "version": "1.0.0",
  "caseId": "checkout-happy-path",
  "createdAt": "2026-04-20T10:00:00Z",
  "traces": [ /* ... */ ],
  "rules": [
    {
      "name": "redact-auth-headers",
      "priority": 10000,
      "when": { "direction": "outbound" },
      "then": { "action": "capture_only" }
    },
    {
      "name": "healthcheck-passthrough",
      "priority": 50,
      "when": {
        "direction": "outbound",
        "host": "localhost",
        "path": "/healthz"
      },
      "then": { "action": "passthrough" }
    }
  ],
  "fixtures": [
    { "name": "test_user_id", "value": "u_test_abc" },
    { "name": "test_tenant", "value": "tenant_01" }
  ]
}
```

Validate with the same CLI call:

```bash
softprobe validate case cases/checkout-happy-path.case.json
```

## Authoring embedded rules

### By hand

Edit the case JSON with your editor of choice. Case files are plain JSON — any diff tool understands them.

### By template from an existing case

If you have a golden case file and want to add a new embedded rule:

1. Open the case in your editor.
2. Add the rule to `rules[]` (before `traces[]` for readability).
3. Run `softprobe validate case <path>` — it validates each rule against the [rule schema](/reference/rule-schema).
4. Commit.

### By generating from CLI

You can seed a case's rules from a YAML rule pack:

```bash
softprobe case merge-rules \
  --case cases/checkout.case.json \
  --rules rules/redaction.yaml \
  --out cases/checkout.case.json
```

(In v0.5 this is experimental; `jq` edits are equivalent and well-supported.)

## How embedded rules reach the runtime

When the SDK calls `loadCaseFromFile` (or the CLI loads a case), the runtime:

1. Parses the case file.
2. Installs `case.rules[]` as the **case-embedded layer** of rules for that session — one layer beneath session rules.
3. Stores `case.fixtures[]` in the session fixture map.
4. Bumps `sessionRevision` so the proxy sees the new state on the next inject.

You do **not** need a separate `softprobe session rules apply` call — the case file is the source of truth.

## Precedence worked through

Say your case ships:

```yaml
# case.rules[] — layer B
- id: partner-default
  priority: 100
  when:  { direction: outbound, host: partner.example.com }
  then:  { action: mock, response: { status: 200, body: "{\"source\":\"case\"}" } }
```

Your test does:

```ts
await session.mockOutbound({
  name: 'partner-override',
  priority: 100,
  direction: 'outbound',
  host: 'partner.example.com',
  response: { status: 200, body: JSON.stringify({ source: 'test' }) },
});
```

On `/v1/inject` the runtime sees both rules, equal priority, but the **session layer is "later"** than the case-embedded layer. So the test's mock wins: the SUT sees `{"source":"test"}`. No priority bump required. See [How precedence works](/concepts/rules-and-policy#how-precedence-works) for the full algorithm.

## Using fixtures from a test

Fixtures are free-form key/value pairs surfaced through the SDK (and hook contexts in suites). Accessing them from a Jest test:

```ts
const fixtures = await session.getFixtures();  // { test_user_id: 'u_test_abc', ... }
const userId = fixtures.test_user_id;
```

Hooks receive the same object in their `context.fixtures` — useful for rewriting request bodies without hard-coding test values. See [Write a hook](/guides/write-a-hook).

## Editing an embedded rule in a single test

Because session rules override case-embedded rules on ties, you rarely need to **remove** an embedded rule. Instead, add a session rule with the same `when` and a higher priority (or equal priority, relying on the layer tie-break). The embedded rule still loads; the session rule wins.

If you *truly* need to delete an embedded rule for a specific test — for instance, an auth redaction that interferes with a test about auth itself — clone the case file, edit it, and point that test at the variant:

```bash
jq 'del(.rules[] | select(.id == "redact-auth-headers"))' \
  cases/checkout.case.json \
  > cases/checkout-auth-test.case.json
```

## Review checklist

Before merging a case file with embedded rules:

- [ ] Every rule has a stable `id` (makes diffs readable and enables hooks to target it).
- [ ] Priorities are spread out (e.g. `10`, `100`, `1000`, `10000`) so later edits have room to insert rules without reshuffling.
- [ ] `capture_only` rules have meaningful `id`s (audit tooling correlates them).
- [ ] `softprobe validate case` passes.
- [ ] A representative test runs green against the new embedded rules.

## See also

- [Rule schema](/reference/rule-schema) — the normative `when` / `then` fields.
- [Case file schema](/reference/case-schema) — the top-level case shape.
- [Rules and policy](/concepts/rules-and-policy) — precedence and composition.
- [Auth fixtures](/guides/auth-fixtures) — tokens that aren't HTTP-captured.
