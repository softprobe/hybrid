# HTTP control API

The JSON HTTP surface used by the CLI, SDKs, and any custom integration. The canonical spec and JSON schemas live at [`spec/protocol/http-control-api.md`](https://github.com/softprobe/softprobe/blob/main/spec/protocol/http-control-api.md) and [`spec/schemas/session-*.schema.json`](https://github.com/softprobe/softprobe/tree/main/spec/schemas); this page is the user-facing summary.

All endpoints live under `{runtimeBase}/v1/...`. Content type is `application/json` on requests and responses. Every mutating call increments `sessionRevision` (returned in the response).

## Base URL

In local OSS setups, `SOFTPROBE_RUNTIME_URL` (for CLI/SDK) and `sp_backend_url` (for the proxy WASM) both point at the **same** runtime base URL. For the reference Docker Compose, that is `http://127.0.0.1:8080` from the host and `http://softprobe-runtime:8080` from inside the compose network.

Hosted deployments use `https://o.softprobe.ai`.

## Health

### `GET /health`

```bash
curl http://127.0.0.1:8080/health
# 200 OK
# {"status":"ok","version":"0.5.0"}
```

Liveness probe. Returns 200 when the process is running; does not check downstream dependencies.

## Sessions

### `POST /v1/sessions`

Create a new session.

Request:
```json
{
  "mode": "replay"
}
```

Response (201):
```json
{
  "sessionId": "sess_01H7P8Q4XYZ...",
  "sessionRevision": 1,
  "mode": "replay",
  "createdAt": "2026-04-20T10:00:00Z"
}
```

| Body field | Values | Purpose |
|---|---|---|
| `mode` | `"capture"`, `"replay"`, `"generate"` | Determines what the runtime does with observed traffic |

### `POST /v1/sessions/{sessionId}/load-case`

Upload a case document.

Request body: same shape as an on-disk `*.case.json` file (top-level `version`, `caseId`, `traces`, `rules?`, `fixtures?`). Schema: [`case.schema.json`](/reference/case-schema).

Response (200):
```json
{
  "sessionRevision": 2,
  "traceCount": 3
}
```

### `POST /v1/sessions/{sessionId}/rules`

Replace the session's rule document (wholesale — the runtime stores what you send, previous rules are discarded).

Request:
```json
{
  "version": 1,
  "rules": [
    {
      "id": "stripe-mock",
      "priority": 100,
      "when": {
        "direction": "outbound",
        "hostSuffix": "stripe.com",
        "method": "POST",
        "pathPrefix": "/v1/payment_intents"
      },
      "then": {
        "action": "mock",
        "response": {
          "status": 200,
          "headers": { "content-type": "application/json" },
          "body": "{\"id\":\"pi_test\",\"status\":\"succeeded\"}"
        }
      }
    }
  ]
}
```

Response (200):
```json
{ "sessionRevision": 3, "ruleCount": 1 }
```

::: info SDKs handle the merge
Because the runtime replaces rules wholesale, the SDK's `mockOutbound` merges with its in-memory state before posting. If you call the endpoint directly (e.g. from `curl`), you are responsible for combining new rules with any existing ones.
:::

### `POST /v1/sessions/{sessionId}/policy`

Set session policy.

Request:
```json
{
  "externalHttp": "strict",
  "externalAllowlist": ["localhost", "internal.svc"],
  "defaultOnMiss": "error"
}
```

Response (200):
```json
{ "sessionRevision": 4 }
```

### `POST /v1/sessions/{sessionId}/fixtures/auth`

Register non-HTTP auth material (tokens, cookies).

Request:
```json
{
  "fixtures": [
    { "name": "auth_token", "value": "stub_eyJ..." }
  ]
}
```

### `GET /v1/sessions/{sessionId}`

Read current session state (policy, rule count, case summary, stats).

```bash
curl http://127.0.0.1:8080/v1/sessions/$SESSION_ID | jq
```

```json
{
  "sessionId": "sess_...",
  "sessionRevision": 4,
  "mode": "replay",
  "policy": { "externalHttp": "strict" },
  "ruleCount": 2,
  "case": { "caseId": "checkout", "traceCount": 3 },
  "stats": { "extractedSpans": 0, "injectedSpans": 2 }
}
```

### `GET /v1/sessions/{sessionId}/stats`

Counters only, cheap to poll.

### `POST /v1/sessions/{sessionId}/close`

End the session. For `mode: capture`, the runtime flushes buffered traces to the configured path.

Response (200):
```json
{
  "closed": true,
  "casePath": "/cases/captured-sess_01H...case.json",
  "traceCount": 3
}
```

After `close`, the `sessionId` is invalid; follow-up calls return 404.

## Errors

All errors return a stable envelope:

```json
{
  "error": "session_not_found",
  "message": "session 'sess_xxx' does not exist or has been closed",
  "code": 4
}
```

| HTTP | `error` | When |
|---|---|---|
| 400 | `invalid_body` | JSON parse or schema validation failed |
| 400 | `invalid_mode` | Unsupported `mode` value |
| 404 | `session_not_found` | Unknown or closed `sessionId` |
| 409 | `session_closed` | Already closed |
| 422 | `rule_validation_failed` | `rules` document violates `rule.schema.json` |
| 500 | `internal_error` | Bug — file an issue |

## Version and schema alignment

`GET /v1/meta` returns:

```json
{
  "runtimeVersion": "0.5.0",
  "specVersion": "v1",
  "supportedSchemas": ["case.schema.json", "rule.schema.json", ...]
}
```

`softprobe doctor` checks your CLI and SDK against this metadata.

## Authentication

The OSS runtime is **unauthenticated by default** — use only on trusted networks or behind a reverse proxy.

To require a bearer token, set `SOFTPROBE_API_TOKEN` when starting the runtime:

```bash
docker run -e SOFTPROBE_API_TOKEN=sp_secret ghcr.io/softprobe/softprobe-runtime:v0.5
```

Then pass it on every request:

```bash
curl -H "Authorization: Bearer sp_secret" http://.../v1/sessions
```

SDKs and the CLI read `SOFTPROBE_API_TOKEN` from the environment.

## Rate limits

The OSS runtime does not rate-limit. Hosted deployments apply per-org quotas; see [Hosted deployment](/deployment/hosted).

## See also

- [CLI reference](/reference/cli) — high-level commands that wrap these endpoints.
- [Sessions and cases](/concepts/sessions-and-cases) — the mental model.
- [Session headers](/reference/session-headers) — the inbound header protocol used alongside these endpoints.
