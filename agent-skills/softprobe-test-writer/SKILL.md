---
name: softprobe-test-writer
description: Help write or fix Softprobe-powered integration tests in Claude Code, Cursor, or another coding IDE. Use when the user asks to add replay tests, capture cases, mock HTTP dependencies with Softprobe, debug session headers, or turn a *.case.json file into Jest, pytest, or JUnit tests.
when_to_use: Trigger for requests mentioning Softprobe, softprobe, captured case files, replay tests, integration tests without live dependencies, x-softprobe-session-id, findInCase, mockOutbound, softprobe doctor, or softprobe generate jest-session.
---

# Softprobe Test Writer

Use this skill to help a developer write practical Softprobe integration tests. The goal is to get a real application test passing against captured HTTP behavior without inventing a second mocking system.

## Required Documentation Lookup

Before writing detailed code, choose the relevant official documentation from [references/docs-map.md](references/docs-map.md). Prefer the online docs at `https://docs.softprobe.dev` when network access is available. If this skill is being used inside the Softprobe repository, the same pages exist under `docs-site/`.

Do not rely on memory for SDK method names, CLI flags, generated Jest behavior, or header propagation details when the official docs are available.

## Core Product Model

Softprobe is proxy-first for HTTP. Tests and SDKs use the JSON control API through the `softprobe` CLI or language SDK. The proxy and runtime use OTLP-shaped inject/extract internally. Do not make application tests call `/v1/sessions`, `/v1/inject`, or `/v1/traces` directly.

Default replay flow:

1. Verify the runtime and proxy path with `softprobe doctor`.
2. Start or generate a replay session.
3. Load a committed `*.case.json` file.
4. Register outbound dependency responses with `findInCase` plus `mockOutbound`.
5. Exercise the app through the ingress proxy with `x-softprobe-session-id`.
6. Close the session in teardown.

Prefer the CLI and SDK surface over raw HTTP. Prefer copy-paste commands and small test helpers over abstract explanations.

## First Actions

When invoked in a repository:

1. Inspect the project language and test runner before editing files.
2. Look for existing case files with `*.case.json`, Softprobe SDK imports, `softprobe` scripts, and tests that already hit an ingress proxy.
3. Run or suggest `softprobe doctor` before debugging runtime, proxy, or header issues.
4. If no case file exists, help the user capture one before writing replay assertions.
5. If a case exists, inspect it with `softprobe inspect case <path>` when available, or read enough JSON to identify outbound HTTP spans.

Ask a concise clarification only when the app URL, case file, or intended user flow cannot be inferred.

## Decision Tree

Use generated Jest session helpers when the user wants the simplest Jest path:

```bash
softprobe generate jest-session \
  --case cases/checkout.case.json \
  --out test/generated/checkout.replay.session.ts
```

Then write a normal Jest test that imports `startReplaySession()`, sends requests to the app through the ingress proxy, sets `x-softprobe-session-id`, asserts the application response, and calls `close()` in `afterAll`.

Use hand-written SDK tests when the user needs to mutate captured responses, add custom match predicates, mix real and mocked upstreams, or support pytest/JUnit.

Use `softprobe suite run` for large collections of cases rather than generating hundreds of individual test functions.

## Patterns

For Jest, pytest, and JUnit examples, load the relevant official guide from [references/docs-map.md](references/docs-map.md). Keep generated code aligned with the docs rather than copying stale examples from this skill.

Rules that apply across languages:

- Use the Softprobe SDK for the language; do not hand-roll HTTP calls to the runtime control API.
- Load cases from committed `cases/*.case.json` files.
- Use `findInCase` / `find_in_case` after loading the case.
- Narrow case lookup predicates enough to avoid zero or multiple matches.
- Register dependency behavior with `mockOutbound` / `mock_outbound`.
- Use `clearRules()` / `clear_rules()` when one session backs multiple scenarios with different mocks.
- Always close sessions in test teardown.
- Exercise the app through the ingress proxy, not the direct app port.
- Set `x-softprobe-session-id` on the inbound request made by the test client.

## Capture Before Replay

If the user needs a case file, guide them through the CLI-first capture path:

```bash
softprobe doctor
eval "$(softprobe session start --mode capture --shell)"
curl -s -H "x-softprobe-session-id: $SOFTPROBE_SESSION_ID" http://127.0.0.1:8082/checkout
sleep 2
softprobe session close --session "$SOFTPROBE_SESSION_ID" --out cases/checkout.case.json
softprobe inspect case cases/checkout.case.json
```

Make sure traffic goes through the ingress proxy, not directly to the app. The reference local convention is app on `:8081` and ingress proxy on `:8082`; adapt to the repository.

## Header Propagation Rules

The test client must set `x-softprobe-session-id` on the inbound request to the app. The app must propagate W3C `traceparent` and `tracestate` on outbound HTTP. Do not manually forward `x-softprobe-session-id` from the app to dependencies; the proxy translates session correlation into `tracestate`.

When mocks are not hit, check:

```bash
softprobe doctor
softprobe inspect case cases/checkout.case.json
curl -v -H "x-softprobe-session-id: $SOFTPROBE_SESSION_ID" "$APP_URL/checkout"
```

Likely causes:

- Test called the app directly instead of the ingress proxy.
- Test forgot the `x-softprobe-session-id` header.
- App outbound HTTP does not carry `traceparent` and `tracestate`.
- `findInCase` predicate does not match the captured outbound span.
- The session was closed or the runtime restarted.

## Response Mutation

To test application behavior around changed dependency responses, mutate the captured response before registering the mock:

```ts
const hit = session.findInCase({ direction: 'outbound', hostSuffix: 'stripe.com' });
const body = JSON.parse(hit.response.body);
body.status = 'requires_payment_method';

await session.mockOutbound({
  direction: 'outbound',
  hostSuffix: 'stripe.com',
  response: { ...hit.response, body: JSON.stringify(body) },
});
```

Do not edit generated `*.replay.session.ts` files by hand. Add a wrapper or write an ad-hoc SDK test when mutation is required.

## Guardrails

- Do not introduce a custom mock DSL when SDK `mockOutbound` covers the need.
- Do not use raw runtime HTTP in tests unless the user explicitly asks for protocol-level testing.
- Do not make strict policy the first step for a beginner flow; add it after the happy path is green.
- Do not assume upstream dependencies are live in replay tests.
- Keep app assertions focused on user-visible behavior, not Softprobe internals.
- Prefer `--json` or `--shell` CLI output when generating commands for agents and CI.
