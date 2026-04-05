---
name: softprobe
description: >
  Validates backend service behavior using Softprobe capture/replay. Use this skill whenever
  the user wants to record HTTP, Postgres, or Redis traffic, replay cassettes against a live
  service, compare API responses between versions, run regression tests, or integrate Softprobe
  into a Node.js project. Trigger this skill when the user mentions softprobe, capture/replay
  testing, cassettes, traffic recording, replay diffs, behavioral regression, or wants to verify
  that a refactored or updated service still behaves the same way as before — even if they don't
  say "softprobe" explicitly. If the user is trying to confirm "does my new code still return the
  same thing?", this skill is the right one to use.
---

# Softprobe Capture/Replay Skill

Softprobe provides deterministic record/replay for backend traffic: capture inbound and outbound
dependency behavior to NDJSON cassettes, then replay against the current service to detect
behavioral regressions.

## Required Read Order

Before executing any Softprobe task in any repository, read these reference files in order:

1. `references/softprobe-spec.md` — product baseline, modes, cassette layout, matching
2. `references/architecture-contract.md` — OTel ordering, dependency rules, replay flow contract
3. `references/integration-runbook.md` — bootstrap pattern, config, preflight checks, CLI commands
4. `references/workflow-contract.md` — required sequence, verification standard, failure policy
5. `references/compatibility-matrix.md` — supported runtimes, protocols, required platform pieces
6. `references/do-not-infer.md` — what must never be assumed; when to stop and ask

Do not guess behavior outside these docs. If required details are missing in the target repo,
stop and ask.

## Prerequisites

- Global CLI installed: `npm install -g @softprobe/softprobe-js@latest`
- Target bootstrap loads `@softprobe/softprobe-js/init`, initializes OTel NodeSDK, and calls `sdk.start()`
- Config file `.softprobe/config.yml` exists with `mode: PASSTHROUGH` and `cassetteDirectory` set

## Critical Integration Rules

- Add `"start": "node -r ./instrumentation.js server.js"` to the `scripts` section
  of `package.json`, then launch with `npm start`. The `-r` preload is what activates
  the bootstrap before the server module loads.
- Never deep-import internal package files such as `@softprobe/softprobe-js/dist/...`.
- Do not add manual middleware imports unless there is an explicit public export and user request.
- If init ordering or config is unclear, stop and ask instead of guessing.

## Inputs

- `TARGET_URL`: service URL (e.g. `http://127.0.0.1:3000`)
- `ROUTE`: path/query to capture (e.g. `/price?sku=coffee-beans`)
- `TRACE_ID`: deterministic 32-char hex trace id
- `CASSETTE`: cassette path (e.g. `./cassettes/<TRACE_ID>.ndjson`)

## Workflow

1. Run integration preflight from `references/integration-runbook.md`.
2. Capture baseline request using `scripts/capture.sh`.
3. Replay and diff against target using `scripts/diff.sh`.
4. Report PASS/FAIL and mismatched fields.

## Scripts

Use the bundled scripts in `scripts/`:

- `scripts/capture.sh` — capture a request to cassette
- `scripts/diff.sh` — replay cassette and compare against live service
- `scripts/demo-pricing.sh` — end-to-end pricing regression demo

## Output Format

Always report results in this structure:

- `Result`: PASS or FAIL
- `Trace`: trace id used
- `Cassette`: resolved cassette file path
- `Mismatch`: list path + recorded/live values (only on FAIL)
