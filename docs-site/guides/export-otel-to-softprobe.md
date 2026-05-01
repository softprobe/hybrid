# Export OpenTelemetry data to Softprobe

This guide shows how to send OpenTelemetry (OTLP/HTTP) data into the hosted Softprobe runtime.

Use this when you want to validate connectivity, test ingestion, or stream telemetry directly from OTel tooling.

## Before you start

You need:

- a Softprobe API token
- network access to `https://runtime.softprobe.dev`
- an OTLP producer (OpenTelemetry Collector or `telemetrygen`)

Set your token:

```bash
export SOFTPROBE_API_TOKEN=...
```

Softprobe expects bearer auth on hosted endpoints:

```http
Authorization: Bearer <token>
```

## Option 1: OpenTelemetry Collector

Configure an `otlphttp` exporter that targets the Softprobe runtime and includes your auth header.

```yaml
receivers:
  otlp:
    protocols:
      http:
      grpc:

exporters:
  otlphttp/softprobe:
    endpoint: https://runtime.softprobe.dev
    headers:
      Authorization: "Bearer ${SOFTPROBE_API_TOKEN}"
    traces_endpoint: /v1/traces
    metrics_endpoint: /v1/metrics
    logs_endpoint: /v1/logs

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlphttp/softprobe]
    metrics:
      receivers: [otlp]
      exporters: [otlphttp/softprobe]
    logs:
      receivers: [otlp]
      exporters: [otlphttp/softprobe]
```

Run your collector with this config and send any OTLP telemetry to it. The collector forwards to Softprobe.

## Option 2: `telemetrygen` quick validation

Use the repo helper script to generate sample telemetry and post it directly:

```bash
./softprobe-runtime/scripts/telemetrygen_hosted.sh all
```

You can also target a custom endpoint:

```bash
OTLP_ENDPOINT=runtime.softprobe.dev:443 ./softprobe-runtime/scripts/telemetrygen_hosted.sh traces --traces 50 --workers 5
```

The script sends:

- traces to `/v1/traces`
- metrics to `/v1/metrics`
- logs to `/v1/logs`

and passes `Authorization: Bearer ...` via OTLP headers.

## Verify ingestion

After sending data, query the runtime SQL API directly:

```bash
curl -sS -X POST "https://runtime.softprobe.dev/v1/query/sql" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${SOFTPROBE_API_TOKEN}" \
  -d '{"sql":"SELECT COUNT(*) AS rows FROM union_spans"}'
```

Run other checks with the same endpoint:

```bash
curl -sS -X POST "https://runtime.softprobe.dev/v1/query/sql" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${SOFTPROBE_API_TOKEN}" \
  -d '{"sql":"SELECT message_type, COUNT(*) AS n FROM union_spans GROUP BY message_type ORDER BY n DESC"}'
```

## Notes and boundaries

- This flow is for OTLP ingestion into Softprobe runtime storage.
- Replay control (`session`, `rules`, `load-case`) still uses the JSON control API and SDK/CLI.
- Proxy request-path decisions still use the Proxy OTLP API (`/v1/inject`) as documented in [Proxy OTLP API](/reference/proxy-otel-api).

## Related docs

- [Hosted runtime](/deployment/hosted)
- [Proxy OTLP API](/reference/proxy-otel-api)
- [HTTP control API](/reference/http-control-api)
