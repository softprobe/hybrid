package com.softprobe;

/**
 * Raised when the control runtime cannot be reached (connection refused, DNS
 * failure, timeout, ...). Distinct from {@link SoftprobeRuntimeException}, which
 * carries a real HTTP response from the runtime.
 */
public final class SoftprobeRuntimeUnreachableException extends RuntimeException {
  public SoftprobeRuntimeUnreachableException(String message) {
    super(message);
  }

  public SoftprobeRuntimeUnreachableException(String message, Throwable cause) {
    super(message, cause);
  }
}
