package com.softprobe;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

/**
 * Covers the P4.6c parity surface for softprobe-java:
 * <ul>
 *   <li>{@code loadCase(String)} accepts already-prepared JSON documents.</li>
 *   <li>{@code findAllInCase(predicate)} returns every match without throwing.</li>
 *   <li>{@code setPolicy(String)} and {@code setAuthFixtures(String)} push to
 *       the documented control endpoints.</li>
 *   <li>Typed exceptions distinguish runtime-unreachable, unknown-session,
 *       case-load, and case-lookup ambiguity failures.</li>
 * </ul>
 */
class ParitySurfaceTest {
  private static final ObjectMapper MAPPER = new ObjectMapper();

  private static String minimalCase() {
    return "{\n"
        + "  \"version\": \"1.0.0\",\n"
        + "  \"caseId\": \"fragment\",\n"
        + "  \"traces\": [{\"resourceSpans\": [{\"scopeSpans\": [{\"spans\": [\n"
        + "    {\"spanId\": \"s1\", \"attributes\": [\n"
        + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
        + "      {\"key\": \"sp.traffic.direction\", \"value\": {\"stringValue\":"
        + " \"outbound\"}},\n"
        + "      {\"key\": \"http.request.method\", \"value\": {\"stringValue\": \"GET\"}},\n"
        + "      {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/fragment\"}},\n"
        + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 200}},\n"
        + "      {\"key\": \"http.response.body\", \"value\": {\"stringValue\":"
        + " \"{\\\"dep\\\":\\\"ok\\\"}\"}}\n"
        + "    ]}\n"
        + "  ]}]}]}]\n"
        + "}\n";
  }

  private static final class RecordingTransport implements Client.Transport {
    final List<Client.Request> calls = new ArrayList<>();

    @Override
    public Client.Response send(Client.Request request) {
      calls.add(request);
      if (request.uri().getPath().endsWith("/close")) {
        return new Client.Response(200, "{\"sessionId\":\"sess_j\",\"closed\":true}");
      }
      return new Client.Response(200, "{\"sessionId\":\"sess_j\",\"sessionRevision\":1}");
    }
  }

  @Test
  void loadCaseAcceptsAJsonDocumentAndEnablesFindInCase() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    session.loadCase(minimalCase());

    assertEquals(2, transport.calls.size());
    assertTrue(transport.calls.get(1).uri().getPath().endsWith("/load-case"));
    assertEquals(minimalCase(), transport.calls.get(1).body());

    CapturedHit hit =
        session.findInCase(new CaseSpanPredicate().direction("outbound").path("/fragment"));
    assertEquals(200, hit.response().status());
  }

  @Test
  void findAllInCaseReturnsEveryMatch() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");
    String doubled =
        "{\n"
            + "  \"traces\": [{\"resourceSpans\": [{\"scopeSpans\": [{\"spans\": [\n"
            + "    {\"spanId\": \"a1\", \"attributes\": [\n"
            + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
            + "      {\"key\": \"sp.traffic.direction\", \"value\": {\"stringValue\":"
            + " \"outbound\"}},\n"
            + "      {\"key\": \"http.request.method\", \"value\": {\"stringValue\":"
            + " \"GET\"}},\n"
            + "      {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/dup\"}},\n"
            + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 200}}\n"
            + "    ]},\n"
            + "    {\"spanId\": \"a2\", \"attributes\": [\n"
            + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"extract\"}},\n"
            + "      {\"key\": \"sp.traffic.direction\", \"value\": {\"stringValue\":"
            + " \"outbound\"}},\n"
            + "      {\"key\": \"http.request.method\", \"value\": {\"stringValue\":"
            + " \"GET\"}},\n"
            + "      {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/dup\"}},\n"
            + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 201}}\n"
            + "    ]}\n"
            + "  ]}]}]}]\n"
            + "}\n";
    session.loadCase(doubled);

    List<CapturedHit> hits =
        session.findAllInCase(new CaseSpanPredicate().direction("outbound").path("/dup"));

    assertEquals(2, hits.size());
  }

  @Test
  void setPolicyPostsTheDocumentToTheRuntime() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    session.setPolicy("{\"externalHttp\":\"strict\"}");

    Client.Request last = transport.calls.get(transport.calls.size() - 1);
    assertTrue(last.uri().getPath().endsWith("/policy"));
    assertEquals("{\"externalHttp\":\"strict\"}", last.body());
  }

  @Test
  void setAuthFixturesPostsTheDocumentToTheRuntime() {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    session.setAuthFixtures("{\"tokens\":[\"t1\"]}");

    Client.Request last = transport.calls.get(transport.calls.size() - 1);
    assertTrue(last.uri().getPath().endsWith("/fixtures/auth"));
    assertEquals("{\"tokens\":[\"t1\"]}", last.body());
  }

  @Test
  void runtimeUnreachableRaisesTypedException() {
    Client.Transport transport = request -> {
      throw new IOException("connect ECONNREFUSED");
    };
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);

    SoftprobeRuntimeUnreachableException error =
        assertThrows(
            SoftprobeRuntimeUnreachableException.class,
            () -> softprobe.startSession("replay"));

    assertTrue(error.getMessage().contains("connect ECONNREFUSED"));
  }

  @Test
  void unknownSessionRaisesTypedException() {
    Client.Transport transport =
        request -> {
          if (request.uri().getPath().endsWith("/v1/sessions")) {
            return new Client.Response(200, "{\"sessionId\":\"sess_missing\",\"sessionRevision\":0}");
          }
          return new Client.Response(
              404,
              "{\"error\":{\"code\":\"unknown_session\",\"message\":\"unknown session\"}}");
        };
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    SoftprobeUnknownSessionException error =
        assertThrows(SoftprobeUnknownSessionException.class, session::close);

    assertEquals(404, error.statusCode());
    assertTrue(error.body().contains("unknown_session"));
  }

  @Test
  void caseLoadExceptionWrapsFileAndParseFailures(@TempDir Path tmp) throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");

    // Missing file.
    assertThrows(
        SoftprobeCaseLoadException.class,
        () -> session.loadCaseFromFile(tmp.resolve("missing.json")));

    // Invalid JSON.
    Path bad = tmp.resolve("bad.json");
    Files.writeString(bad, "{\"version\":");
    assertThrows(
        SoftprobeCaseLoadException.class, () -> session.loadCaseFromFile(bad));
  }

  @Test
  void caseLookupAmbiguityIsATypedException() throws Exception {
    RecordingTransport transport = new RecordingTransport();
    Softprobe softprobe = new Softprobe("http://runtime.test", transport);
    SoftprobeSession session = softprobe.startSession("replay");
    String ambiguous =
        "{\n"
            + "  \"traces\": [{\"resourceSpans\": [{\"scopeSpans\": [{\"spans\": [\n"
            + "    {\"spanId\": \"a1\", \"attributes\": [\n"
            + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
            + "      {\"key\": \"http.request.method\", \"value\": {\"stringValue\":"
            + " \"GET\"}},\n"
            + "      {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/dup\"}},\n"
            + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 200}}\n"
            + "    ]},\n"
            + "    {\"spanId\": \"a2\", \"attributes\": [\n"
            + "      {\"key\": \"sp.span.type\", \"value\": {\"stringValue\": \"inject\"}},\n"
            + "      {\"key\": \"http.request.method\", \"value\": {\"stringValue\":"
            + " \"GET\"}},\n"
            + "      {\"key\": \"url.path\", \"value\": {\"stringValue\": \"/dup\"}},\n"
            + "      {\"key\": \"http.response.status_code\", \"value\": {\"intValue\": 200}}\n"
            + "    ]}\n"
            + "  ]}]}]}]\n"
            + "}\n";
    session.loadCase(ambiguous);

    SoftprobeCaseLookupAmbiguityException error =
        assertThrows(
            SoftprobeCaseLookupAmbiguityException.class,
            () -> session.findInCase(new CaseSpanPredicate().path("/dup")));
    assertTrue(error.getMessage().contains("a1"));
    assertTrue(error.getMessage().contains("a2"));

    // Silence unused warning on JsonNode import to keep the file self-contained.
    JsonNode ignored = MAPPER.readTree(ambiguous);
    assertTrue(ignored.has("traces"));
  }
}
