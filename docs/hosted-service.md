# Softprobe Hosted Service: Design Document

**Status:** Draft for decision  
**Audience:** Founding engineers making architecture decisions  
**Principles:** Simple over complete. No backwards compat. 3rd-party over custom. 5-minute self-onboarding.

---

## 1. What we're building

The OSS `softprobe-runtime` is a single-process server with an in-memory store that vanishes on restart. The hosted service (`app.softprobe.dev`) makes it durable and multi-tenant so users can sign up, get an API key, and run `softprobe` against `https://runtime.softprobe.dev` rather than a local process.

The existing `otel-server` (Java/Spring, deployed at `o.softprobe.ai`) already handles durable span storage in BigQuery/GCS with a working multi-tenant API key model. We build on top of it rather than replacing it.

**What changes vs. OSS:**
- Sessions and their attached state (rules, policy, fixtures, loaded case) are persisted in Redis and survive restarts.
- Captured case files are stored in GCS rather than written to the local filesystem.
- Every API call requires an API key that resolves to a tenant.
- Users onboard via a web UI (sign up → copy API key → run one command).

**What does not change:**
- The control API wire contract (`spec/protocol/http-control-api.md`) is identical — the hosted runtime is the same binary with a persistence backend wired in.
- The proxy OTLP API (`spec/protocol/proxy-otel-api.md`) is identical — the WASM plugin points at the hosted URL instead of `localhost`.
- All SDK code is unchanged; users swap `SOFTPROBE_RUNTIME_URL` and `SOFTPROBE_API_KEY`.

---

## 2. System topology

```
                         ┌─────────────────────────────────┐
                         │         softprobe-runtime        │
test / CI ──────────────▶│  control API  │  OTLP backend   │
                         │  (Go, hosted) │  /v1/inject      │
                         │               │  /v1/traces      │
                         └──────┬────────┴────────┬─────────┘
                                │                 │
                       session  │                 │ span payloads
                       store    │                 │
                                ▼                 ▼
                             Redis             GCS bucket
                         (per-tenant,       (case files +
                          TTL sessions)      extract blobs)
                                              │
                                              │ on close (capture mode)
                                              ▼
                                        otel-server
                                    (BigQuery, per-tenant)
                                    historical session view
```

The runtime talks to three external systems:
1. **Supabase** (`auth.softprobe.ai`) — API key → tenant ID. Already running; the existing `/api/api-key/validate` endpoint is unchanged.
2. **Redis** — session control state (rules, policy, fixtures, revision, mode, loaded case ref). TTL-native, shared with otel-server.
3. **GCS** — case file blobs and extract payloads.

otel-server stays as-is. The runtime writes to GCS on capture `close`; otel-server ingests from GCS into BigQuery for the historical session browser.

---

## 3. Authentication and tenancy

### 3.1 HTTP auth

Every request to the hosted runtime carries:

```
Authorization: Bearer <api-key>
```

The runtime validates the key against `auth.softprobe.ai/api/api-key/validate`. The response gives `tenantId` plus the GCS bucket name for that tenant. Cache the result in-process with a short TTL (60 s) — identical to what otel-server already does.

The OSS self-hosted runtime keeps the existing `SOFTPROBE_API_TOKEN` env var (single static token, no tenancy). Zero changes to OSS behavior.

### 3.2 Session namespacing

Session IDs are globally unique (`sess_` + 12-byte random). Redis keys are namespaced by tenant:

```
session:{tenantId}:{sessionId}
```

An API key can only read/write sessions belonging to its own tenant. Cross-tenant access returns `404`, not `403`, to avoid information leakage.

### 3.3 User onboarding

Goal: working `softprobe doctor` in under 5 minutes.

1. User visits `app.softprobe.dev` → signs up with Google/GitHub via **Supabase Auth** (already powers `auth.softprobe.ai`).
2. After signup, the dashboard shows one command:
   ```bash
   export SOFTPROBE_RUNTIME_URL=https://runtime.softprobe.dev
   export SOFTPROBE_API_KEY=sk_live_...
   softprobe doctor
   ```
3. `softprobe doctor` passes → user follows existing quickstart docs unchanged.

No credit card on sign-up. Free tier: 10 sessions/day, 7-day case retention.

