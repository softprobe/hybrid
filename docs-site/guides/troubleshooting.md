# Troubleshooting

This page lists the failures you're most likely to hit, what causes them, and how to fix them. Commands assume the reference Docker Compose stack; adapt addresses for your environment.

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
curl -v -H "x-softprobe-session-id: $SESSION_ID" http://127.0.0.1:8082/hello
# Look for: > x-softprobe-session-id: sess_...
```

**Check 2: is the request hitting the proxy, not the app directly?**

```bash
# WRONG: this skips the proxy
curl -H "x-softprobe-session-id: $SESSION_ID" http://127.0.0.1:8081/hello

# RIGHT: through the ingress listener
curl -H "x-softprobe-session-id: $SESSION_ID" http://127.0.0.1:8082/hello
```

**Check 3: is the proxy actually calling the runtime?**

```bash
docker logs e2e-softprobe-proxy-1 2>&1 | grep -i backend
# Look for successful calls to sp_backend_url
```

If the proxy logs show connection failures to `softprobe-runtime:8080`, check the `sp_backend_url` in `envoy.yaml` — it must point at the runtime container, not `127.0.0.1`.

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
curl -s "http://127.0.0.1:8080/v1/sessions/$SESSION_ID/rules" | jq
```

Adjust or remove the redaction rule if you want the raw body.

### `SOFTPROBE_CAPTURE_CASE_PATH is not set — no case file written`

The runtime only flushes to disk if the env var is set. Restart with it configured:

```bash
docker run \
  -e SOFTPROBE_CAPTURE_CASE_PATH=/cases/out.case.json \
  -v $PWD/cases:/cases \
  ghcr.io/softprobe/softprobe-runtime:v0.5
```

In hosted deployments, set the path (or object-storage URL) in the runtime's config.

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

Someone closed the session, or the runtime restarted. Sessions are in-memory (v1). Recreate the session.

Common pitfall: Jest's `beforeAll` ran on worker A, but the test runs on worker B (Jest isolates each test file). Put `startSession` in the same file as the test.

### `Runtime unreachable (ECONNREFUSED)`

```bash
curl -v http://127.0.0.1:8080/health
```

If curl fails, the runtime isn't running or isn't bound to `0.0.0.0`. Check:

```bash
docker ps | grep softprobe-runtime
docker logs e2e-softprobe-runtime-1 | tail -50
```

In hosted mode, verify `SOFTPROBE_RUNTIME_URL` is reachable from CI runners (firewalls, VPCs).

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

CI usually has the runtime on a different hostname. Make sure you honor env vars:

```ts
const softprobe = new Softprobe({ baseUrl: process.env.SOFTPROBE_RUNTIME_URL });
```

And set the env var in CI config.

### `403 Forbidden` on outbound under strict policy

Strict policy blocks anything not mocked. Either mock the missing hop, or move that host to the allowlist:

```ts
await session.setPolicy({
  externalHttp: 'strict',
  externalAllowlist: ['internal.svc.cluster.local', 'auth.internal'],
});
```

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

Either the runtime is under memory pressure (check `docker stats`), or the proxy is syncing to the runtime on every hop without caching. Enable the proxy's inject cache:

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

### Session id missing from egress captures

Your app doesn't propagate W3C Trace Context on outbound HTTP. Solutions by language:

| Language | Fix |
|---|---|
| Node.js | Use `@opentelemetry/instrumentation-http` auto-instrumentation. |
| Python | Use `opentelemetry-instrumentation-requests` (or httpx equivalent). |
| Java | Use the OpenTelemetry Java Agent with HTTP auto-instrumentation. |
| Go | Wrap your `http.Client` with `otelhttp.NewTransport`. |

You do **not** manually forward `x-softprobe-session-id` — the proxy puts session correlation into `tracestate` on ingress, and the OTel propagator moves it through.

## Still stuck?

1. **Run `softprobe doctor --verbose`** — it checks version drift, header propagation, and produces a diagnostic bundle.
2. **Tail the runtime logs** — `docker logs -f softprobe-runtime` shows every `/v1/inject` and every 404.
3. **File an issue** with the doctor output attached at [github.com/softprobe/softprobe/issues](https://github.com/softprobe/softprobe/issues).
4. **Ask the community** at [softprobe.dev/community](https://softprobe.dev/community).

---

## Quick reference

```bash
# Health checks
softprobe doctor
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8081/health          # your SUT
curl http://127.0.0.1:8082/                # ingress listener

# Session introspection
curl http://127.0.0.1:8080/v1/sessions/$ID
curl http://127.0.0.1:8080/v1/sessions/$ID/rules
curl http://127.0.0.1:8080/v1/sessions/$ID/stats

# Logs
docker logs -f e2e-softprobe-runtime-1
docker logs -f e2e-softprobe-proxy-1

# Case file
softprobe inspect case cases/checkout.case.json
softprobe validate case cases/checkout.case.json
```
