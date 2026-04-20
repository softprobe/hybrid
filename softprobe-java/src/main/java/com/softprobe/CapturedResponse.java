package com.softprobe;

import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Materialized HTTP response extracted from a captured OTLP span (see
 * {@code docs/design.md} §3.2.2). Also used as the payload for
 * {@link SoftprobeSession#mockOutbound(MockRuleSpec)}; authors are expected to
 * mutate it freely before registering it as a mock rule.
 */
public final class CapturedResponse {
  private int status;
  private Map<String, String> headers = new LinkedHashMap<>();
  private String body = "";

  public CapturedResponse() {}

  public CapturedResponse(int status, Map<String, String> headers, String body) {
    this.status = status;
    this.headers = headers == null ? new LinkedHashMap<>() : new LinkedHashMap<>(headers);
    this.body = body == null ? "" : body;
  }

  public int status() {
    return status;
  }

  public CapturedResponse status(int status) {
    this.status = status;
    return this;
  }

  public Map<String, String> headers() {
    return headers;
  }

  public CapturedResponse headers(Map<String, String> headers) {
    this.headers = headers == null ? new LinkedHashMap<>() : new LinkedHashMap<>(headers);
    return this;
  }

  public String body() {
    return body;
  }

  public CapturedResponse body(String body) {
    this.body = body == null ? "" : body;
    return this;
  }
}
