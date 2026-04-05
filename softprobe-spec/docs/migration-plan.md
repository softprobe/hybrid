# Softprobe Spec Migration Plan

This document defines how to move from the current two-repo state to the target contract-first layout.

---

## 1) Current state

Current repos:

- `proxy`
- `softprobe-js`

Current issue:

- shared architecture and contracts are being described inside `softprobe-js`, which is not the right permanent home for cross-language truth

---

## 2) Immediate correction

Create `softprobe-spec` and move the shared concerns there:

- platform architecture
- repo layout
- case schema
- rule schema
- decision protocol
- session headers

---

## 3) Next migration steps

1. Move canonical shared docs into `softprobe-spec`.
2. Leave transition notes in `softprobe-js` pointing to `softprobe-spec`.
3. Add real JSON Schemas and an OpenAPI contract to `softprobe-spec`.
   Keep this limited to test/session control APIs and artifact schemas; do not replace the proxy OTEL/protobuf wire protocol with JSON.
4. Make `proxy` and `softprobe-js` validate against `softprobe-spec`.
5. Add `softprobe-python` and `softprobe-java` against the same contracts.

---

## 4) Acceptance criteria

- New shared docs are authored in `softprobe-spec`, not `softprobe-js`.
- `softprobe-js` clearly identifies itself as a language implementation repo.
- `proxy` and all language repos can consume the same versioned contracts.
