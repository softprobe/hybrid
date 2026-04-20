package com.softprobe;

/**
 * Specification for a {@code mock} outbound rule (see {@code docs/design.md}
 * §3.2). All predicate fields are optional; {@link #response(CapturedResponse)}
 * is required.
 */
public final class MockRuleSpec {
  private String id;
  private Integer priority;
  private String direction;
  private String service;
  private String host;
  private String hostSuffix;
  private String method;
  private String path;
  private String pathPrefix;
  private CapturedResponse response;

  public MockRuleSpec id(String id) {
    this.id = id;
    return this;
  }

  public MockRuleSpec priority(int priority) {
    this.priority = priority;
    return this;
  }

  public MockRuleSpec direction(String direction) {
    this.direction = direction;
    return this;
  }

  public MockRuleSpec service(String service) {
    this.service = service;
    return this;
  }

  public MockRuleSpec host(String host) {
    this.host = host;
    return this;
  }

  public MockRuleSpec hostSuffix(String hostSuffix) {
    this.hostSuffix = hostSuffix;
    return this;
  }

  public MockRuleSpec method(String method) {
    this.method = method;
    return this;
  }

  public MockRuleSpec path(String path) {
    this.path = path;
    return this;
  }

  public MockRuleSpec pathPrefix(String pathPrefix) {
    this.pathPrefix = pathPrefix;
    return this;
  }

  public MockRuleSpec response(CapturedResponse response) {
    this.response = response;
    return this;
  }

  String idValue() {
    return id;
  }

  Integer priorityValue() {
    return priority;
  }

  String directionValue() {
    return direction;
  }

  String serviceValue() {
    return service;
  }

  String hostValue() {
    return host;
  }

  String hostSuffixValue() {
    return hostSuffix;
  }

  String methodValue() {
    return method;
  }

  String pathValue() {
    return path;
  }

  String pathPrefixValue() {
    return pathPrefix;
  }

  CapturedResponse responseValue() {
    return response;
  }
}
