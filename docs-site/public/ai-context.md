Last updated: 2026-04-25

This file is the **canonical short context** for AI agents working with Softprobe (replay tests, case files, CLI/SDK). Prefer it over memory for workflow rules. For details, follow the links at the end.

Stable URL (after deploy): `https://docs.softprobe.dev/ai-context.md`

## Product model

- Softprobe is **proxy-first for HTTP**. Tests and SDKs use the JSON **control API** through the `softprobe` CLI or language SDKs.
- The proxy and runtime use OTLP-shaped inject/extract internally.
- **Do not** make application tests call `/v1/sessions`, `/v1/inject`, or `/v1/traces` directly; use CLI/SDK.

Default replay flow:

1. `softprobe doctor`
2. Start or generate a replay session
3. Load a committed `*.case.json`
4. Register outbound mocks with `findInCase` / `find_in_case` plus `mockOutbound` / `mock_outbound`
5. Exercise the app **through the ingress proxy** with `x-softprobe-session-id`
6. Close the session in teardown

## CLI and SDK commands

- Verify install and wiring: `softprobe doctor`
- Capture session shell env: `eval "$(softprobe session start --mode capture --shell)"`
- Close session to case file: `softprobe session close --session "$SOFTPROBE_SESSION_ID" --out cases/example.case.json`
- Inspect case: `softprobe inspect case cases/example.case.json`
- Many cases: `softprobe suite run` (see suite YAML reference)
- Prefer `--json` or `--shell` when scripting for agents or CI

Use the **Softprobe SDK** for your language; do not hand-roll HTTP to the runtime control API from app tests.

## Header and session rules

- The **test client** must set `x-softprobe-session-id` on the **inbound** request to the app (through the ingress proxy).
- The app must propagate W3C **`traceparent`** and **`tracestate`** on outbound HTTP.
- **Do not** manually forward `x-softprobe-session-id` from the app to dependencies; the proxy correlates session via `tracestate`.
- Load cases from committed `cases/*.case.json`. Narrow `findInCase` predicates to avoid zero or multiple matches.
- Use `clearRules()` / `clear_rules()` when one session backs multiple scenarios with different mocks.
- Always **close** sessions in test teardown.

## Troubleshooting pointers

When mocks are not hit:

1. `softprobe doctor`
2. `softprobe inspect case <path>`
3. Confirm the test hits the **ingress proxy**, not the app port directly.
4. Confirm inbound request includes `x-softprobe-session-id`.
5. Confirm app outbound HTTP carries `traceparent` and `tracestate`.
6. Confirm `findInCase` matches the captured outbound span.

Official guides: [Troubleshooting](https://docs.softprobe.dev/guides/troubleshooting), [Session headers](https://docs.softprobe.dev/reference/session-headers).

## Task index (optional)

For page-by-page links, agents may use the published skill bundle `references/docs-map.md` or browse [docs.softprobe.dev](https://docs.softprobe.dev). This file stays the **normative** workflow summary; the site stays the **detailed** reference.

## Maintenance

When CLI flags, SDK APIs, header rules, or replay behavior change, update **this file** in the same change (or immediately after) so `https://docs.softprobe.dev/ai-context.md` stays accurate after the next docs deploy.
