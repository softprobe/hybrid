# Contribute to the Dogfood Case

The checked-in file `spec/examples/cases/control-plane-v1.case.json` is the **reference case** for the dogfood parity tests in every SDK. It records exactly what HTTP traffic the CLI sends to the control-plane runtime during a canonical flow. Any runtime or SDK change that alters that traffic surface must be accompanied by a refreshed case — this guide explains how.

## When does the dogfood test fail?

The Go dogfood replay test (`softprobe-runtime/cmd/softprobe/dogfood_replay_test.go`) fails when the recorded case no longer matches the runtime's current behavior. There are two root causes:

| `errorType` | Meaning |
|---|---|
| `code_regression` | The runtime rejected a request that the case expected to match — a real regression. |
| `case_staleness` | The HTTP surface changed legitimately (new verb, new path, renamed field). Refresh the case. |

## Prerequisites

You need a working local compose stack to drive the capture:

```bash
# Build the WASM proxy module (one-time)
cd softprobe-proxy && make build

# Confirm the stack starts
docker compose -f e2e/docker-compose.yaml up --build --wait
```

## Running a refresh

```bash
make capture-refresh
```

This command:

1. **Guards against mixing code + case changes** — aborts if `softprobe-runtime/`, `softprobe-go/`, `softprobe-js/`, `softprobe-python/`, or `softprobe-java/` have uncommitted modifications. Commit or stash those first.
2. Starts the e2e compose stack, runs the capture driver (`spec/dogfood/capture.sh`), and canonicalizes session/trace IDs to stable placeholders.
3. Overwrites `spec/examples/cases/control-plane-v1.case.json` with the new recording.
4. Prints the diff.

## Inspect the diff

A healthy refresh diff changes only span attribute values that reflect the new surface (e.g., a new query parameter, an added response header). It should **never** add or remove entire traces unless the CLI flow itself changed.

Red flags:

- Completely new traces — the canonical flow changed. Update `spec/dogfood/capture.sh` to match and re-run.
- Removed spans — something the CLI used to call is gone. Verify it is intentional.
- Changed session IDs or trace IDs that were not canonicalized — the capture driver needs updating.

## Landing the PR

Open a PR with **only** the updated `spec/examples/cases/control-plane-v1.case.json`. The PR description should explain why the surface changed. CI will run the dogfood replay tests against the new case to confirm the refresh resolves the staleness failure.

The nightly refresh job (`.github/workflows/dogfood-refresh.yml`) automates this for unattended runs. When you see an automated PR labelled `dogfood`, review the diff using the criteria above before merging.

## Fixing a code regression instead

If `errorType` is `code_regression` — not `case_staleness` — do **not** refresh the case. Instead:

1. Run `softprobe replay run --session <id> --json` to see which rules missed.
2. Bisect against recent runtime commits.
3. Fix the regression and re-run the dogfood tests.
