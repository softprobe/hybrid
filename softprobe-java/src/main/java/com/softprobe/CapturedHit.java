package com.softprobe;

import com.fasterxml.jackson.databind.JsonNode;

/**
 * Result of a successful {@link SoftprobeSession#findInCase(CaseSpanPredicate)}
 * lookup: the materialized response and the raw span that produced it.
 */
public record CapturedHit(CapturedResponse response, JsonNode span) {}
