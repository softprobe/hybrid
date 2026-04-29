# Hosted runtime (`runtime.softprobe.dev`)

The hosted Softprobe runtime is the fastest way to get started — no Docker, no containers, no infrastructure to manage. Sign up, get an API key, and run `softprobe doctor` in under five minutes.

The hosted runtime speaks the **HTTP control API and OTLP trace API** used by
every SDK, the CLI, and the Softprobe WASM proxy. In the official setup,
`SOFTPROBE_RUNTIME_URL` stays at the hosted default and `SOFTPROBE_API_TOKEN`
provides authentication.

## Getting started (5 minutes)

### 1. Sign up and get an API key

Visit [https://dashboard.softprobe.ai](https://dashboard.softprobe.ai), sign in with Google or GitHub, and copy your API key from the dashboard. It looks like `…`.

### 2. Set environment variables

```bash
export SOFTPROBE_API_TOKEN=...
```

Add this to your shell profile (`.zshrc`, `.bashrc`) or your CI secret store. All SDKs and the CLI read `SOFTPROBE_API_TOKEN` directly.

### 3. Verify connectivity

```bash
softprobe doctor
# ✓ runtime reachable at https://runtime.softprobe.dev
# ✓ authenticated as <your-org>
# ✓ spec version matches CLI (http-control-api@v1)
```

If `doctor` shows a green check, you are ready to capture and replay.

### 4. Follow the quick start

The rest of the [Quick start](/quickstart) uses the hosted runtime. The local
Docker Compose stack in that guide runs only the sample app, dependency, and
proxy.

## Authentication

Every request to the hosted runtime must carry:

```
Authorization: Bearer ...
```

All SDKs pick this up from `SOFTPROBE_API_TOKEN`. The CLI reads the same variable. The Softprobe WASM proxy reads `sp_api_key` from its plugin config (inject via Kubernetes secret; see [Kubernetes deployment](/deployment/kubernetes)).

## Session persistence

Sessions and their state (rules, policy, loaded case) are stored in Redis and survive runtime restarts. Captured traces are ingested directly into datalake (DuckLake). Capture exports are fetched by `captureId` from `GET /v1/captures/{captureId}`.

## Free tier

| Limit | Value |
|---|---|
| Sessions per day | 10 |
| Case retention | 7 days |
| Auth | API key (bearer token) |

No credit card required. Free tier is intended for individual evaluation and small projects.

## Runtime behavior

The hosted runtime is durable and tenant-scoped:

| Feature | Hosted behavior |
|---|---|
| Session store | Redis-backed session state with a 24 h TTL |
| Capture storage | Datalake (DuckLake) per-tenant queryable spans |
| Auth | Required API key, resolved by the Softprobe auth service |
| `GET /v1/captures/{captureId}` | Fetches capture JSON scoped to the authenticated tenant |
| `GET /v1/sessions` | Lists open sessions for the authenticated tenant |

The control API, OTLP trace API, inject/extract behavior, rules, policy, and
fixtures use the same contracts documented in [Reference](/reference/cli).

## Next

- [Quick start](/quickstart) — capture and replay your first session.
- [Installation](/installation) — install the CLI and SDKs.
- [CI integration](/guides/ci-integration) — running suites against the hosted runtime in CI.
- [Local proxy stack](/deployment/local) — run your app and proxy locally against the hosted runtime.
