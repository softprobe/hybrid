package com.softprobe;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public final class Client {
  public record Request(String method, URI uri, Map<String, String> headers, String body) {}

  public record Response(int statusCode, String body) {}

  @FunctionalInterface
  public interface Transport {
    Response send(Request request) throws IOException;
  }

  public final class Sessions {
    public Map<String, Object> create(String mode) {
      return postJson("/v1/sessions", "{\"mode\":\"" + escapeJson(mode) + "\"}");
    }

    public Map<String, Object> loadCase(String sessionId, String caseJson) {
      return postJson("/v1/sessions/" + sessionId + "/load-case", caseJson);
    }

    public Map<String, Object> updateRules(String sessionId, String rulesJson) {
      return postJson("/v1/sessions/" + sessionId + "/rules", rulesJson);
    }

    public Map<String, Object> setPolicy(String sessionId, String policyJson) {
      return postJson("/v1/sessions/" + sessionId + "/policy", policyJson);
    }

    public Map<String, Object> setAuthFixtures(String sessionId, String fixturesJson) {
      return postJson("/v1/sessions/" + sessionId + "/fixtures/auth", fixturesJson);
    }

    public Map<String, Object> close(String sessionId) {
      return postJson("/v1/sessions/" + sessionId + "/close", "{}");
    }
  }

  private static final Pattern FIELD_PATTERN =
      Pattern.compile("\"([^\"]+)\"\\s*:\\s*(\"((?:\\\\.|[^\"])*)\"|true|false|-?\\d+)");

  private final URI baseUri;
  private final Transport transport;
  private final String apiToken;
  private final Sessions sessions = new Sessions();

  public Client(String baseUrl) {
    this(baseUrl, Client::sendWithHttpClient, null);
  }

  public Client(String baseUrl, Transport transport) {
    this(baseUrl, transport, null);
  }

  /** Convenience factory: default HTTP transport with an explicit bearer token. */
  public static Client withApiToken(String baseUrl, String apiToken) {
    return new Client(baseUrl, Client::sendWithHttpClient, apiToken);
  }

  /**
   * Creates a client that attaches {@code Authorization: Bearer <token>} on every
   * control-plane request when a bearer token is configured. Token resolution:
   * the explicit {@code apiToken} argument wins; otherwise we read the
   * {@code SOFTPROBE_API_TOKEN} environment variable. Blank / whitespace-only
   * tokens are treated as "no token" — matching the runtime's
   * {@code withOptionalBearerAuth} contract.
   */
  public Client(String baseUrl, Transport transport, String apiToken) {
    this.baseUri = URI.create(baseUrl.endsWith("/") ? baseUrl : baseUrl + "/");
    this.transport = transport;
    this.apiToken = apiToken;
  }

  public Sessions sessions() {
    return sessions;
  }

  private Map<String, Object> postJson(String path, String body) {
    Map<String, String> headers = new LinkedHashMap<>();
    headers.put("content-type", "application/json");
    String token = resolveBearerToken(apiToken);
    if (token != null) {
      headers.put("authorization", "Bearer " + token);
    }

    Response response;
    try {
      response =
          transport.send(
              new Request(
                  "POST",
                  baseUri.resolve(path),
                  Map.copyOf(headers),
                  body));
    } catch (IOException e) {
      throw new SoftprobeRuntimeUnreachableException(
          "softprobe runtime is unreachable: " + (e.getMessage() == null ? "" : e.getMessage()),
          e);
    }

    if (response.statusCode() < 200 || response.statusCode() >= 300) {
      throw classifyRuntimeException(response.statusCode(), response.body());
    }

    return parseFlatJsonObject(response.body());
  }

  private static String resolveBearerToken(String explicit) {
    String candidate = explicit != null ? explicit : System.getenv("SOFTPROBE_API_TOKEN");
    if (candidate == null) {
      return null;
    }
    String trimmed = candidate.trim();
    return trimmed.isEmpty() ? null : trimmed;
  }

  private static SoftprobeRuntimeException classifyRuntimeException(int status, String body) {
    // Recognize the stable `{"error":{"code":"unknown_session",...}}` envelope
    // without pulling in a JSON dependency here (the thin Client keeps its
    // regex-based parser). We only care about the single code token.
    if (body != null && body.contains("\"unknown_session\"") && body.contains("\"code\"")) {
      return new SoftprobeUnknownSessionException(status, body);
    }
    return new SoftprobeRuntimeException(status, body);
  }

  private static Response sendWithHttpClient(Request request) throws IOException {
    HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build();
    HttpRequest.Builder builder =
        HttpRequest.newBuilder(request.uri())
            .timeout(Duration.ofSeconds(5))
            .method(request.method(), HttpRequest.BodyPublishers.ofString(request.body()));
    for (Map.Entry<String, String> header : request.headers().entrySet()) {
      builder.header(header.getKey(), header.getValue());
    }
    HttpRequest httpRequest = builder.build();

    try {
      HttpResponse<String> httpResponse =
          httpClient.send(httpRequest, HttpResponse.BodyHandlers.ofString());
      return new Response(httpResponse.statusCode(), httpResponse.body());
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
      throw new IOException("request interrupted", e);
    }
  }

  private static Map<String, Object> parseFlatJsonObject(String json) {
    Map<String, Object> values = new LinkedHashMap<>();
    Matcher matcher = FIELD_PATTERN.matcher(json);
    while (matcher.find()) {
      String key = matcher.group(1);
      String rawValue = matcher.group(2);
      values.put(key, parseValue(rawValue));
    }
    return values;
  }

  private static Object parseValue(String rawValue) {
    if ("true".equals(rawValue)) {
      return Boolean.TRUE;
    }
    if ("false".equals(rawValue)) {
      return Boolean.FALSE;
    }
    if (rawValue.startsWith("\"") && rawValue.endsWith("\"")) {
      return unescapeJson(rawValue.substring(1, rawValue.length() - 1));
    }
    return Integer.valueOf(rawValue);
  }

  private static String escapeJson(String value) {
    return value.replace("\\", "\\\\").replace("\"", "\\\"");
  }

  private static String unescapeJson(String value) {
    return value.replace("\\\"", "\"").replace("\\\\", "\\");
  }
}
