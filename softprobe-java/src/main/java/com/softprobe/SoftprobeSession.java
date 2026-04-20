package com.softprobe;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.softprobe.core.CaseLookup;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;

/**
 * Session-bound helper for replay authoring. Holds the parsed case in memory
 * after {@link #loadCaseFromFile(Path)} so {@link #findInCase(CaseSpanPredicate)}
 * can do pure, synchronous lookups. See {@code docs/design.md} §3.2.
 */
public final class SoftprobeSession {
  private static final ObjectMapper MAPPER = new ObjectMapper();

  private final String id;
  private final Client client;
  private final List<ObjectNode> rules = new ArrayList<>();
  private JsonNode loadedCase;

  SoftprobeSession(String id, Client client) {
    this.id = id;
    this.client = client;
  }

  public String id() {
    return id;
  }

  /**
   * Reads an OTLP-shaped case document from {@code path}, pushes it to the
   * runtime, and keeps a parsed copy in memory for {@link #findInCase}.
   */
  public void loadCaseFromFile(Path path) {
    try {
      String caseJson = Files.readString(path);
      this.loadedCase = MAPPER.readTree(caseJson);
      client.sessions().loadCase(id, caseJson);
    } catch (IOException e) {
      throw new SoftprobeRuntimeException(0, "failed to read case file: " + e.getMessage());
    }
  }

  /**
   * Pure in-memory lookup against the case most recently loaded. Returns the
   * single matching span and its materialized response. Throws if zero or more
   * than one span matches — test authors disambiguate at authoring time.
   */
  public CapturedHit findInCase(CaseSpanPredicate predicate) {
    if (loadedCase == null) {
      throw new IllegalStateException(
          "findInCase requires a case: call `session.loadCaseFromFile(path)` first.");
    }
    List<JsonNode> matches = CaseLookup.findSpans(loadedCase, predicate);
    if (matches.isEmpty()) {
      throw new IllegalStateException(
          "findInCase: no span in the loaded case matches "
              + CaseLookup.formatPredicate(predicate)
              + ". Check the predicate (direction / method / path / host) or re-capture the case.");
    }
    if (matches.size() > 1) {
      StringBuilder ids = new StringBuilder();
      for (JsonNode span : matches) {
        if (ids.length() > 0) {
          ids.append(", ");
        }
        ids.append(span.path("spanId").asText("<unknown>"));
      }
      throw new IllegalStateException(
          "findInCase: "
              + matches.size()
              + " spans match "
              + CaseLookup.formatPredicate(predicate)
              + ". Disambiguate the predicate — candidate span ids: "
              + ids);
    }
    JsonNode span = matches.get(0);
    return new CapturedHit(CaseLookup.responseFromSpan(span), span);
  }

  /** Appends a {@code mock} rule for the session and pushes the full rule-set. */
  public void mockOutbound(MockRuleSpec spec) {
    if (spec.responseValue() == null) {
      throw new IllegalArgumentException("mockOutbound requires a response");
    }
    rules.add(buildMockRule(spec));
    syncRules();
  }

  /** Clears all rules registered in this session (locally and on the runtime). */
  public void clearRules() {
    rules.clear();
    syncRules();
  }

  public void close() {
    client.sessions().close(id);
  }

  private void syncRules() {
    ObjectNode payload = MAPPER.createObjectNode();
    payload.put("version", 1);
    ArrayNode rulesArray = payload.putArray("rules");
    for (ObjectNode rule : rules) {
      rulesArray.add(rule);
    }
    try {
      client.sessions().updateRules(id, MAPPER.writeValueAsString(payload));
    } catch (com.fasterxml.jackson.core.JsonProcessingException e) {
      throw new SoftprobeRuntimeException(0, "failed to serialize rules: " + e.getMessage());
    }
  }

  private static ObjectNode buildMockRule(MockRuleSpec spec) {
    ObjectNode rule = MAPPER.createObjectNode();
    if (spec.idValue() != null) {
      rule.put("id", spec.idValue());
    }
    if (spec.priorityValue() != null) {
      rule.put("priority", spec.priorityValue());
    }

    ObjectNode when = rule.putObject("when");
    if (spec.directionValue() != null) {
      when.put("direction", spec.directionValue());
    }
    if (spec.serviceValue() != null) {
      when.put("service", spec.serviceValue());
    }
    if (spec.hostValue() != null) {
      when.put("host", spec.hostValue());
    } else if (spec.hostSuffixValue() != null) {
      when.put("host", spec.hostSuffixValue());
    }
    if (spec.methodValue() != null) {
      when.put("method", spec.methodValue());
    }
    if (spec.pathValue() != null) {
      when.put("path", spec.pathValue());
    }
    if (spec.pathPrefixValue() != null) {
      when.put("pathPrefix", spec.pathPrefixValue());
    }

    ObjectNode then = rule.putObject("then");
    then.put("action", "mock");
    ObjectNode response = then.putObject("response");
    CapturedResponse src = spec.responseValue();
    response.put("status", src.status());
    ObjectNode headers = response.putObject("headers");
    src.headers().forEach(headers::put);
    response.put("body", src.body());

    return rule;
  }
}