**3rd-party services used:**
- **Supabase** for identity, API key storage, and tenant metadata. Already running as `auth.softprobe.ai`. The dashboard is a thin UI on top of existing Supabase tables.

---

## 4. Session persistence

### 4.1 Redis session store

Each session is stored as a single JSON blob:

```
Key:   session:{tenantId}:{sessionId}
Value: JSON { id, tenantId, mode, revision, status, policy, rules,
              fixturesAuth, loadedCaseRef, stats }
TTL:   24h (reset on every write)
```

Every mutating control call does a read-modify-write with `WATCH`/`MULTI`/`EXEC` to atomically increment `revision` — same consistency guarantee as the current in-memory mutex.

Extract GCS paths are stored in a parallel list (appended per `POST /v1/traces`):

```
Key:   session:{tenantId}:{sessionId}:extracts
Value: Redis list of GCS object paths
TTL:   24h (same as session key)
```

This keeps large binary payloads out of Redis entirely.

The existing otel-server Redis instance is reused. No new infrastructure.

### 4.2 Extract payloads (capture mode)

When the WASM proxy posts `POST /v1/traces` with `sp.span.type=extract`, the hosted runtime writes the payload directly to GCS:

```
gs://{tenantBucket}/extracts/{sessionId}/{uuid}.otlp.json
```

and appends the object path to `session:{tenantId}:{sessionId}:extracts`.

On `close`, the runtime reads all paths from the extracts list, merges them into a case JSON document (same format as today), writes the result to:

```
gs://{tenantBucket}/cases/{sessionId}.case.json
```

and sets `loadedCaseRef` on the session.

### 4.3 Case files

`POST /v1/sessions/{id}/load-case` in the hosted runtime accepts either:
- A raw case JSON body (same as OSS) → stored to GCS, `loadedCaseRef` set.
- A GCS URI (`gs://...`) → `loadedCaseRef` set directly.

`GET /v1/sessions/{id}/case` (new endpoint, hosted only) returns the case JSON from `loadedCaseRef`. This lets SDKs load a remotely-stored case without managing file paths locally.

---

## 5. API changes (hosted only)

