# Troubleshooting

This page lists the failures you're most likely to hit, what causes them, and
how to fix them. Commands assume the hosted runtime and a local app/proxy stack.

## The first thing to run

```bash
softprobe doctor
```

`doctor` reports:

- runtime reachability (`SOFTPROBE_RUNTIME_URL`)
- CLI ↔ runtime version compatibility
- proxy WASM binary presence and version
- expected environment variables
- spec / schema alignment

Most "what's wrong?" questions get answered here.

## Capture

### `extractedSpans: 0` on a closed session

Your traffic didn't carry the session header, or it didn't pass through the proxy.

**Check 1: is the header set?**

```bash
curl -v -H "x-softprobe-session-id: $SOFTPROBE_SESSION_ID" http://127.0.0.1:8082/hello
# Look for: > x-softprobe-session-id: sess_...
```

**Check 2: is the request hitting the proxy, not the app directly?**

```bash
# WRONG: this skips the proxy
curl -H "x-softprobe-session-id: $SOFTPROBE_SESSION_ID" http://127.0.0.1:8081/hello

# RIGHT: through the ingress listener
curl -H "x-softprobe-session-id: $SOFTPROBE_SESSION_ID" http://127.0.0.1:8082/hello
```

**Check 3: is the proxy actually calling the runtime?**

```bash
docker logs e2e-softprobe-proxy-1 2>&1 | grep -i backend
# Look for successful calls to sp_backend_url
```

If the proxy logs show connection failures to the backend, check the
`sp_backend_url` in `envoy.yaml` — it must be `https://runtime.softprobe.dev`.
Also confirm `public_key` is set to your hosted API token.

### Captured file missing `/fragment` (or another egress hop)

The app likely called the dependency directly, not through the egress proxy. Two common causes:

1. **Missing `EGRESS_PROXY_URL`.** The app container needs to route outbound HTTP through the egress listener. Set it in `docker-compose.yaml`:

   ```yaml
   environment:
     EGRESS_PROXY_URL: http://softprobe-proxy:8084
   ```

2. **App uses a non-OpenTelemetry HTTP client.** For W3C Trace Context propagation, use `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`, `@opentelemetry/instrumentation-http`, or equivalent. Bare clients drop `traceparent`/`tracestate` — the proxy still sees the hop but can't correlate it to a session on some setups.

### Captured response body is `"[REDACTED]"`

You're running a `capture_only` redaction rule. Check your applied rules:

```bash
softprobe inspect session --session "$SOFTPROBE_SESSION_ID" --json | jq '.rules'
```

Adjust or remove the redaction rule if you want the raw body.

### Closed session did not create a local case file

The hosted runtime stores the captured case remotely. Use the CLI close command
with `--out` to download a copy into your repository:

```bash
softprobe session close --session "$SOFTPROBE_SESSION_ID" --out cases/out.case.json
```

## Replay — SDK errors

### `findInCase threw: 0 matches`

No span in the loaded case matches your predicate. In order of likelihood:

1. The capture didn't include that hop — inspect with `softprobe inspect case <file>`.
2. The predicate is wrong (typo in method, host, path).
3. The case file path is wrong — `loadCaseFromFile` loaded nothing. Check the resolved path:

   ```ts
   console.log(path.resolve(__dirname, '../cases/checkout.case.json'));
   ```

### `findInCase threw: N matches (N > 1)`

Multiple spans match. Narrow the predicate by adding `method`, `host`, `pathPrefix`, or `direction`. Or, if you want them all:

```ts
const hits = session.findAllInCase({ direction: 'outbound' });
for (const hit of hits) {
  await session.mockOutbound({ /* matched predicate */, response: hit.response });
}
```

### `Session not found (404)`

Someone closed the session, the session expired, or the token belongs to a
different tenant. Recreate the session.

Common pitfall: Jest's `beforeAll` ran on worker A, but the test runs on worker B (Jest isolates each test file). Put `startSession` in the same file as the test.

### `Runtime unreachable`

```bash
softprobe doctor --verbose
```

Verify `SOFTPROBE_API_TOKEN` is set and that your laptop or CI runner can reach
`https://runtime.softprobe.dev` over HTTPS.

### `x-softprobe-session-id` rejected

The runtime rejects unknown session ids with `404`. Your test probably hit `http://127.0.0.1:8082` without setting the header, or typed the session id wrong. Log it:

```ts
console.log('session id:', sessionId);
```

## Replay — test failures

### Response body is `{"dep":"ok"}` instead of the expected nested object

The SUT version is different from the captured version. Rebuild the app, or update the assertion to match the current shape. Inspect the actual response:

```ts
const res = await fetch(appUrl, { headers: { 'x-softprobe-session-id': sessionId } });
console.log(await res.json());
```

### Assertion fails with "extra fields" in strict mode

By default, Softprobe's body comparison is JSON-subset (actual may have fields the captured didn't). If you switched to `mode: exact`, every field must match — use `ignore:` for volatile fields.

### Test passes locally but fails in CI with `ECONNREFUSED`

CI must use the same hosted runtime and token as local development. Make sure
you do not hard-code a local URL:

```ts
const softprobe = new Softprobe({ baseUrl: process.env.SOFTPROBE_RUNTIME_URL });
```

And set the env var in CI config.

### `403 Forbidden` on outbound under strict policy

