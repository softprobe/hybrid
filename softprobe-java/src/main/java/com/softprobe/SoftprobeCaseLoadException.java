package com.softprobe;

/**
 * Raised when a case document cannot be loaded. Covers file read / JSON parse
 * errors in {@link SoftprobeSession#loadCaseFromFile} as well as non-typed
 * runtime failures while pushing the case. Runtime-unreachable and
 * unknown-session failures are passed through with their typed form so callers
 * can distinguish them.
 *
 * <p>Extends {@link IllegalStateException} so existing callers that caught the
 * "no case loaded" signal via {@code IllegalStateException} keep working.
 */
public final class SoftprobeCaseLoadException extends IllegalStateException {
  public SoftprobeCaseLoadException(String message) {
    super(message);
  }

  public SoftprobeCaseLoadException(String message, Throwable cause) {
    super(message, cause);
  }
}
