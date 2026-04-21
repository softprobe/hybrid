# Versioning

Softprobe follows **Semantic Versioning 2.0** with one clarification: the version number describes the **protocol surface**, not just the code. This matters because the CLI, SDKs, proxy, and runtime are released together and must agree on the same wire contract.

---

## Current version

- **Platform**: `v0.5`
- **Protocol / spec**: `specVersion = 1`
- **Case schema**: `schemaVersion = 1`

You can confirm what your environment is running:

```bash
softprobe doctor --json
```

The output includes `cliVersion`, `runtimeVersion`, `specVersion`, and `schemaVersion`. See [`doctor` reference](/reference/cli#softprobe-doctor) for the exact JSON shape.

---

## Release cadence

| Kind | Frequency | Contents |
|---|---|---|
| **Minor** (`v0.N` → `v0.N+1`) | Every 4–6 weeks | New features, non-breaking refinements |
| **Patch** (`v0.N.0` → `v0.N.1`) | As needed | Bug fixes, doc updates, security fixes |
| **Major** (`v0.N` → `v1.0`) | Planned for 2026 | First stable protocol commitment, SLA on breakage |

Pre-`v1`, all minors are treated as potentially breaking — we try hard to avoid it, but we reserve the right. From `v1.0` onward, the compatibility promise below is binding.

---

## Compatibility matrix

Four surfaces have independent stability contracts. A change that breaks any of them requires a major-version bump of that surface.

| Surface | Stability contract | Detection |
|---|---|---|
| **SDK public API** (`SoftprobeClient`, `findInCase`, `mockOutbound`, …) | Semver per language package | Package manager + type-check |
| **CLI commands and flags** | Semver on the command name + documented flags | `softprobe --version` |
| **HTTP control API** | Versioned by `specVersion`; changes under a major `specVersion` are additive-only | `softprobe doctor --json` (`specVersion` field) |
| **Case file schema** | Versioned by `schemaVersion`; old files stay readable through at least one major bump | `schemaVersion` inside the case file |

The CLI refuses to talk to a runtime whose `specVersion` or `schemaVersion` major differs from its own — see [spec-drift detection](/reference/cli#spec-drift-detection).

---

## What counts as a breaking change?

For each surface, the following are **breaking** and require a major-version bump:

### SDK

- Removing or renaming a public method or class.
- Changing the signature of a public method in a way existing callers can't compile.
- Changing the type of a thrown/returned error.
- Changing whether a method is sync or async.

### CLI

- Removing or renaming a subcommand.
- Removing a documented flag, or changing its default.
- Changing the **name or type** of a field in `--json` output (see [`--json` field stability](/reference/cli#json-field-stability)). Adding new fields is **not** breaking.
- Changing a **stable** exit code mapping.

### HTTP control API (`specVersion`)

- Removing or renaming an endpoint or a required field.
- Changing the type of a request/response field.
- Tightening validation that would reject previously-accepted payloads.

Additive changes (new endpoints, new optional fields, new `then.action` values handled as no-op by older clients) are **not** breaking and may ship in a minor.

### Case file schema (`schemaVersion`)

- Removing a previously required attribute.
- Changing the meaning of an attribute.
- Tightening validation.

The runtime guarantees to read any case file whose major `schemaVersion` is the same as its own. Older major versions may be migrated with `softprobe scrub --migrate` (planned).

---

## SDK ↔ platform compatibility

SDK majors track platform majors. Concretely, **SDK `v0.5.x` is compatible with platform `v0.5.x` only**. Mixing `sdk@v0.5.0` with `softprobe-runtime@v0.6.0` is unsupported; upgrade them together.

We publish a compatibility table in each SDK's package README (`@softprobe/sdk`, `softprobe-python`, `io.softprobe:softprobe-sdk`, `github.com/softprobe/softprobe-go`).

---

## Deprecation policy

When we need to break something, we:

1. Mark the affected method, flag, or field as **deprecated** in one release. `softprobe doctor` surfaces a warning; SDKs log a one-shot warning on first use.
2. Keep the deprecated surface **working** for at least one subsequent minor release.
3. Remove it only in the next **major** bump of the affected surface.

Deprecations are listed in the [Changelog](/changelog) under each release.

---

## Further reading

- [Changelog](/changelog) — every shipped change, in order.
- [Roadmap](/roadmap) — what's coming.
- [`softprobe doctor`](/reference/cli#softprobe-doctor) — verify your install's version drift.
