package com.softprobe;

/**
 * Raised when {@link SoftprobeSession#findInCase(CaseSpanPredicate)} matches
 * more than one span. Authors disambiguate the predicate at authoring time.
 *
 * <p>Extends {@link IllegalStateException} so existing callers that caught the
 * ambiguity via {@code IllegalStateException} keep working.
 */
public final class SoftprobeCaseLookupAmbiguityException extends IllegalStateException {
  public SoftprobeCaseLookupAmbiguityException(String message) {
    super(message);
  }
}
