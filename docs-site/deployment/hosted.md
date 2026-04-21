# Hosted deployment (o.softprobe.ai)

`https://o.softprobe.ai` is the managed Softprobe runtime operated by the Softprobe team. It speaks the same HTTP control API and OTLP trace API as the OSS runtime — you can point any SDK, CLI, or proxy at it without code changes.

Use it when you want:

- **Zero ops** — no container to run, no schema to upgrade.
- **Durable captures** — case files stream to per-org object storage with lifecycle policies.
- **Multi-tenant isolation** — per-org API keys, sessions, and quotas.
- **Built-in PII redaction** — optional org-wide redaction rules applied before data hits storage.

## Getting started

1. **Sign up** at [softprobe.dev/signup](https://softprobe.dev/signup) with a work email. A team member will verify and activate the org.
2. **Generate an API key** in the dashboard under *Settings → API keys*.
3. **Point your tooling** at `https://o.softprobe.ai`:

   ```bash
   export SOFTPROBE_RUNTIME_URL=https://o.softprobe.ai
   export SOFTPROBE_API_TOKEN=sp_live_...
   ```

4. **Run `softprobe doctor`** to confirm connectivity.

## Authentication

Every request to the hosted runtime must carry:

```
Authorization: Bearer sp_live_...
```

SDKs and the CLI read `SOFTPROBE_API_TOKEN` from the environment automatically.

### Key types

| Prefix | Purpose | Scope |
|---|---|---|
| `sp_live_` | Long-lived org API key | Create sessions, load cases, manage rules |
| `sp_ci_` | CI-scoped key | Same as live, restricted to an org project |
| `sp_test_` | Test account | Rate-limited for development |

### Rotation

Rotate keys in the dashboard. Old keys continue to work for **24 hours** after the new key is issued.

## Regions

| Region | Endpoint |
|---|---|
| US East (default) | `https://o.softprobe.ai` |
| EU West | `https://eu.o.softprobe.ai` |
| Asia Pacific | `https://ap.o.softprobe.ai` |

Data stays in the region where it was captured. EU and AP regions are GDPR / APPI-compliant.

## Capture storage

All captures stream to a per-org bucket provisioned automatically. Access captures via:

- **Dashboard** — human-friendly browser with filters, diff, and export.
- **CLI** — `softprobe captures list`, `softprobe captures get $ID --out cases/x.case.json`.
- **API** — `GET /v1/captures?limit=100` returns an array of metadata.

### Retention

Default: **30 days**. Configurable per org (7–365 days) in *Settings → Retention*.

### Export to your own bucket

For long-term archiving or feeding an observability pipeline:

```bash
softprobe export otlp \
  --since 24h \
  --endpoint https://my-otel-collector.example.com/v1/traces
```

Or configure S3 Streaming Export in the dashboard (enterprise tier).

## Rate limits

| Tier | Sessions / minute | Sessions / month | Replay volume |
|---|---|---|---|
| **Free** | 60 | 5,000 | 50 MB / month |
| **Team** | 600 | 500,000 | 10 GB / month |
| **Enterprise** | custom | custom | custom |

Current usage and hard limits are in the dashboard.

## Proxy configuration

Point your mesh's Softprobe WASM at the hosted endpoint:

```yaml
pluginConfig:
  sp_backend_url: https://o.softprobe.ai
  sp_api_token: sp_live_...     # injected via K8s secret
```

If your cluster egress is restricted, allowlist:

- `o.softprobe.ai:443` (or the regional variant)
- `storage.softprobe.ai:443` (for capture uploads)

## SLA

- **Availability:** 99.9% monthly (Team), 99.99% (Enterprise).
- **Latency:** P50 < 30ms, P99 < 200ms for `/v1/inject` from North America, EU, and APAC regions.
- **Status page:** [status.softprobe.dev](https://status.softprobe.dev).

## Multi-tenant model

- **Sessions** are namespaced per org; knowing a session id from another tenant is not sufficient to access it.
- **API keys** never cross orgs.
- **Captures** are encrypted at rest with per-org keys.
- **Audit logs** show who created/deleted/closed sessions and when.

## Compliance

| Standard | Status |
|---|---|
| SOC 2 Type II | audited annually |
| GDPR | EU region provides Data Processing Agreement |
| HIPAA | available on Enterprise with BAA |
| ISO 27001 | certified |

Compliance documentation is available under NDA at [softprobe.dev/security](https://softprobe.dev/security).

## Migration from OSS → Hosted

1. Keep your OSS runtime running.
2. Point new test runs at the hosted endpoint.
3. Export existing captures from the OSS runtime:

   ```bash
   softprobe captures export --runtime-url http://my-oss:8080 --out oss-backup/
   softprobe captures import --runtime-url https://o.softprobe.ai oss-backup/
   ```

4. Decommission the OSS runtime once the migration is verified.

The SDK, CLI, and proxy WASM are identical — only `SOFTPROBE_RUNTIME_URL` and the bearer token change.

## Pricing

See [softprobe.dev/pricing](https://softprobe.dev/pricing). Free tier for individuals and small teams; paid tiers for capture volume and SSO / audit features.

## Support

- **Docs & guides:** you're reading them.
- **Community:** [softprobe.dev/community](https://softprobe.dev/community)
- **Team plan:** email `support@softprobe.dev` (response within one business day).
- **Enterprise:** dedicated Slack connect channel + named engineer.

## Next

- [Installation](/installation) — CLI and SDK install for local + hosted use.
- [CI integration](/guides/ci-integration) — running suites against the hosted runtime.
- [Session headers](/reference/session-headers) — propagation rules (same in OSS and hosted).
