# HTTP control API

The JSON HTTP surface used by the CLI, SDKs, and any custom integration. The canonical spec and JSON schemas live at [`spec/protocol/http-control-api.md`](https://github.com/softprobe/softprobe/blob/main/spec/protocol/http-control-api.md) and [`spec/schemas/session-*.schema.json`](https://github.com/softprobe/softprobe/tree/main/spec/schemas); this page is the user-facing summary.

All endpoints live under `{runtimeBase}/v1/...`. Content type is `application/json` on requests and responses. Every mutating call increments `sessionRevision` (returned in the response).

::: info Scope of this page
This page documents the hosted runtime contract used by the CLI, SDKs, and
Softprobe proxy. The canonical base URL is `https://runtime.softprobe.dev`.
:::

## Base URL

`SOFTPROBE_RUNTIME_URL` defaults to `https://runtime.softprobe.dev`. The proxy
WASM `sp_backend_url` should point at the same hosted base URL.

## Health and metadata

### `GET /health`

```bash
curl https://runtime.softprobe.dev/health
```

Example response:

```json
{
  "status": "ok",
  "specVersion": "http-control-api@v1",
  "schemaVersion": "1"
}
```

Liveness probe. Returns 200 when the process is running; does not check
per-session state or downstream dependencies.

### `GET /v1/meta`

Runtime and contract metadata. Used by `softprobe doctor` to check CLI/SDK
alignment with the runtime.

Example response:

```json
{
  "runtimeVersion": "0.0.0-dev",
  "specVersion": "http-control-api@v1",
  "schemaVersion": "1"
}
```

## Session endpoints

Mutating endpoints return `200 OK` on success. New sessions start with
`sessionRevision = 0`; every subsequent mutating call increments it.

### `POST /v1/sessions`

Create a session.

Request:

```json
{
  "mode": "replay"
}
```

Response:

```json
{
  "sessionId": "sess_abc123",
  "sessionRevision": 0
}
```

### `POST /v1/sessions/{sessionId}/load-case`

Upload a case document.

Request body: the same JSON shape as an on-disk `*.case.json` file.

Response:

```json
{
  "sessionId": "sess_abc123",
  "sessionRevision": 1
}
```

### `POST /v1/sessions/{sessionId}/rules`

Replace the session's stored rule document (wholesale — the runtime stores what
you send; previous rules are discarded).

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
{
  "sessionId": "sess_abc123",
  "sessionRevision": 2
}
```

::: info SDKs merge, the runtime replaces
The runtime replaces the stored rules document on each call. SDK helpers such as
`mockOutbound(...)` keep a local merged list and resend the whole document so
consecutive calls accumulate.  If you call the endpoint directly (e.g. from `curl`), you are responsible for combining new rules with any existing ones.
:::

### `POST /v1/sessions/{sessionId}/policy`

Replace the session's stored policy document.

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
{
  "sessionId": "sess_abc123",
  "sessionRevision": 3
}
```

### `POST /v1/sessions/{sessionId}/fixtures/auth`

Replace the session's stored auth-fixtures document (non-HTTP auth material such
as bearer tokens and cookies that replay should present).

Request:

```json
{
  "tokens": ["stub_eyJhbGciOi..."],
  "cookies": [
    { "name": "session", "value": "stub_cookie", "domain": "example.test", "path": "/" }
  ],
  "metadata": {
    "source": "test-fixture"
  }
}
```

Response (200):

```json
{
  "sessionId": "sess_abc123",
  "sessionRevision": 4
}
```

### `GET /v1/sessions/{sessionId}/stats`

Read session counters. Cheap to poll.

Example response:

```json
{
  "sessionId": "sess_abc123",
  "sessionRevision": 4,
  "mode": "replay",
  "stats": {
    "injectedSpans": 2,
    "extractedSpans": 0,
    "strictMisses": 0
  }
}
```

### `POST /v1/sessions/{sessionId}/close`

Close the replay session and delete runtime session state.

Response:

```json
{
  "sessionId": "sess_abc123",
  "closed": true
}
```

After `close`, the session id is invalid for future control-plane calls.

## Hosted capture export

### `GET /v1/captures/{captureId}`

Return a tenant-scoped capture JSON assembled from datalake query results.

```bash
curl -H "Authorization: Bearer $SOFTPROBE_API_TOKEN" \
  "https://runtime.softprobe.dev/v1/captures/cap_123"
```

## Errors

All errors return a JSON envelope with a nested `error` object:

```json
{
  "error": {
    "code": "unknown_session",
    "message": "unknown session"
  }
}
```

| HTTP | `error.code` | When |
|---|---|---|
| 400 | `invalid_request` | Body could not be parsed or was empty |
| 401 | `missing_bearer_token` | Auth enabled and no `Authorization: Bearer` header |
| 403 | `invalid_bearer_token` | Auth enabled and token did not match |
| 404 | `unknown_session` | Unknown or already-closed `sessionId` |
| 405 | `method_not_allowed` | Wrong HTTP method for the route |
| 500 | `internal_error` | Runtime-side failure (e.g. writing a captured case) |

## Authentication

Every hosted `/v1/...` request must carry the header:

```bash
curl -H "Authorization: Bearer $SOFTPROBE_API_TOKEN" \
  https://runtime.softprobe.dev/v1/sessions
```

`/health` is unauthenticated for reachability checks. The CLI and SDKs read
`SOFTPROBE_API_TOKEN` from the environment and attach the header automatically.

## Relationship to the proxy

Tests, SDKs, and the CLI use the JSON control API. The proxy uses the OTLP API:

- `POST /v1/inject`
- `POST /v1/traces`

Both handler groups are served by the hosted runtime and share the same
tenant-scoped session state.

## See also

- [CLI reference](/reference/cli)
- [Case schema](/reference/case-schema)
- [Rule schema](/reference/rule-schema)
- [Sessions and cases](/concepts/sessions-and-cases)
