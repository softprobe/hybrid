# Node language-plane port (Phase PD6.5)

**Status:** canonical implementation plan for the **clean cutover** from NDJSON/YAML-first boot to **runtime + case JSON**.  
**Audience:** `softprobe-js` maintainers and anyone wiring the TypeScript SDK to `softprobe-runtime`.  
**Hybrid context:** [docs/language-instrumentation.md](../docs/language-instrumentation.md), [design-proxy-first.md](./design-proxy-first.md).

---

## 1. Goals

1. **Control plane:** Session lifecycle, rules, policy, and auth fixtures go only through **`softprobe-runtime`** JSON endpoints (see [http-control-api.md](../spec/protocol/http-control-api.md) in-repo).
2. **Artifacts:** On-disk capture/replay for the language plane uses **`{traceId}.case.json`** under a data directory (JSON document matching [case.schema.json](../spec/schemas/case.schema.json)), not `{traceId}.ndjson`.
3. **No backward compatibility:** NDJSON-first sinks, `SOFTPROBE_CONFIG_PATH` / `.softprobe/config.yml` as the **authoritative** boot path, and implicit per-framework `require` patching from default `softprobe/init` are **removed** from the default product surface. Teams that still need auto-patched Express/Fastify during migration import **`@softprobe/softprobe-js/legacy`** explicitly.

---

## 2. Target control-plane calls

The TypeScript **`Softprobe` / `SoftprobeRuntimeClient`** already map to:

| Operation | HTTP |
|-----------|------|
| Create session | `POST /v1/sessions` |
| Load case | `POST /v1/sessions/{id}/load-case` |
| Rules | `POST /v1/sessions/{id}/rules` |
| Policy | `POST /v1/sessions/{id}/policy` |
| Auth fixtures | `POST /v1/sessions/{id}/fixtures/auth` |
| Close | `POST /v1/sessions/{id}/close` |

The Node **language plane** (in-process capture/replay of HTTP/Redis/Postgres, etc.) does **not** replace those calls; it complements them for dependency traffic. Application code should prefer **`SoftprobeSession`** for session + rules + `loadCase` / `findInCase` / `mockOutbound` / `clearRules` / `close`.

---

## 3. Case JSON as the on-disk artifact

- **File layout:** `{dataDirectory}/{traceId}.case.json` (same layout as the old NDJSON layout, different extension and format).
- **Document shape:** `version`, `caseId`, `mode`, `createdAt`, `traces[]` (OTLP JSON resourceSpans), optional `rules` / `fixtures`.
- **Bridge:** In-process capture still produces **`SoftprobeCassetteRecord` (v4.1)** internally for matchers and interceptors. The **`CaseJsonFileCassette`** adapter converts between buffered v4.1 records and a single case file on **read** / **after each write** so replay tests and servers keep using `SoftprobeContext.run({ cassetteDirectory, traceId })` with only the storage format changing.

---

## 4. Explicit no–backward-compatibility policy

The following are **not** supported on the default path after PD6.5:

| Removed / unsupported | Replacement |
|----------------------|-------------|
| `{dir}/{traceId}.ndjson` as the product capture file | `{dir}/{traceId}.case.json` |
| `SOFTPROBE_CONFIG_PATH` / `.softprobe/config.yml` read by **`softprobe/init`** | Runtime/session API as the product path (`Softprobe`/`SoftprobeSession`); explicit `SoftprobeContext.initGlobal` in tests |
| Default **`require`**-based Express/Fastify auto patch from init | `import '@softprobe/softprobe-js/legacy'` after init (or register middleware yourself) |

NDJSON **CLI** (`softprobe diff <file>`) and **`CassetteStore`** (NDJSON line queue) may still exist for narrow tooling until migrated; the **runtime default** for `getOrCreateCassette` is case JSON only.

---

## 5. Removal criteria (legacy modules)

| Module / pattern | Remove when |
|------------------|-------------|
| **`NdjsonCassette`** as default storage | Done when `context.ts` only constructs `CaseJsonFileCassette` for directory+traceId (PD6.5c/d). |
| **Config/env-driven init** (`ConfigManager` or mode/data-dir env in default init) | Done when default init stays PASSTHROUGH-only and runtime/session controls behavior. |
| **`applyFrameworkMutators` from default init** | Done when moved to **`legacy`** entry and examples/e2e import it (PD6.5f). |
| **`CassetteStore` NDJSON** | Optional follow-up: rewrite to append OTLP spans or deprecate in favor of `CaseJsonFileCassette` only. |

---

## 6. Verification (from tasks.md)

- **PD6.5b:** `Softprobe` / `SoftprobeSession` tests assert `mockOutbound`, `clearRules`, `close`, `loadCase` hit the mocked `fetch` URLs under `/v1/sessions/...`.
- **PD6.5c–d:** Unit tests round-trip v4.1 records ↔ case JSON; replay integration uses **only** `.case.json` + `cassetteDirectory` + `traceId`.
- **PD6.5e:** README and docs-site TS guides show runtime/session-first usage + optional `legacy` import; no primary YAML/env boot mode flow.
- **PD6.5f:** Default init does not call `applyFrameworkMutators`; legacy is opt-in.

---

## 7. Related links

- [language-instrumentation.md](../docs/language-instrumentation.md) §4–6  
- [http-control-api.md](../spec/protocol/http-control-api.md)  
- [case.schema.json](../spec/schemas/case.schema.json)  
- [tasks.md](../tasks.md) Phase PD6.5
