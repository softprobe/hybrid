# Auth fixtures (non-HTTP secrets)

Most authentication in Softprobe is handled **through** captured HTTP traffic — your login flow is just another sequence of HTTP hops, captured once and replayed deterministically. But some auth material doesn't travel as HTTP and needs a different channel: API keys in environment variables, rotating bearer tokens, signed cookies, OAuth access tokens the test harness already holds.

That's what **auth fixtures** are for.

## When to use auth fixtures (and when not to)

**Use HTTP-captured auth when:**

- The login flow itself is an HTTP exchange (e.g. `POST /oauth/token`).
- The token is returned in a response body and forwarded to subsequent calls.
- You want the login flow to be part of the test's fidelity.

**Use auth fixtures when:**

- The test harness holds a token from a prior setup step (e.g. a CI job that authenticates once and hands every test a fresh JWT).
- The SUT reads secrets from env vars, not HTTP responses.
- You need to register cookies or signed headers the captured case doesn't include.
- You need to surface metadata (tenant id, feature flag) to matchers or hooks without inventing a dummy HTTP call.

Auth fixtures are **not**:

- A way to inject responses — use `mockOutbound` for that.
- A way to configure the proxy — proxy config is static YAML.
- A secret-scanning layer — they're just key/value pairs held in session memory.

## Control-API shape

The normative endpoint is [`POST /v1/sessions/{id}/fixtures/auth`](/reference/http-control-api#post-v1-sessions-sessionid-fixtures-auth).

Request:

```json
{
  "fixtures": [
    { "name": "auth_token", "value": "stub_eyJhbGci..." },
    { "name": "tenant_id",  "value": "tenant_01" },
    { "name": "idempotency_key", "value": "test-run-42" }
  ]
}
```

Response:

```json
{ "sessionRevision": 4 }
```

The fixture document **replaces** any previously-registered fixtures on the session (same semantics as rules). Bump-safe: `sessionRevision` is incremented.

Fixtures live in session memory only — they are **not** persisted to the case file on close, and they are not echoed back in `sessionRevision`-keyed proxy caches.

## SDK wrappers

Each SDK provides a one-line wrapper:

### TypeScript

```ts
await session.setAuthFixtures([
  { name: 'auth_token', value: process.env.TEST_OAUTH_TOKEN! },
  { name: 'tenant_id',  value: 'tenant_01' },
]);
```

### Python

```python
session.set_auth_fixtures([
    {"name": "auth_token", "value": os.environ["TEST_OAUTH_TOKEN"]},
    {"name": "tenant_id",  "value": "tenant_01"},
])
```

### Java

```java
session.setAuthFixtures(List.of(
    AuthFixture.of("auth_token", System.getenv("TEST_OAUTH_TOKEN")),
    AuthFixture.of("tenant_id",  "tenant_01")
));
```

### Go

```go
err := session.SetAuthFixtures(ctx, []softprobe.AuthFixture{
    {Name: "auth_token", Value: os.Getenv("TEST_OAUTH_TOKEN")},
    {Name: "tenant_id",  Value: "tenant_01"},
})
```

All four are thin wrappers over the control-API POST. They do **not** merge — call with the full desired fixture set.

## How fixtures surface to tests

Fixtures are available in three places:

### 1. From the test directly (read-back)

```ts
const current = await session.getFixtures();
console.log(current.tenant_id);   // "tenant_01"
```

Use sparingly — usually the test already has the values in scope.

### 2. In hooks (suite runs)

Hook contexts receive `ctx.fixtures` as a plain object:

```ts
export const rewriteHeader = (ctx) => {
  ctx.request.headers['x-tenant'] = ctx.fixtures.tenant_id;
  return { request: ctx.request };
};
```

This is the most common use — hook rewrites a captured request body or header based on per-run fixture material.

### 3. In matchers (rule bodies)

Rules can reference fixtures via `${fixtures.name}` templating (v1 preview, opt-in):

```yaml
- id: auth-ok
  when:
    direction: outbound
    host: api.example.com
    headers:
      authorization: "Bearer ${fixtures.auth_token}"
  then:
    action: mock
    response: { status: 200, body: '{"ok":true}' }
```

Enable with `softprobe runtime --feature fixture-templates=true`. This is a preview feature — prefer hooks for stable rewrites.

## Lifetime and teardown

Fixtures live as long as the session. On `close`, they're discarded with the rest of the session state.

Between tests in the same session:

- **Keep fixtures**: just call `mockOutbound` / `findInCase` as usual. Fixtures carry over.
- **Replace fixtures**: call `setAuthFixtures` with the new list — the old list is dropped.
- **Clear fixtures**: call `setAuthFixtures([])`.

Fixtures **are not** reset by `clearRules()` — that method only affects rules.

## CI example

A GitHub Actions job that authenticates once and injects the token into every test session:

```yaml
jobs:
  replay:
    steps:
      - uses: actions/checkout@v4
      - name: Obtain test OAuth token
        id: auth
        run: |
          TOKEN=$(curl -s -X POST https://idp.example.com/token \
            -d "client_id=$CI_CLIENT_ID&client_secret=$CI_CLIENT_SECRET&grant_type=client_credentials" \
            | jq -r .access_token)
          echo "token=$TOKEN" >> $GITHUB_OUTPUT
      - run: npm test
        env:
          TEST_OAUTH_TOKEN: ${{ steps.auth.outputs.token }}
```

Your Jest setup then reads `process.env.TEST_OAUTH_TOKEN` and calls `setAuthFixtures(...)` before each session's tests.

## Security notes

- Fixtures are in-memory only. They are **not** written to case files on `close`.
- Avoid echoing fixture values into assertion messages or logs — CI log aggregators index these strings.
- Prefer short-lived tokens: `setAuthFixtures` is cheap to re-call between tests if tokens rotate.

## See also

- [HTTP control API → `POST /v1/sessions/{id}/fixtures/auth`](/reference/http-control-api#post-v1-sessions-sessionid-fixtures-auth)
- [TypeScript SDK → `setAuthFixtures`](/reference/sdk-typescript)
- [Ship rules with a case](/guides/ship-rules-with-a-case) — static fixtures embedded per-case.
- [Write a hook](/guides/write-a-hook) — where fixtures usually get consumed.
