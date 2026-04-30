package com.softprobe;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.net.URI;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;

/**
 * PD7.3c — Java SDK parity test.
 *
 * Drives the full Softprobe facade (startSession → loadCaseFromFile →
 * findInCase → mockOutbound → close) against a fake transport using the
 * checked-in golden case fragment-happy-path.case.json.
 */
class ParityDogfoodTest {

  private static final Path GOLDEN_CASE =
      Paths.get("../spec/examples/cases/fragment-happy-path.case.json").toAbsolutePath();

  private static final class RecordingTransport implements Client.Transport {
    final List<Client.Request> calls = new ArrayList<>();
    int revision = 0;

    @Override
    public Client.Response send(Client.Request request) {
      calls.add(request);
      if (request.uri().getPath().endsWith("/close")) {
        return new Client.Response(200, "{\"sessionId\":\"dogfood-session\",\"closed\":true}");
      }
      revision += 1;
      return new Client.Response(
          200,
          "{\"sessionId\":\"dogfood-session\",\"sessionRevision\":" + revision + "}");
    }
  }

  @Test
  void fullFacadeAgainstGoldenCase() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://fake-runtime.test", transport);

    SoftprobeSession session = softprobe.startSession("replay");
    assertEquals("dogfood-session", session.id());

    session.loadCaseFromFile(GOLDEN_CASE);

    CapturedHit hit =
        session.findInCase(
            new CaseSpanPredicate().direction("outbound").method("GET").path("/fragment"));
    assertNotNull(hit);
    assertNotNull(hit.response());

    session.mockOutbound(
        new MockRuleSpec()
            .name("fragment-replay")
            .direction("outbound")
            .method("GET")
            .path("/fragment")
            .response(hit.response()));

    session.close();

    // Verify call sequence: create, load-case, rules, close
    List<URI> uris = new ArrayList<>();
    for (Client.Request call : transport.calls) {
      uris.add(call.uri());
    }
    assertTrue(uris.stream().anyMatch(u -> u.getPath().endsWith("/v1/sessions")));
    assertTrue(uris.stream().anyMatch(u -> u.getPath().endsWith("/load-case")));
    assertTrue(uris.stream().anyMatch(u -> u.getPath().endsWith("/rules")));
    assertTrue(uris.stream().anyMatch(u -> u.getPath().endsWith("/close")));

    // rules call must carry the /fragment rule
    Client.Request rulesCall =
        transport.calls.stream()
            .filter(c -> c.uri().getPath().endsWith("/rules"))
            .findFirst()
            .orElseThrow();
    assertTrue(rulesCall.body().contains("/fragment"));
  }
}
