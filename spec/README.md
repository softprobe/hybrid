# Softprobe Spec

This repo is the canonical home for Softprobe's language-neutral contracts.

It exists so `softprobe-runtime`, `softprobe-proxy`, `softprobe-js`, `softprobe-python`, and `softprobe-java` can implement the same product behavior without treating one language implementation as the source of truth.

This repo owns:

- case-file schema
- rule schema
- HTTP decision protocol
- session header protocol
- OTEL mapping conventions for Softprobe replay metadata
- compatibility fixtures used by all implementations
- the OTLP JSON profile for case files in [protocol/case-otlp-json.md](./protocol/case-otlp-json.md)

Protocol files in `spec/protocol/` define **interfaces**. **`softprobe-runtime`** implements [http-control-api.md](./protocol/http-control-api.md) only. The **proxy backend** (e.g. `https://o.softprobe.ai`) implements [proxy-otel-api.md](./protocol/proxy-otel-api.md) (see [platform architecture](../docs/platform-architecture.md)).

This repo does not own:

- proxy implementation
- runtime server implementation (beyond these contracts)
- JavaScript, Python, or Java SDK implementation
- language-specific code generation templates
- framework-specific patching

## Primary design reference

- **[Hybrid platform design (engineering)](../docs/design.md)** — proxy-first HTTP, per-case JSON + OTLP traces, rules, CLI, cross-language test control, acceptance criteria.

## Layout

```text
spec/
  README.md
  schemas/
    case.schema.json
    case-trace.schema.json
    rule.schema.json
    session.schema.json
    session-*.request.schema.json
    session-*.response.schema.json
    session-error.response.schema.json
  protocol/
    http-control-api.md
    proxy-otel-api.md
    session-headers.md
  examples/
    cases/
      checkout-happy-path.case.json
      minimal.case.json
    rules/
  compatibility-tests/
    fixtures/

../docs/
  design.md
  platform-architecture.md
  repo-layout.md
  migration-plan.md
```

## Current scope

The initial content in this repo establishes the target architecture and contract boundaries. It is intentionally small. The next step is to harden these docs into versioned schemas, OpenAPI definitions, and golden compatibility fixtures.

To validate the current spec set locally, run `bash spec/scripts/validate-spec.sh`.

## License

Apache-2.0. See [`LICENSE`](./LICENSE) and the monorepo [`LICENSING.md`](../LICENSING.md) for the full dual-license map (server components are under the Softprobe Source License 1.0).
