# Softprobe Case OTLP JSON Profile

This document defines the minimal OTLP JSON subset used by `case.traces[]` on disk.

It is the canonical on-disk profile for case files. The proxy backend may accept the same trace schema in protobuf or JSON at request time, but case artifacts themselves remain JSON documents.

## Scope

- Applies to each element of `case.traces[]`
- Covers the OTLP JSON envelope, required HTTP identity attributes, and recommended size limits
- Targets replay, capture, diffing, and codegen workflows

## Envelope shape

Each trace document in `case.traces[]` MUST be a JSON object shaped like an OTLP `ExportTraceServiceRequest`:

- `resourceSpans[]`
- each `resourceSpans[]` item MAY contain:
  - `resource.attributes[]`
  - `scopeSpans[]`
- each `scopeSpans[]` item MUST contain:
  - `spans[]`

The profile intentionally stays at the OTLP payload level. It does not define a separate case-specific wrapper around `resourceSpans`.

## Required attributes

For Softprobe replay and inject lookup, each span SHOULD carry the following identity attributes:

- `sp.session.id`
- `sp.traffic.direction`
- `sp.span.type`
- `url.host`
- `url.path`

The following attributes SHOULD be present when the trace represents HTTP traffic:

- `sp.service.name`
- `http.request.method`
- `http.request.header.<name>`
- `http.request.body`
- `http.response.status_code`
- `http.response.header.<name>`
- `http.response.body`

The OTLP resource MAY include standard attributes such as `service.name`.

## Naming alignment

The profile uses the same attribute names as `proxy-otel-api.md` for request-path lookup and extract uploads. In particular:

- `sp.session.id` identifies the session
- `sp.traffic.direction` identifies inbound or outbound traffic
- `sp.span.type` distinguishes `inject` and `extract`
- `http.request.*` and `http.response.*` carry normalized HTTP identity and payload data

## Recommended size limits

To keep validation, diffing, and AI-generated cases predictable, v1 tooling SHOULD enforce:

- at most `100` spans per case file
- at most `128` attributes per span
- at most `64 KiB` for any string attribute value
- at most `1 MiB` for a single trace document

These limits are recommendations for the on-disk profile. Implementations MAY be stricter, but they SHOULD not silently accept larger payloads without explicit review.

## Example

```json
{
  "resourceSpans": [
    {
      "resource": {
        "attributes": [
          { "key": "service.name", "value": { "stringValue": "api" } }
        ]
      },
      "scopeSpans": [
        {
          "spans": [
            {
              "traceId": "5b8efff798038103d269b633813fc60c",
              "spanId": "051581bf3cb55c13",
              "name": "HTTP GET",
              "kind": 2,
              "attributes": [
                { "key": "sp.session.id", "value": { "stringValue": "sess_abc" } },
                { "key": "sp.traffic.direction", "value": { "stringValue": "outbound" } },
                { "key": "sp.span.type", "value": { "stringValue": "inject" } },
                { "key": "url.host", "value": { "stringValue": "api.stripe.com" } },
                { "key": "url.path", "value": { "stringValue": "/v1/payment_intents" } }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```
