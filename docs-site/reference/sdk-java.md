# Java SDK reference

The `dev.softprobe:softprobe-java` Maven artifact. Targets Java 17+.

## Install

### Maven

```xml
<dependency>
  <groupId>dev.softprobe</groupId>
  <artifactId>softprobe-java</artifactId>
  <version>0.5.0</version>
  <scope>test</scope>
</dependency>
```

### Gradle

```kotlin
testImplementation("dev.softprobe:softprobe-java:0.5.0")
```

## `Softprobe`

### `new Softprobe(String baseUrl)`

```java
Softprobe softprobe = new Softprobe(
    System.getenv().getOrDefault("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:8080")
);
```

Or with the builder for advanced options:

```java
Softprobe softprobe = Softprobe.builder()
    .baseUrl("http://127.0.0.1:8080")
    .timeout(Duration.ofSeconds(5))
    .build();
```

### `startSession(String mode) → SoftprobeSession`

```java
SoftprobeSession session = softprobe.startSession("replay");
```

### `attach(String sessionId) → SoftprobeSession`

## `SoftprobeSession`

Implements `AutoCloseable` — use in try-with-resources:

```java
try (SoftprobeSession session = softprobe.startSession("replay")) {
    session.loadCaseFromFile("cases/checkout.case.json");
    // ...
}
// session.close() runs automatically
```

| JS | Java |
|---|---|
| `session.id` | `session.getId()` |
| `session.loadCaseFromFile(path)` | `session.loadCaseFromFile(String path)` |
| `session.findInCase({...})` | `session.findInCase(CaseSpanPredicate)` |
| `session.findAllInCase({...})` | `session.findAllInCase(CaseSpanPredicate)` |
| `session.mockOutbound({...})` | `session.mockOutbound(MockRuleSpec)` |
| `session.clearRules()` | `session.clearRules()` |
| `session.setPolicy({...})` | `session.setPolicy(Policy)` |
| `session.close()` | `session.close()` |

### Predicates and specs use builder pattern

```java
CapturedHit hit = session.findInCase(
    CaseSpanPredicate.builder()
        .direction("outbound")
        .method("POST")
        .hostSuffix("stripe.com")
        .pathPrefix("/v1/payment_intents")
        .build()
);

session.mockOutbound(
    MockRuleSpec.builder()
        .direction("outbound")
        .hostSuffix("stripe.com")
        .pathPrefix("/v1/payment_intents")
        .response(hit.getResponse())
        .build()
);
```

## JUnit 5 extension

```java
@ExtendWith(SoftprobeExtension.class)
class CheckoutReplayTest {

    @SoftprobeSession(mode = "replay", casePath = "cases/checkout-happy-path.case.json")
    SoftprobeSession session;

    @Test
    void chargesTheCapturedCard() throws Exception {
        var hit = session.findInCase(
            CaseSpanPredicate.builder().direction("outbound").hostSuffix("stripe.com").build()
        );
        session.mockOutbound(
            MockRuleSpec.builder().hostSuffix("stripe.com").response(hit.getResponse()).build()
        );
        // ... HTTP call + assertions
    }
}
```

The extension handles create/close and field injection.

### `@SoftprobeSession` attributes

| Attribute | Default | Purpose |
|---|---|---|
| `mode` | `"replay"` | Session mode |
| `casePath` | — | Relative path to `.case.json` loaded at session start |
| `baseUrl` | env | Override runtime URL |
| `strict` | `false` | Shortcut for strict external HTTP policy |

## `SuiteRunner`

```java
@ExtendWith(SoftprobeSuiteExtension.class)
@SuiteSource("suites/checkout.suite.yaml")
class CheckoutSuiteTest extends SoftprobeSuite {
    // no body needed — extension discovers cases and generates tests
}
```

Hook classes registered via `@SuiteHooks({CheckoutHooks.class})`:

```java
public class CheckoutHooks {
    @MockResponseHook("checkout.unmaskCard")
    public CapturedResponse unmaskCard(MockResponseContext ctx) {
        // ...
    }

    @BodyAssertHook("checkout.assertTotalsMatchItems")
    public List<Issue> assertTotals(BodyAssertContext ctx) {
        // ...
    }
}
```

## Errors

```java
import dev.softprobe.errors.*;

try {
    CapturedHit hit = session.findInCase(...);
} catch (CaseLookupException e) {
    // e.getMatches() → List<Span>
} catch (RuntimeException e) {
    // wire-level errors
}
```

## Logging

Uses SLF4J. To enable debug:

```xml
<!-- logback-test.xml -->
<logger name="dev.softprobe" level="DEBUG"/>
```

## See also

- [Replay in JUnit](/guides/replay-in-junit) — tutorial
- [HTTP control API](/reference/http-control-api) — wire-level spec
