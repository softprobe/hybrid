# Replay in JUnit

The Java flow mirrors [Replay in Jest](/guides/replay-in-jest). Same case file, same control API, same `findInCase` + `mockOutbound` pattern — just JUnit 5 and `java.net.http.HttpClient` instead of Jest and `fetch`.

## 1. Add the dependency

```xml
<dependency>
  <groupId>dev.softprobe</groupId>
  <artifactId>softprobe-java</artifactId>
  <version>0.5.0</version>
  <scope>test</scope>
</dependency>
```

JUnit 5 is assumed (`junit-jupiter-engine` etc.).

## 2. The minimum working test

```java
// src/test/java/com/example/CheckoutReplayTest.java
package com.example;

import dev.softprobe.Softprobe;
import dev.softprobe.SoftprobeSession;
import dev.softprobe.CapturedHit;
import org.junit.jupiter.api.*;

import java.net.URI;
import java.net.http.*;
import java.nio.file.Paths;

import static org.junit.jupiter.api.Assertions.*;

@TestInstance(TestInstance.Lifecycle.PER_CLASS)
class CheckoutReplayTest {

    private final Softprobe softprobe = new Softprobe(
        System.getenv().getOrDefault("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:8080")
    );
    private final String appUrl =
        System.getenv().getOrDefault("APP_URL", "http://127.0.0.1:8082");

    private SoftprobeSession session;

    @BeforeAll
    void setUp() throws Exception {
        session = softprobe.startSession("replay");
        session.loadCaseFromFile(
            Paths.get("cases", "checkout-happy-path.case.json").toString()
        );

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
                .method("POST")
                .hostSuffix("stripe.com")
                .pathPrefix("/v1/payment_intents")
                .response(hit.getResponse())
                .build()
        );
    }

    @AfterAll
    void tearDown() throws Exception {
        session.close();
    }

    @Test
    void chargesTheCapturedCard() throws Exception {
        HttpClient client = HttpClient.newHttpClient();
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(appUrl + "/checkout"))
            .header("content-type", "application/json")
            .header("x-softprobe-session-id", session.getId())
            .POST(HttpRequest.BodyPublishers.ofString("{\"amount\":1000,\"currency\":\"usd\"}"))
            .build();

        HttpResponse<String> response = client.send(request,
            HttpResponse.BodyHandlers.ofString());

        assertEquals(200, response.statusCode());
        assertTrue(response.body().contains("\"status\":\"paid\""));
    }
}
```

## 3. Run it

```bash
mvn test
```

Expected:

```
[INFO] Tests run: 1, Failures: 0, Errors: 0, Skipped: 0
[INFO] BUILD SUCCESS
```

## API parity

| JavaScript | Java |
|---|---|
| `new Softprobe({ baseUrl })` | `new Softprobe(baseUrl)` |
| `softprobe.startSession({ mode: 'replay' })` | `softprobe.startSession("replay")` |
| `session.loadCaseFromFile(path)` | `session.loadCaseFromFile(path)` |
| `session.findInCase({ direction, … })` | `session.findInCase(CaseSpanPredicate.builder()...build())` |
| `session.mockOutbound({ …, response })` | `session.mockOutbound(MockRuleSpec.builder()...build())` |
| `session.clearRules()` | `session.clearRules()` |
| `session.setPolicy({ externalHttp: 'strict' })` | `session.setPolicy(Policy.strict())` |
| `session.close()` | `session.close()` |

Builder-pattern DTOs (`CaseSpanPredicate.builder()`, `MockRuleSpec.builder()`) make the fluent Java API read similarly to the JavaScript object literals.

## Using the JUnit extension

Boilerplate (create/close session, inject into the test class) can be delegated to a JUnit extension:

```java
@ExtendWith(SoftprobeExtension.class)
class CheckoutReplayTest {
    @SoftprobeSession(mode = "replay", casePath = "cases/checkout-happy-path.case.json")
    SoftprobeSession session;

    @Test
    void chargesTheCapturedCard() throws Exception {
        var hit = session.findInCase(CaseSpanPredicate.builder()
            .direction("outbound").hostSuffix("stripe.com").build());
        session.mockOutbound(MockRuleSpec.builder()
            .hostSuffix("stripe.com").response(hit.getResponse()).build());

        // ... your HTTP call and assertions
    }
}
```

## Parallel tests

Configure JUnit 5 parallel execution in `junit-platform.properties`:

```properties
junit.jupiter.execution.parallel.enabled = true
junit.jupiter.execution.parallel.mode.default = concurrent
junit.jupiter.execution.parallel.config.fixed.parallelism = 8
```

Each test class should create its own session; the runtime handles many in parallel.

## Next

- [Java SDK reference](/reference/sdk-java) — complete API.
- [Run a suite at scale](/guides/run-a-suite-at-scale) — for hundreds of cases.
- [CI integration](/guides/ci-integration) — running `mvn test` in CI with the Softprobe stack.
