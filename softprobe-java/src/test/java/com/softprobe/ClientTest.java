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
}