The control API spec is **unchanged**. These are additive hosted-only endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/cases/{caseId}` | Download a stored case JSON by session/case ID |
| `GET` | `/v1/sessions` | List open sessions for the authenticated tenant |

The existing OSS endpoints (`/health`, `/v1/meta`, `/v1/sessions`, `/v1/sessions/{id}/*`) are identical in behavior. The hosted runtime adds the auth middleware and swaps the store backend; nothing else in the API changes.

otel-server's `/api/tenants/{tenantId}/sessions` (historical session browser) remains on otel-server, accessed by the web UI for the "past captures" view. It does not move into softprobe-runtime.

---

## 6. What happens to otel-server

otel-server stays deployed and keeps doing what it does: OTLP ingestion from the WASM proxy, BigQuery persistence, the tenant session browser API for the web UI.

**One change:** remove `sp.api.key` extraction from span attributes as the auth mechanism. The WASM proxy should send `Authorization: Bearer <api-key>` as an HTTP header on its calls to `POST /v1/traces` (otel-server ingestion). This is a one-line change to the proxy WASM config (add `sp_api_key` as an HTTP header, not a span attribute) and removes a security smell.

otel-server's `/v1/inject` endpoint is not used in the current architecture (softprobe-runtime owns inject resolution). Leave it in place but do not wire it into the WASM plugin. It can be removed in a future cleanup.

---

## 7. Infrastructure

**Target: Cloud Run + GCP managed services.** The runtime is stateless — all mutable state lives in Redis and GCS.

| Component | Service |
|-----------|---------|
| `softprobe-runtime` (hosted) | Cloud Run (existing Docker image + env vars) |
| `otel-server` | Cloud Run (already deployed) |
| Session store | Redis — Cloud Memorystore (shared with otel-server) |
| Case/extract blobs | GCS (already provisioned per tenant) |
| Auth/identity/API keys | Supabase (already running as `auth.softprobe.ai`) |
| Web dashboard | Next.js on Vercel or Cloudflare Pages |
| Custom domain routing | Cloudflare (already used for docs) |

**3rd-party services summary:**
- **Supabase** — identity, API keys, tenant metadata. Already running; dashboard is a thin UI layer.
- **Redis (Cloud Memorystore)** — session persistence with native TTL. Already provisioned for otel-server.
- **GCS** — already in use. No new storage system.
- **Cloud Run** — serverless Go containers. No cluster to manage.

### Cloud Run deployment commands

```bash
# 1. Create a Serverless VPC Access connector (one-time, ~2 min)
gcloud compute networks vpc-access connectors create softprobe-connector \
  --project=coral-smoke-455007-j2 \
  --region=us-central1 \
  --network=default \
  --range=10.8.0.0/28

# 2. Deploy softprobe-runtime to Cloud Run
gcloud run deploy softprobe-runtime \
  --project=coral-smoke-455007-j2 \
  --region=us-central1 \
  --image=ghcr.io/softprobe/softprobe-runtime:latest \
  --service-account=softprobe-runtime@coral-smoke-455007-j2.iam.gserviceaccount.com \
  --vpc-connector=softprobe-connector \
  --allow-unauthenticated \
  --set-env-vars="SOFTPROBE_HOSTED=true,\
SOFTPROBE_LISTEN_ADDR=:8080,\
SOFTPROBE_AUTH_URL=https://auth.softprobe.ai/api/api-key/validate,\
REDIS_HOST=10.42.202.91,\
REDIS_PORT=6379,\
GCS_BUCKET=softprobe-otel-data,\
GCS_PROJECT=coral-smoke-455007-j2"

# 3. Post-deploy smoke test
SOFTPROBE_RUNTIME_URL=$(gcloud run services describe softprobe-runtime \
  --project=coral-smoke-455007-j2 --region=us-central1 \
  --format="value(status.url)") \
SOFTPROBE_API_KEY=<your-key> \
go test ./e2e/hosted/ -v -count=1
```

---

## 8. Implementation plan (phased)

### Phase 1 — Hosted runtime with durable sessions (no UI)

Goal: an engineer can point `SOFTPROBE_RUNTIME_URL` at the hosted instance, authenticate with an API key, and run the full capture/replay workflow end-to-end with sessions that survive restarts.

1. Add `Authorization: Bearer` middleware to softprobe-runtime; call `auth.softprobe.ai` to resolve tenant. Feature-flagged by `SOFTPROBE_HOSTED=true` env var — OSS behavior unchanged when flag is absent.
2. Implement `RedisStore` satisfying the same `Store` interface as the current in-memory store.
3. Wire extract payloads to GCS writes instead of in-memory append.
4. Wire `close` (capture mode) to GCS case file write and BigQuery ingestion via otel-server.
5. Add `GET /v1/cases/{caseId}` endpoint.
6. Deploy to Cloud Run. Smoke test with existing e2e suite pointed at the hosted URL.

### Phase 2 — Self-service onboarding

1. Build minimal dashboard on top of existing Supabase: sign up → generate API key → show quickstart command.
2. Lazy tenant provisioning on first API key use: create GCS bucket prefix if it doesn't exist.
3. Free tier quota enforcement (10 sessions/day) via a Supabase Postgres counter incremented on `POST /v1/sessions`.

### Phase 3 — Web UI (case browser)

Wire the existing otel-server `/api/tenants/{tenantId}/sessions` and the new `/v1/cases/{caseId}` into a simple read-only case browser in the web dashboard. Users can see their captured sessions, download case files, and copy the `softprobe session load-case` command.

---

## 9. Open questions

1. **otel-server language:** otel-server is Java/Spring; softprobe-runtime is Go. They are separate Cloud Run deployments that communicate over HTTP — no need to merge. Long-term, if BigQuery query logic needs to move into the runtime, that would be a Go rewrite. Not needed for Phase 1.

2. **Case retention:** 7-day free tier retention is a product decision. The enforcement mechanism is a GCS lifecycle rule on the tenant bucket + a background job or TTL on the Redis extracts list. Straightforward once the policy is decided.

3. **Inject fallback to historical data:** otel-server has a body-hash inject lookup against BigQuery history. The current softprobe-runtime design does not use it — inject is purely rule-driven. Leave the fallback out of Phase 1; revisit if users ask for "replay without writing any rules."

4. **Multi-region:** Cloud Memorystore Redis is regional. For Phase 1, single region (us-central1, same as otel-server) is fine. Cross-region replication is a Cloud Memorystore Enterprise feature if needed later.
