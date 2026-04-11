package com.softprobe;

public final class SoftprobeRuntimeException extends RuntimeException {
  private final int statusCode;
  private final String body;

  public SoftprobeRuntimeException(int statusCode, String body) {
    super("softprobe runtime request failed: status " + statusCode + ": " + body.trim());
    this.statusCode = statusCode;
    this.body = body;
  }

  public int statusCode() {
    return statusCode;
  }

  public String body() {
    return body;
  }
}
