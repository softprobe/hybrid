package com.softprobe.core;

import com.fasterxml.jackson.databind.JsonNode;
import com.softprobe.CapturedResponse;
import com.softprobe.CaseSpanPredicate;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * In-memory lookup against an OTLP-shaped case document.
 *
 * <p>Mirrors {@code softprobe-js/src/core/case/find-span.ts} and
 * {@code softprobe-python/softprobe/core/case_lookup.py}; see
 * {@code docs/design.md} §3.2.1 / §3.2.3 for the design.
 */
public final class CaseLookup {
  private static final String HTTP_RESPONSE_HEADER_PREFIX = "http.response.header.";

  private CaseLookup() {}

  /**
   * Returns every {@code inject}/{@code extract} span in {@code caseDocument}
   * whose attributes satisfy {@code predicate}. Order matches document order.
   */
  public static List<JsonNode> findSpans(JsonNode caseDocument, CaseSpanPredicate predicate) {
    List<JsonNode> matches = new ArrayList<>();
    if (caseDocument == null || !caseDocument.isObject()) {
      return matches;
    }
    JsonNode traces = caseDocument.path("traces");
    if (!traces.isArray()) {
      return matches;
    }
    for (JsonNode trace : traces) {
      JsonNode resourceSpans = trace.path("resourceSpans");
      if (!resourceSpans.isArray()) {
        continue;
      }
      for (JsonNode resourceSpan : resourceSpans) {
        JsonNode resourceAttrs = resourceSpan.path("resource").path("attributes");
        String serviceName = readAttributeString(resourceAttrs, "service.name");
        JsonNode scopeSpans = resourceSpan.path("scopeSpans");
        if (!scopeSpans.isArray()) {
          continue;
        }
        for (JsonNode scopeSpan : scopeSpans) {
          JsonNode spans = scopeSpan.path("spans");
          if (!spans.isArray()) {
            continue;
          }
          for (JsonNode span : spans) {
            if (spanSatisfies(span, serviceName, predicate)) {
              matches.add(span);
            }
          }
        }
      }
    }
    return matches;
  }

  /**
   * Materializes a {@link CapturedResponse} from an OTLP span's attributes.
   *
   * <p>{@code http.response.status_code} must be present; throws otherwise.
   * {@code http.response.body} defaults to empty string and headers default to
   * an empty map if absent.
   */
  public static CapturedResponse responseFromSpan(JsonNode span) {
    Map<String, String> headers = new LinkedHashMap<>();
    Integer status = null;
    String body = "";

    JsonNode attributes = span == null ? null : span.path("attributes");
    if (attributes != null && attributes.isArray()) {
      for (JsonNode attr : attributes) {
        String key = attr.path("key").asText("");
        JsonNode value = attr.path("value");
        if ("http.response.status_code".equals(key)) {
          if (value.has("intValue")) {
            JsonNode iv = value.get("intValue");
            if (iv.isIntegralNumber()) {
              status = iv.asInt();
            } else if (iv.isTextual()) {
              try {
                status = Integer.parseInt(iv.asText());
              } catch (NumberFormatException ignored) {
                // intentionally ignored — status stays null and we fail below.
              }
            }
          }
        } else if ("http.response.body".equals(key)) {
          body = anyValueToString(value);
        } else if (key.startsWith(HTTP_RESPONSE_HEADER_PREFIX)) {
          String name = key.substring(HTTP_RESPONSE_HEADER_PREFIX.length());
          headers.put(name, anyValueToString(value));
        }
      }
    }

    if (status == null) {
      String spanId = span == null ? "<unknown>" : span.path("spanId").asText("<unknown>");
      throw new IllegalStateException(
          "Captured span "
              + spanId
              + " is missing http.response.status_code; cannot materialize a captured"
              + " response.");
    }

    return new CapturedResponse(status, headers, body);
  }

  /** Compact, stable string rendering of the predicate for error messages. */
  public static String formatPredicate(CaseSpanPredicate p) {
    StringBuilder out = new StringBuilder();
    appendIfSet(out, "direction", p.directionValue());
    appendIfSet(out, "service", p.serviceValue());
    appendIfSet(out, "host", p.hostValue());
    appendIfSet(out, "hostSuffix", p.hostSuffixValue());
    appendIfSet(out, "method", p.methodValue());
    appendIfSet(out, "path", p.pathValue());
    appendIfSet(out, "pathPrefix", p.pathPrefixValue());
    if (out.length() == 0) {
      return "{}";
    }
    return "{ " + out + " }";
  }

  private static void appendIfSet(StringBuilder out, String key, String value) {
    if (value == null || value.isEmpty()) {
      return;
    }
    if (out.length() > 0) {
      out.append(", ");
    }
    out.append(key).append(": \"").append(value).append("\"");
  }

  private static boolean spanSatisfies(
      JsonNode span, String resourceServiceName, CaseSpanPredicate predicate) {
    JsonNode attrs = span.path("attributes");
    String spanType = readAttributeString(attrs, "sp.span.type");
    if (!"inject".equals(spanType) && !"extract".equals(spanType)) {
      return false;
    }

    if (predicate.directionValue() != null) {
      String direction = readAttributeString(attrs, "sp.traffic.direction");
      if (!predicate.directionValue().equals(direction)) {
        return false;
      }
    }

    if (predicate.methodValue() != null) {
      String method = readAttributeString(attrs, "http.request.method");
      if (method == null) {
        method = readAttributeString(attrs, "http.request.header.:method");
      }
      if (!predicate.methodValue().equals(method)) {
        return false;
      }
    }

    String urlPath = readAttributeString(attrs, "url.path");
    if (urlPath == null) {
      urlPath = readAttributeString(attrs, "http.request.header.:path");
    }
    if (urlPath == null) {
      urlPath = "";
    }
    if (predicate.pathValue() != null && !urlPath.equals(predicate.pathValue())) {
      return false;
    }
    if (predicate.pathPrefixValue() != null && !urlPath.startsWith(predicate.pathPrefixValue())) {
      return false;
    }

    String host = readAttributeString(attrs, "url.host");
    if (host == null) {
      host = "";
    }
    if (predicate.hostValue() != null && !host.equals(predicate.hostValue())) {
      return false;
    }
    if (predicate.hostSuffixValue() != null && !host.endsWith(predicate.hostSuffixValue())) {
      return false;
    }

    String spanService = readAttributeString(attrs, "sp.service.name");
    if (spanService == null) {
      spanService = resourceServiceName;
    }
    if (spanService == null) {
      spanService = "";
    }
    if (predicate.serviceValue() != null && !spanService.equals(predicate.serviceValue())) {
      return false;
    }

    return true;
  }

  private static String readAttributeString(JsonNode attributes, String key) {
    if (attributes == null || !attributes.isArray()) {
      return null;
    }
    for (JsonNode attr : attributes) {
      if (key.equals(attr.path("key").asText())) {
        return anyValueToString(attr.path("value"));
      }
    }
    return null;
  }

  private static String anyValueToString(JsonNode value) {
    if (value == null || value.isMissingNode() || value.isNull()) {
      return "";
    }
    if (value.isTextual()) {
      return value.asText();
    }
    if (value.isNumber() || value.isBoolean()) {
      return value.asText();
    }
    if (value.isObject()) {
      if (value.has("stringValue")) {
        return value.get("stringValue").asText("");
      }
      if (value.has("intValue")) {
        return value.get("intValue").asText("");
      }
      if (value.has("boolValue")) {
        return value.get("boolValue").asText("");
      }
      if (value.has("doubleValue")) {
        return value.get("doubleValue").asText("");
      }
    }
    return "";
  }
}
