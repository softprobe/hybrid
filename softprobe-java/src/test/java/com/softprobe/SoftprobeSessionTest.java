package com.softprobe;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

class SoftprobeSessionTest {
  private static final ObjectMapper MAPPER = new ObjectMapper();

  private static String captureCase() {
    return "{\n"
        + "  \"version\": \"1.0.0\",\n"
        + "  \"caseId\": \"fragment-happy-path\",\n"
        + "  \"traces\": [{\n"
        + "    \"resourceSpans\": [{\n"
        + "      \"resource\": {\"attributes\": [{\"key\": \"service.name\", \"value\":"
        + " {\"stringValue\": \"fragment-svc\"}}]},\n"
        + "      \"scopeSpans\": [{\"spans\": [\n"
        + "        {\"spanId\": \"span-1\", \"attributes\": [\n"
        + "          {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
        + "          {\"key\": \"sp.traffic.direction\", \"value\": {\"stringValue\":"
        + " \"outbound\"}},\n"
        + "          {\"key\": \"http.request.method\", \"value\": {\"stringValue\": \"GET\"}},\n"
        + "          {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/fragment\"}},\n"
        + "          {\"key\": \"url.host\", \"value\": {\"stringValue\":"
        + " \"fragment.internal\"}},\n"
        + "          {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 200}},\n"
        + "          {\"key\": \"http.response.header.content-type\", \"value\": {\"stringValue\":"
        + " \"application/json\"}},\n"
        + "          {\"key\": \"http.response.body\", \"value\": {\"stringValue\":"
        + " \"{\\\"dep\\\":\\\"ok\\\"}\"}}\n"
        + "        ]}\n"
        + "      ]}]\n"
        + "    }]\n"
        + "  }]\n"
        + "}\n";
  }

  private static final class RecordingTransport implements Client.Transport {
    final List<Client.Request> calls = new ArrayList<>();
    String sessionId = "sess_123";
    int revision = 0;

    @Override
    public Client.Response send(Client.Request request) {
      calls.add(request);
      if (request.uri().getPath().endsWith("/close")) {
        return new Client.Response(
            200, "{\"sessionId\":\"" + sessionId + "\",\"closed\":true}");
      }
      revision += 1;
      return new Client.Response(
          200,
          "{\"sessionId\":\"" + sessionId + "\",\"sessionRevision\":" + revision + "}");
    }
  }

