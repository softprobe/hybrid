package com.softprobe;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.net.URI;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.Test;

class ClientTest {
  @Test
  void postsSessionCreateLoadCaseAndCloseRequests() {
    List<Client.Request> calls = new ArrayList<>();
    Client.Transport transport = request -> {
      calls.add(request);
      if (request.uri().getPath().endsWith("/close")) {
        return new Client.Response(200, "{\"sessionId\":\"sess_123\",\"closed\":true}");
      }
      return new Client.Response(200, "{\"sessionId\":\"sess_123\",\"sessionRevision\":" + (calls.size() - 1) + "}");
    };

    Client client = new Client("http://runtime.test", transport);

    Map<String, Object> created = client.sessions().create("replay");
    Map<String, Object> loaded = client.sessions().loadCase(
        "sess_123",
        "{\"version\":\"1.0.0\",\"caseId\":\"checkout\",\"traces\":[]}");
    Map<String, Object> closed = client.sessions().close("sess_123");

    assertEquals("sess_123", created.get("sessionId"));
    assertEquals(0, created.get("sessionRevision"));
    assertEquals("sess_123", loaded.get("sessionId"));
    assertEquals(1, loaded.get("sessionRevision"));
    assertEquals("sess_123", closed.get("sessionId"));
    assertEquals(Boolean.TRUE, closed.get("closed"));

    assertEquals(3, calls.size());
    assertEquals("POST", calls.get(0).method());
    assertEquals(URI.create("http://runtime.test/v1/sessions"), calls.get(0).uri());
    assertEquals("{\"mode\":\"replay\"}", calls.get(0).body());

    assertEquals(URI.create("http://runtime.test/v1/sessions/sess_123/load-case"), calls.get(1).uri());
    assertEquals("{\"version\":\"1.0.0\",\"caseId\":\"checkout\",\"traces\":[]}", calls.get(1).body());

    assertEquals(URI.create("http://runtime.test/v1/sessions/sess_123/close"), calls.get(2).uri());
    assertEquals("{}", calls.get(2).body());
  }

  @Test
  void surfacesStableErrorTypeWithStatusAndBody() {
    Client.Transport transport = request -> new Client.Response(404, "{\"error\":\"unknown session\"}");
    Client client = new Client("http://runtime.test", transport);

    SoftprobeRuntimeException error =
        assertThrows(SoftprobeRuntimeException.class, () -> client.sessions().close("missing"));

    assertEquals(404, error.statusCode());
    assertEquals("{\"error\":\"unknown session\"}", error.body());
  }

  @Test
  void attachesBearerFromExplicitApiTokenOverload() {
    List<Client.Request> calls = new ArrayList<>();
    Client.Transport transport = request -> {
      calls.add(request);
      return new Client.Response(200, "{\"sessionId\":\"s\",\"sessionRevision\":0}");
    };
    Client client = new Client("http://runtime.test", transport, "sp_explicit_token");

    client.sessions().create("replay");

    assertEquals("Bearer sp_explicit_token", calls.get(0).headers().get("authorization"));
  }

  @Test
  void doesNotAttachAuthorizationHeaderWhenTokenIsNull() {
    List<Client.Request> calls = new ArrayList<>();
    Client.Transport transport = request -> {
      calls.add(request);
      return new Client.Response(200, "{\"sessionId\":\"s\",\"sessionRevision\":0}");
    };
    // Three-arg overload with null explicit token; the env var is not under
    // our control here, so we only assert that the *explicit* null path does
    // not inject a header when the env var resolves to empty/unset on CI.
    Client client = new Client("http://runtime.test", transport, null);

    client.sessions().create("replay");

    String envToken = System.getenv("SOFTPROBE_API_TOKEN");
    if (envToken == null || envToken.isBlank()) {
      if (calls.get(0).headers().containsKey("authorization")) {
        throw new AssertionError(
            "expected no Authorization header when neither explicit token nor env var is set");
      }
    }
  }

  @Test
  void explicitTokenIsTrimmedAndEmptyTokenIsNoOp() {
    List<Client.Request> calls = new ArrayList<>();
    Client.Transport transport = request -> {
      calls.add(request);
      return new Client.Response(200, "{\"sessionId\":\"s\",\"sessionRevision\":0}");
    };
    Client client = new Client("http://runtime.test", transport, "   ");

    client.sessions().create("replay");

    String envToken = System.getenv("SOFTPROBE_API_TOKEN");
    if (envToken == null || envToken.isBlank()) {
      if (calls.get(0).headers().containsKey("authorization")) {
        throw new AssertionError("whitespace-only explicit token should not send Authorization");
      }
    }
  }
}
