package com.softprobe;

/**
 * Raised when the control runtime returns the stable {@code unknown_session}
 * error envelope, i.e. the session id is unknown (e.g. already closed or never
 * existed). Test authors catch this type specifically to distinguish it from
 * other 4xx responses.
 */
public final class SoftprobeUnknownSessionException extends SoftprobeRuntimeException {
  public SoftprobeUnknownSessionException(int statusCode, String body) {
    super(statusCode, body);
  }
}
