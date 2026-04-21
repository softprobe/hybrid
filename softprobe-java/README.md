# softprobe-java

Java SDK for the **Softprobe Hybrid** platform. It talks HTTP to
`softprobe-runtime` and gives test authors an ergonomic `Softprobe` /
`SoftprobeSession` pair that mirrors the TypeScript, Python, and Go SDKs.

Targets Java 17+. Uses Jackson (`jackson-databind`) for case-document parsing;
the thin `Client` layer keeps a regex parser for flat control-plane responses
so the lightweight path stays free of JSON library usage.

## Status

This package is **not yet released** to Maven Central from this monorepo. The
`pom.xml` is `0.1.0-SNAPSHOT` and there is no CI workflow that publishes it.
The `dev.softprobe:softprobe-java` coordinates on the docs site refer to a
**planned** release and are not wired to this repository today.

In-repo harnesses (for example [`e2e/junit-replay/`](../e2e/junit-replay/))
build this SDK from source.

## Build

```bash
cd softprobe-java
mvn test
```

## Minimal replay example

```java
import com.softprobe.Softprobe;
import com.softprobe.SoftprobeSession;
import com.softprobe.CaseSpanPredicate;
import com.softprobe.CapturedHit;
import com.softprobe.MockRuleSpec;
import java.nio.file.Path;

Softprobe softprobe = new Softprobe("http://127.0.0.1:8080");
SoftprobeSession session = softprobe.startSession("replay");

session.loadCaseFromFile(Path.of("spec/examples/cases/fragment-happy-path.case.json"));
CapturedHit hit =
    session.findInCase(new CaseSpanPredicate().direction("outbound").method("GET").path("/fragment"));
session.mockOutbound(
    new MockRuleSpec()
        .direction("outbound")
        .method("GET")
        .path("/fragment")
        .response(hit.response()));

// Drive the SUT through the proxy with `x-softprobe-session-id: session.id()`.
session.close();
```

## Public surface

Mirrors the TypeScript SDK:

- `Softprobe` — entry point (`startSession`, `attach`)
- `SoftprobeSession`:
  - `loadCaseFromFile(path)` / `loadCase(caseJson)`
  - `findInCase(predicate)` / `findAllInCase(predicate)`
  - `mockOutbound(spec)` / `clearRules()`
  - `setPolicy(policyJson)` / `setAuthFixtures(fixturesJson)`
  - `close()`

Typed exceptions:

- `SoftprobeRuntimeException` — non-2xx response
- `SoftprobeRuntimeUnreachableException` — transport-layer failure
- `SoftprobeUnknownSessionException` — stable `unknown_session` envelope
- `SoftprobeCaseLoadException` — file read / parse / runtime load failure
- `SoftprobeCaseLookupAmbiguityException` — more than one `findInCase` match

## Canonical CLI

The `softprobe` command lives in [`softprobe-runtime/`](../softprobe-runtime/),
not in this package. This SDK only speaks the JSON control API over HTTP.
