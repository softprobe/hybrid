# HTTP control API

The JSON HTTP surface used by the CLI, SDKs, and any custom integration. The canonical spec and JSON schemas live at [`spec/protocol/http-control-api.md`](https://github.com/softprobe/softprobe/blob/main/spec/protocol/http-control-api.md) and [`spec/schemas/session-*.schema.json`](https://github.com/softprobe/softprobe/tree/main/spec/schemas); this page is the user-facing summary.

All endpoints live under `{runtimeBase}/v1/...`. Content type is `application/json` on requests and responses. Every mutating call increments `sessionRevision` (returned in the response).

::: info Scope of this page
This page documents what the current OSS `softprobe-runtime` in this repository
actually serves. One read-only endpoint is called out under [Planned](#planned)
where the design defines a future direction but the implementation is not yet
wired.
:::

## Base URL

In local OSS setups, `SOFTPROBE_RUNTIME_URL` (for CLI/SDK) and `sp_backend_url` (for the proxy WASM) both point at the **same** runtime base URL. For the reference Docker Compose, that is `http://127.0.0.1:8080` from the host and `http://softprobe-runtime:8080` from inside the compose network.

Hosted deployments use `https://runtime.softprobe.dev`.

## Health and metadata

### `GET /health`

```bash
curl $SOFTPROBE_RUNTIME_URL/health
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

Close the session. In capture mode, the runtime flushes buffered traces to a case
file before deleting the session.

Response:

```json
{
  "sessionId": "sess_abc123",
  "closed": true
}
```

After `close`, the session id is invalid for future control-plane calls.

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

The OSS runtime is **unauthenticated by default** — use only on trusted networks
or behind a reverse proxy.

To require a bearer token, set `SOFTPROBE_API_TOKEN` on the runtime process:

```bash
docker run -e SOFTPROBE_API_TOKEN=sp_secret ghcr.io/softprobe/softprobe-runtime:latest
```

When set, every `/v1/...` request must carry the header:

```bash
curl -H "Authorization: Bearer sp_secret" $SOFTPROBE_RUNTIME_URL/v1/sessions
```

`/health` is always unauthenticated so orchestrators can probe it. The CLI and
SDKs read `SOFTPROBE_API_TOKEN` from the environment and attach the header
automatically.

## Planned

The design references one read-only endpoint that is **not** yet implemented in
the OSS runtime:

- `GET /v1/sessions/{sessionId}` — full session snapshot (policy, rule count,
  case summary, stats in one payload). Until it lands, use
  `GET /v1/sessions/{sessionId}/stats` for counters.

## Relationship to the proxy

Tests, SDKs, and the CLI use the JSON control API. The proxy uses the OTLP API:

- `POST /v1/inject`
- `POST /v1/traces`

In the OSS reference layout, both handler groups live in the same
`softprobe-runtime` process and share one in-memory session store.

## See also

- [CLI reference](/reference/cli)
- [Case schema](/reference/case-schema)
- [Rule schema](/reference/rule-schema)
- [Sessions and cases](/concepts/sessions-and-cases)
