package com.softprobe.e2e;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.softprobe.CapturedHit;
import com.softprobe.CaseSpanPredicate;
import com.softprobe.MockRuleSpec;
import com.softprobe.Softprobe;
import com.softprobe.SoftprobeSession;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Path;
import java.time.Duration;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestInstance;

/**
 * End-to-end replay check mirroring {@code e2e/jest-replay/fragment.replay.test.ts}
 * and {@code e2e/pytest-replay/test_fragment_replay.py}. Drives the compose
 * stack through the Java SDK's {@code findInCase} + {@code mockOutbound} path.
 */
@TestInstance(TestInstance.Lifecycle.PER_CLASS)
class FragmentReplayTest {
  private static final String RUNTIME_URL =
      System.getenv().getOrDefault("SOFTPROBE_RUNTIME_URL", "http://127.0.0.1:8080");
  private static final String APP_URL =
      System.getenv().getOrDefault("APP_URL", "http://127.0.0.1:8081");

  private Softprobe softprobe;
  private SoftprobeSession session;

  @BeforeAll
  void setUp() {
    softprobe = new Softprobe(RUNTIME_URL);
    session = softprobe.startSession("replay");

    Path casePath =
        Path.of(System.getProperty("user.dir"))
            .resolve("../../spec/examples/cases/fragment-happy-path.case.json")
            .normalize();
    session.loadCaseFromFile(casePath);

    CapturedHit hit =
        session.findInCase(
            new CaseSpanPredicate().direction("outbound").method("GET").path("/fragment"));

    session.mockOutbound(
        new MockRuleSpec()
            .id("fragment-replay")
            .priority(100)
            .direction("outbound")
            .method("GET")
            .path("/fragment")
            .response(hit.response()));
  }

  @AfterAll
  void tearDown() {
    if (session != null) {
      session.close();
    }
  }

  @Test
  void replaysTheFragmentDependencyThroughTheMesh() throws Exception {
    HttpClient http = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build();
    HttpRequest req =
        HttpRequest.newBuilder(URI.create(APP_URL + "/hello"))
            .timeout(Duration.ofSeconds(5))
            .header("x-softprobe-session-id", session.id())
            .GET()
            .build();

    HttpResponse<String> response = http.send(req, HttpResponse.BodyHandlers.ofString());

    assertEquals(200, response.statusCode(), response.body());
    String body = response.body();
    assertTrue(body.contains("\"message\""), body);
    assertTrue(body.contains("\"hello\""), body);
    assertTrue(body.contains("\"dep\""), body);
    assertTrue(body.contains("\"ok\""), body);
  }
}
