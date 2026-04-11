package com.softprobe;

import static org.junit.jupiter.api.Assertions.assertEquals;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicReference;
import org.junit.jupiter.api.Test;

class JunitExampleTest {
  private static final class ServerHandle implements AutoCloseable {
    private final HttpServer server;
    private final ExecutorService executorService;

    private ServerHandle(HttpServer server, ExecutorService executorService) {
      this.server = server;
      this.executorService = executorService;
    }

    URI uri() {
      return URI.create("http://127.0.0.1:" + server.getAddress().getPort());
    }

    @Override
    public void close() {
      server.stop(0);
      executorService.shutdownNow();
    }
  }

  private static ServerHandle serve(HttpHandler handler) throws IOException {
    HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    ExecutorService executorService = Executors.newSingleThreadExecutor();
    server.createContext("/", handler);
    server.setExecutor(executorService);
    server.start();
    return new ServerHandle(server, executorService);
  }

  @Test
  void createsSessionAndForwardsHeaderToSut() throws Exception {
    AtomicReference<String> seenHeader = new AtomicReference<>("");
    try (ServerHandle runtime =
            serve(
                exchange -> {
                  if (!"POST".equals(exchange.getRequestMethod())
                      || !"/v1/sessions".equals(exchange.getRequestURI().getPath())) {
                    sendText(exchange, 404, "not found");
                    return;
                  }
                  sendJson(exchange, 200, "{\"sessionId\":\"sess_junit_001\",\"sessionRevision\":0}");
                });
        ServerHandle sut =
            serve(
                exchange -> {
                  seenHeader.set(exchange.getRequestHeaders().getFirst("x-softprobe-session-id"));
                  exchange.getResponseHeaders().add("x-seen-session-id", seenHeader.get());
                  sendText(exchange, 200, "ok");
                })) {
      Client client = new Client(runtime.uri().toString());
      String sessionId = (String) client.sessions().create("replay").get("sessionId");

      HttpClient httpClient = HttpClient.newHttpClient();
      HttpResponse<String> response =
          httpClient.send(
              HttpRequest.newBuilder(sut.uri().resolve("/checkout"))
                  .header("x-softprobe-session-id", sessionId)
                  .GET()
                  .build(),
              HttpResponse.BodyHandlers.ofString());

      assertEquals(200, response.statusCode());
      assertEquals("ok", response.body());
      assertEquals(sessionId, response.headers().firstValue("x-seen-session-id").orElseThrow());
      assertEquals(sessionId, seenHeader.get());
    }
  }

  private static void sendJson(HttpExchange exchange, int status, String body) throws IOException {
    exchange.getResponseHeaders().add("content-type", "application/json");
    sendText(exchange, status, body);
  }

  private static void sendText(HttpExchange exchange, int status, String body) throws IOException {
    byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
    exchange.sendResponseHeaders(status, bytes.length);
    try (OutputStream out = exchange.getResponseBody()) {
      out.write(bytes);
    }
  }
}
