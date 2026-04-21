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

::: warning `findInCase` throws on ambiguity
`findInCase` throws `CaseLookupException` if **zero** or **more than one** spans match. The exception exposes `getMatches()` with the offending span list. Use `findAllInCase` when you expect multiple matches.
:::

::: info `mockOutbound` merges on the client, replaces on the wire
The runtime replaces the whole rules document on each POST. The SDK keeps a merged list so consecutive `mockOutbound` calls accumulate. Call `session.clearRules()` to reset.
:::

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

All SDK exceptions inherit from `dev.softprobe.errors.SoftprobeException` (a `RuntimeException`).

### Error catalog

| Condition | Exception | Typical cause | Recovery |
|---|---|---|---|
| **Runtime unreachable** | `RuntimeApiException` (cause: `ConnectException` / `HttpTimeoutException`) | Runtime not running, wrong URL, firewall | Start the runtime; `softprobe doctor` |
| **Unknown session** | `RuntimeApiException` with `getStatus() == 404` | Session closed, wrong id | Start a fresh session |
| **Strict miss** (proxy returns error to app) | Not an SDK exception — surfaces as `IOException` in the SUT's HTTP client | Missing `mockOutbound` | Add the rule; see [Debug strict miss](/guides/troubleshooting#_403-forbidden-on-outbound-under-strict-policy) |
| **Invalid rule payload** | `RuntimeApiException` with `getStatus() == 400` | Rule doesn't validate against [rule-schema](/reference/rule-schema) | Fix the spec |
| **`findInCase` zero matches** | `CaseLookupException` with `getMatches().isEmpty()` | Predicate too narrow | Relax predicate; re-capture |
| **`findInCase` multiple matches** | `CaseLookupException` with `getMatches().size() > 1` | Predicate too broad | Narrow predicate; use `findAllInCase` |

### Example

```java
import dev.softprobe.errors.*;

try {
    CapturedHit hit = session.findInCase(
        CaseSpanPredicate.builder().direction("outbound").hostSuffix("stripe.com").build()
    );
} catch (CaseLookupException e) {
    System.err.printf("findInCase: %d matches: %s%n",
        e.getMatches().size(),
        e.getMatches().stream().map(Span::getSpanId).toList());
    throw e;
} catch (RuntimeApiException e) {
    System.err.printf("runtime %d at %s: %s%n",
        e.getStatus(), e.getUrl(), e.getBody());
    throw e;
} catch (CaseLoadException e) {
    System.err.printf("case load failed: %s: %s%n", e.getPath(), e.getMessage());
    throw e;
}
```

### Class hierarchy

| Class | Extends | When thrown |
|---|---|---|
| `SoftprobeException` | `RuntimeException` | Base class |
| `RuntimeApiException` | `SoftprobeException` | Runtime returned non-2xx. Methods: `getStatus()`, `getBody()`, `getUrl()` |
| `CaseLookupException` | `SoftprobeException` | `findInCase` saw 0 or >1 matches. Method: `getMatches()` |
| `CaseLoadException` | `SoftprobeException` | `loadCaseFromFile` failed. Method: `getPath()` |

## Logging

Uses SLF4J. To enable debug:

```xml
<!-- logback-test.xml -->
<logger name="dev.softprobe" level="DEBUG"/>
```

## See also

- [Replay in JUnit](/guides/replay-in-junit) — tutorial
- [HTTP control API](/reference/http-control-api) — wire-level spec
