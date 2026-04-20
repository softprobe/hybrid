package com.softprobe;

/**
 * Predicate used by {@link SoftprobeSession#findInCase(CaseSpanPredicate)}.
 *
 * <p>All fields are optional; {@code null} means "do not constrain". Mirrors the
 * TypeScript and Python counterparts, see {@code docs/design.md} §3.2.
 */
public final class CaseSpanPredicate {
  private String direction;
  private String service;
  private String host;
  private String hostSuffix;
  private String method;
  private String path;
  private String pathPrefix;

  public CaseSpanPredicate direction(String direction) {
    this.direction = direction;
    return this;
  }

  public CaseSpanPredicate service(String service) {
    this.service = service;
    return this;
  }

  public CaseSpanPredicate host(String host) {
    this.host = host;
    return this;
  }

  public CaseSpanPredicate hostSuffix(String hostSuffix) {
    this.hostSuffix = hostSuffix;
    return this;
  }

  public CaseSpanPredicate method(String method) {
    this.method = method;
    return this;
  }

  public CaseSpanPredicate path(String path) {
    this.path = path;
    return this;
  }

  public CaseSpanPredicate pathPrefix(String pathPrefix) {
    this.pathPrefix = pathPrefix;
    return this;
  }

  public String directionValue() {
    return direction;
  }

  public String serviceValue() {
    return service;
  }

  public String hostValue() {
    return host;
  }

  public String hostSuffixValue() {
    return hostSuffix;
  }

  public String methodValue() {
    return method;
  }

  public String pathValue() {
    return path;
  }

  public String pathPrefixValue() {
    return pathPrefix;
  }
}
