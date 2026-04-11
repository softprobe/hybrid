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

    public Map<String, Object> close(String sessionId) {
      return postJson("/v1/sessions/" + sessionId + "/close", "{}");
    }
  }

  private static final Pattern FIELD_PATTERN =
      Pattern.compile("\"([^\"]+)\"\\s*:\\s*(\"((?:\\\\.|[^\"])*)\"|true|false|-?\\d+)");

  private final URI baseUri;
  private final Transport transport;
  private final Sessions sessions = new Sessions();

  public Client(String baseUrl) {
    this(baseUrl, Client::sendWithHttpClient);
  }

  public Client(String baseUrl, Transport transport) {
    this.baseUri = URI.create(baseUrl.endsWith("/") ? baseUrl : baseUrl + "/");
    this.transport = transport;
  }

  public Sessions sessions() {
    return sessions;
  }

  private Map<String, Object> postJson(String path, String body) {
    try {
      Response response =
          transport.send(
              new Request(
                  "POST",
                  baseUri.resolve(path),
                  Map.of("content-type", "application/json"),
                  body));

      if (response.statusCode() < 200 || response.statusCode() >= 300) {
        throw new SoftprobeRuntimeException(response.statusCode(), response.body());
      }

      return parseFlatJsonObject(response.body());
    } catch (IOException e) {
      throw new SoftprobeRuntimeException(0, e.getMessage() == null ? "" : e.getMessage());
    }
  }

  private static Response sendWithHttpClient(Request request) throws IOException {
    HttpClient httpClient = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(5)).build();
    HttpRequest httpRequest =
        HttpRequest.newBuilder(request.uri())
            .timeout(Duration.ofSeconds(5))
            .method(request.method(), HttpRequest.BodyPublishers.ofString(request.body()))
            .header("content-type", request.headers().get("content-type"))
            .build();

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