Strict policy blocks anything not mocked. The proxy actually returns **`599`** with `x-softprobe-strict-miss: 1`, but some HTTP clients surface it as 403, 5xx, or a generic error. Either mock the missing hop, or move that host to the allowlist:

```ts
await session.setPolicy({
  externalHttp: 'strict',
  externalAllowlist: ['internal.svc.cluster.local', 'auth.internal'],
});
```

For a full walkthrough (including how to read runtime logs and decide between `mockOutbound`, policy relaxation, and passthrough), see [Debug a strict-policy miss](/guides/debug-strict-miss).

## Proxy / runtime

### Proxy returns 503 on every hop

The WASM filter failed to load (usually because `sp_istio_agent.wasm` is missing or the wrong architecture). Verify:

```bash
docker exec e2e-softprobe-proxy-1 ls -la /etc/envoy/sp_istio_agent.wasm
# Should be > 0 bytes
```

If missing, rebuild / re-download the WASM binary (see [Installation](/installation#proxy)).

### Runtime 500 on `POST /v1/sessions/$ID/rules`

Your rules payload is malformed. Validate against the schema:

```bash
softprobe validate rules < rules.yaml
```

Common issues:

- `when` has unknown fields (typos like `direciton`).
- `then.action` not in `{mock, error, passthrough, capture_only}`.
- `response.body` is neither a string nor JSON-serializable.

### Slow response (>1s) on every request in replay mode

The proxy may be syncing to the runtime on every hop without caching. Enable the
proxy's inject cache:

```yaml
# envoy.yaml
wasm_config:
  configuration:
    inject_cache_size: 4096  # entries
```

Cache keys include `sessionRevision`, so mutations invalidate entries correctly.

## Suite / CLI

### `softprobe: command not found`

The CLI isn't on your `PATH`. If installed via the curl script:

```bash
ls /usr/local/bin/softprobe
export PATH="$PATH:/usr/local/bin"
```

Via Homebrew, restart your shell. Via npm, call it as `npx softprobe`.

### `suite run` exits 0 but reports "0 tests"

The glob matched no cases. Check:

```bash
ls cases/checkout/*.case.json
```

If the glob is right, make sure you're in the directory the YAML expects. Glob resolution is relative to the CWD where you ran `suite run`, not to the YAML file.

### `hook function not found: checkout.unmaskCard`

Either the hook file wasn't passed via `--hooks`, or the export name is wrong. Remember the naming: `<fileBasename>.<exportName>`.

```bash
softprobe suite run ... --hooks hooks/checkout.ts --verbose
# [hooks] loaded: checkout.unmaskCard, checkout.assertTotalsMatchItems
```

## OpenTelemetry / trace propagation

### My egress mocks aren't hit {#my-egress-mocks-arent-hit}

**Symptom:** you call `mockOutbound({ direction: 'outbound', ... })`, but the request hits the real upstream anyway (or errors under strict policy), as if the rule didn't exist.

**Root cause (90% of the time):** outbound OpenTelemetry propagation is broken. The egress proxy never sees the session id in `tracestate`, so it treats every outbound call as untagged and the runtime can't find a matching session.

**Confirm in 30 seconds:**

1. Start a **capture** session against the same app and run one end-to-end request.
2. `softprobe inspect case <path>` (or `jq '.traces[].resourceSpans[].scopeSpans[].spans[] | .attributes[] | select(.key == "sp.traffic.direction")' captured.case.json`).
3. If you see `inbound` spans but **no** `outbound` spans, propagation is broken.

**Fixes by language:**

| Language | Fix |
|---|---|
| Node.js | Use `@opentelemetry/instrumentation-http` auto-instrumentation. If you use `fetch` on Node < 20, wrap it with `undici` + OTEL instrumentation. |
| Python | `opentelemetry-instrumentation-requests` or `opentelemetry-instrumentation-httpx`. Install the auto-instrumentation agent and verify with `OTEL_LOG_LEVEL=debug`. |
| Java | Run with the OpenTelemetry Java Agent (`-javaagent:opentelemetry-javaagent.jar`). HTTP auto-instrumentation covers Apache HttpClient, OkHttp, JDK `HttpClient`, and others. |
| Go | Wrap your `http.Client` with `otelhttp.NewTransport(http.DefaultTransport)`. The default client does **not** propagate. |

**Verify propagation:** add a debug log in the app that prints `traceparent` on inbound requests and again right before every outbound call — if `traceparent` is missing or doesn't match, your instrumentation isn't wired in.

You do **not** manually forward `x-softprobe-session-id` — the proxy puts session correlation into `tracestate` on ingress, and the OTel propagator moves it through.

## Still stuck?

1. **Run `softprobe doctor --verbose`** — it checks version drift, header propagation, and produces a diagnostic bundle.
2. **Tail proxy and app logs** — proxy logs show backend connectivity and inject misses.
3. **File an issue** with the doctor output attached at [github.com/softprobe/softprobe/issues](https://github.com/softprobe/softprobe/issues).
4. **Ask the community** at [softprobe.dev/community](https://softprobe.dev/community).

---

## Quick reference

```bash
# Health checks
softprobe doctor
curl http://127.0.0.1:8081/health          # your SUT
curl http://127.0.0.1:8082/                # ingress listener

# Session introspection
softprobe inspect session --session "$ID"
softprobe session stats --session "$ID"

# Logs
docker logs -f e2e-softprobe-proxy-1

# Case file
softprobe inspect case cases/checkout.case.json
softprobe validate case cases/checkout.case.json
```
