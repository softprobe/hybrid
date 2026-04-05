# Softprobe Spec

This repo is the canonical home for Softprobe's language-neutral contracts.

It exists so `softprobe-js`, `softprobe-proxy`, `softprobe-python`, and `softprobe-java` can implement the same product behavior without treating one language implementation as the source of truth.

This repo owns:

- case-file schema
- rule schema
- HTTP decision protocol
- session header protocol
- OTEL mapping conventions for Softprobe replay metadata
- compatibility fixtures used by all implementations

This repo does not own:

- proxy implementation
- JavaScript, Python, or Java SDK implementation
- language-specific code generation templates
- framework-specific patching

## Primary design reference

- **[Hybrid platform design (engineering)](./docs/hybrid-platform-design.md)** — proxy-first HTTP, per-case JSON + OTLP traces, rules, CLI, cross-language test control, acceptance criteria.

## Layout

```text
softprobe-spec/
  README.md
  docs/
    hybrid-platform-design.md
    platform-architecture.md
    repo-layout.md
    migration-plan.md
  schemas/
    case.schema.json
    rule.schema.json
    session.schema.json
  protocol/
    http-control-api.md
    proxy-otel-api.md
    session-headers.md
  examples/
    cases/
      minimal.case.json
    rules/
  compatibility-tests/
    fixtures/
```

## Current scope

The initial content in this repo establishes the target architecture and contract boundaries. It is intentionally small. The next step is to harden these docs into versioned schemas, OpenAPI definitions, and golden compatibility fixtures.