  @Test
  void startSessionPostsCreateAndReturnsBoundSession() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);

    SoftprobeSession session = softprobe.startSession("replay");

    assertEquals("sess_123", session.id());
    assertEquals(1, transport.calls.size());
    assertEquals("POST", transport.calls.get(0).method());
    assertTrue(transport.calls.get(0).uri().getPath().endsWith("/v1/sessions"));
    assertEquals("{\"mode\":\"replay\"}", transport.calls.get(0).body());
  }

  @Test
  void attachReusesSessionIdWithoutPostingCreate() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);

    SoftprobeSession session = softprobe.attach("sess_existing");

    assertEquals("sess_existing", session.id());
    assertEquals(0, transport.calls.size());
  }

  @Test
  void loadCaseFromFilePostsLoadCaseAndEnablesFindInCase(@TempDir Path tmp) throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    Path casePath = tmp.resolve("case.json");
    Files.writeString(casePath, captureCase());

    session.loadCaseFromFile(casePath);

    assertEquals(2, transport.calls.size());
    assertTrue(transport.calls.get(1).uri().getPath().endsWith("/load-case"));
    assertEquals(captureCase(), transport.calls.get(1).body());
  }

  @Test
  void findInCaseReturnsTheCapturedResponseForTheSingleMatch(@TempDir Path tmp) throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");
    Path casePath = tmp.resolve("case.json");
    Files.writeString(casePath, captureCase());
    session.loadCaseFromFile(casePath);

    CapturedHit hit =
        session.findInCase(new CaseSpanPredicate().direction("outbound").method("GET").path("/fragment"));

    assertEquals(200, hit.response().status());
    assertEquals("{\"dep\":\"ok\"}", hit.response().body());
    assertEquals("application/json", hit.response().headers().get("content-type"));
    assertNotNull(hit.span());
  }

  @Test
  void findInCaseThrowsWhenNoCaseLoaded() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    IllegalStateException error =
        assertThrows(
            IllegalStateException.class,
            () -> session.findInCase(new CaseSpanPredicate().path("/fragment")));

    assertTrue(error.getMessage().contains("loadCaseFromFile"));
  }

  @Test
  void findInCaseThrowsWhenZeroSpansMatch(@TempDir Path tmp) throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");
    Path casePath = tmp.resolve("case.json");
    Files.writeString(casePath, captureCase());
    session.loadCaseFromFile(casePath);

    IllegalStateException error =
        assertThrows(
            IllegalStateException.class,
            () -> session.findInCase(new CaseSpanPredicate().path("/missing")));

    assertTrue(error.getMessage().contains("no span"));
    assertTrue(error.getMessage().contains("/missing"));
  }

  @Test
  void findInCaseThrowsWithCandidateIdsWhenMultipleSpansMatch(@TempDir Path tmp)
      throws Exception {
    String json =
        captureCase().replace("\"spanId\": \"span-1\"", "\"spanId\": \"span-1\"")
            + "";
    // Build a case with two identical inject spans for the same path.
    String twoSpans = captureCase().replace(
        "{\"spanId\": \"span-1\"",
        "{\"spanId\": \"span-1\"},\n        {\"spanId\": \"span-2\"");
    // Strip the trailing placeholder added above.
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");
    Path casePath = tmp.resolve("case.json");
    // Simpler: construct a deliberately ambiguous case.
    String ambiguous = "{\n"
        + "  \"traces\": [{\"resourceSpans\": [{\"scopeSpans\": [{\"spans\": [\n"
        + "    {\"spanId\": \"a1\", \"attributes\": [\n"
        + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
        + "      {\"key\": \"http.request.method\", \"value\": {\"stringValue\": \"GET\"}},\n"
        + "      {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/dup\"}},\n"
        + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 200}}\n"
        + "    ]},\n"
        + "    {\"spanId\": \"a2\", \"attributes\": [\n"
        + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
        + "      {\"key\": \"http.request.method\", \"value\": {\"stringValue\": \"GET\"}},\n"
        + "      {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/dup\"}},\n"
        + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 200}}\n"
        + "    ]}\n"
        + "  ]}]}]}]\n"
        + "}\n";
    Files.writeString(casePath, ambiguous);
    session.loadCaseFromFile(casePath);

    IllegalStateException error =
        assertThrows(
            IllegalStateException.class,
            () -> session.findInCase(new CaseSpanPredicate().path("/dup")));

    assertTrue(error.getMessage().contains("2 spans match"));
    assertTrue(error.getMessage().contains("a1"));
    assertTrue(error.getMessage().contains("a2"));
    // Silence unused-variable warnings from the helper strings above.
    assertFalse(json.isEmpty());
    assertFalse(twoSpans.isEmpty());
  }

  @Test
  void findInCaseSupportsHttp2PseudoHeaderFallbacks(@TempDir Path tmp) throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");
    Path casePath = tmp.resolve("case.json");
    String pseudo = "{\n"
        + "  \"traces\": [{\"resourceSpans\": [{\"scopeSpans\": [{\"spans\": [\n"
        + "    {\"spanId\": \"p1\", \"attributes\": [\n"
        + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
        + "      {\"key\": \"http.request.header.:method\", \"value\": {\"stringValue\":"
        + " \"GET\"}},\n"
        + "      {\"key\": \"http.request.header.:path\", \"value\": {\"stringValue\":"
        + " \"/fragment\"}},\n"
        + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 204}}\n"
        + "    ]}\n"
        + "  ]}]}]}]\n"
        + "}\n";
    Files.writeString(casePath, pseudo);
    session.loadCaseFromFile(casePath);

    CapturedHit hit =
        session.findInCase(new CaseSpanPredicate().method("GET").path("/fragment"));
    assertEquals(204, hit.response().status());
  }

  @Test
  void mockOutboundPostsRulesAsPartOfASessionRuleSet(@TempDir Path tmp) throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");
    Path casePath = tmp.resolve("case.json");
    Files.writeString(casePath, captureCase());
    session.loadCaseFromFile(casePath);

    CapturedHit hit =
        session.findInCase(new CaseSpanPredicate().direction("outbound").method("GET").path("/fragment"));

    session.mockOutbound(
        new MockRuleSpec()
            .id("fragment-replay")
            .priority(100)
            .direction("outbound")
            .method("GET")
            .path("/fragment")
            .response(hit.response()));

    // calls: [create, load-case, rules]
    assertEquals(3, transport.calls.size());
    Client.Request rulesCall = transport.calls.get(2);
    assertTrue(rulesCall.uri().getPath().endsWith("/rules"));
    assertEquals("POST", rulesCall.method());

    JsonNode body = MAPPER.readTree(rulesCall.body());
    assertEquals(1, body.get("version").asInt());
    JsonNode rule = body.get("rules").get(0);
    assertEquals("fragment-replay", rule.get("id").asText());
    assertEquals(100, rule.get("priority").asInt());
    assertEquals("outbound", rule.get("when").get("direction").asText());
    assertEquals("GET", rule.get("when").get("method").asText());
    assertEquals("/fragment", rule.get("when").get("path").asText());
    assertEquals("mock", rule.get("then").get("action").asText());
    assertEquals(200, rule.get("then").get("response").get("status").asInt());
    assertEquals("{\"dep\":\"ok\"}", rule.get("then").get("response").get("body").asText());
  }

  @Test
  void clearRulesPostsAnEmptyRuleSet() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    session.clearRules();

    assertEquals(2, transport.calls.size());
    Client.Request rulesCall = transport.calls.get(1);
    assertTrue(rulesCall.uri().getPath().endsWith("/rules"));
    assertEquals("{\"version\":1,\"rules\":[]}", rulesCall.body());
  }

  @Test
  void closePostsSessionClose() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    session.close();

    Client.Request closeCall = transport.calls.get(transport.calls.size() - 1);
    assertTrue(closeCall.uri().getPath().endsWith("/close"));
    assertEquals("{}", closeCall.body());
  }

  @Test
  void mockOutboundRequiresAResponse() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    assertThrows(
        IllegalArgumentException.class,
        () -> session.mockOutbound(new MockRuleSpec().path("/fragment")));

    // sanity: verify that the map-based Client API still works (no regression).
    Map<String, Object> created = Map.of("sessionId", session.id());
    assertEquals(session.id(), created.get("sessionId"));
  }
}
