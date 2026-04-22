# Hosted runtime (`runtime.softprobe.dev`)

The hosted Softprobe runtime is the fastest way to get started — no Docker, no containers, no infrastructure to manage. Sign up, get an API key, and run `softprobe doctor` in under five minutes.

The hosted runtime speaks the **same HTTP control API and OTLP trace API** as the OSS runtime. Every SDK, the CLI, and the Softprobe WASM proxy all work against it without code changes. The only difference is `SOFTPROBE_RUNTIME_URL` and the bearer token.

## Getting started (5 minutes)

### 1. Sign up and get an API key

Visit [app.softprobe.dev](https://app.softprobe.dev), sign in with Google or GitHub, and copy your API key from the dashboard. It looks like `sk_live_…`.

### 2. Set environment variables

```bash
export SOFTPROBE_RUNTIME_URL=https://runtime.softprobe.dev
export SOFTPROBE_API_KEY=sk_live_...
export SOFTPROBE_API_TOKEN=$SOFTPROBE_API_KEY   # SDKs and CLI read SOFTPROBE_API_TOKEN
```

Add these three lines to your shell profile (`.zshrc`, `.bashrc`) or your CI secret store. `SOFTPROBE_API_TOKEN` is the name the SDKs and CLI use internally; setting both ensures everything works.

### 3. Verify connectivity

```bash
softprobe doctor
# ✓ runtime reachable at https://runtime.softprobe.dev
# ✓ authenticated as <your-org>
# ✓ spec version matches CLI (http-control-api@v1)
```

If `doctor` shows a green check, you are ready to capture and replay.

### 4. Follow the quick start

The rest of the [Quick start](/quickstart) works unchanged — replace the `docker compose up` step with the environment variables above. The capture, load-case, and replay flows are identical.

## Authentication

Every request to the hosted runtime must carry:

```
Authorization: Bearer sk_live_...
```

All SDKs pick this up from `SOFTPROBE_API_TOKEN`. The CLI reads the same variable. The Softprobe WASM proxy reads `sp_api_key` from its plugin config (inject via Kubernetes secret; see [Kubernetes deployment](/deployment/kubernetes)).

## Session persistence

Sessions and their state (rules, policy, loaded case) are stored in Redis and survive runtime restarts. Captured extract payloads are stored in GCS; on `close`, a case file is assembled and stored under `gs://{your-bucket}/cases/{sessionId}.case.json`.

The OSS runtime stores everything in-memory and loses it on restart. Persistence is a hosted-only feature.

## Free tier

| Limit | Value |
|---|---|
| Sessions per day | 10 |
| Case retention | 7 days |
| Auth | API key (bearer token) |

No credit card required. Free tier is intended for individual evaluation and small projects.

## Differences from the OSS runtime

| Feature | OSS runtime | Hosted runtime |
|---|---|---|
| Session store | In-memory (lost on restart) | Redis (durable, TTL 24 h) |
| Case files | Local disk (`SOFTPROBE_CAPTURE_CASE_PATH`) | GCS (per-tenant bucket) |
| Auth | Optional static `SOFTPROBE_API_TOKEN` | Required API key, resolved via auth service |
| `GET /v1/cases/{id}` | Not available | Available — fetch a stored case by session ID |
| `GET /v1/sessions` | Returns in-memory sessions | Returns all sessions for the tenant from Redis |
| Multi-tenant | No | Yes — all sessions namespaced by tenant |

Everything else — the full control API, the OTLP trace API, inject/extract behavior, rules, policy, fixtures — is identical.

## Migrate from self-hosted to hosted

No code changes needed. Just swap the two environment variables:

```bash
# Before (self-hosted)
export SOFTPROBE_RUNTIME_URL=http://my-runtime:8080

# After (hosted)
export SOFTPROBE_RUNTIME_URL=https://runtime.softprobe.dev
export SOFTPROBE_API_KEY=sk_live_...
export SOFTPROBE_API_TOKEN=$SOFTPROBE_API_KEY
```

Then run `softprobe doctor` to confirm. If you have case files on disk, they continue to work — load them with `session load-case` as usual.

## Next

- [Quick start](/quickstart) — capture and replay your first session.
- [Installation](/installation) — install the CLI and SDKs.
- [CI integration](/guides/ci-integration) — running suites against the hosted runtime in CI.
- [Local deployment](/deployment/local) — run the runtime yourself if you need self-hosted.
