//! Deterministic rule resolution for inject decisions.
//!
//! Ordering is total and stable:
//! 1. Higher `priority` wins.
//! 2. On equal priority, later layers win: session policy < case rules < session rules.
//! 3. On equal priority within a layer, later entries win.

/// Rule layer precedence for inject resolution.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub enum RuleLayer {
    /// Session policy defaults have the lowest precedence.
    SessionPolicy = 0,
    /// Rules embedded in the loaded case override policy defaults.
    CaseEmbedded = 1,
    /// Session-local rules have the highest precedence.
    SessionRules = 2,
}

/// Minimal rule metadata used for deterministic resolution tests.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Rule {
    pub id: String,
    pub priority: i64,
    pub layer: RuleLayer,
    pub order: usize,
}

impl Rule {
    /// Builds a rule candidate with explicit precedence metadata.
    pub fn new(id: impl Into<String>, priority: i64, layer: RuleLayer, order: usize) -> Self {
        Self {
            id: id.into(),
            priority,
            layer,
            order,
        }
    }
}

/// OTLP-style attribute values used by the mock-response encoder.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum AttributeValue {
    Int(i64),
    String(String),
}

/// OTLP-style key/value pair emitted for mock responses.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Attribute {
    pub key: String,
    pub value: AttributeValue,
}

/// Mock response payload used by `then.action = mock`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MockResponse {
    pub status_code: u16,
    pub headers: Vec<(String, String)>,
    pub body: Option<String>,
}

impl MockResponse {
    /// Builds a mock response payload from explicit pieces.
    pub fn new(
        status_code: u16,
        headers: Vec<(String, String)>,
        body: Option<String>,
    ) -> Self {
        Self {
            status_code,
            headers,
            body,
        }
    }
}

/// Replay consumption mode from the loaded case.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ConsumeMode {
    Once,
    Many,
}

/// Ordered replay entry from the loaded case.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ReplayEntry {
    pub match_key: String,
    pub consume: ConsumeMode,
    pub response: MockResponse,
    consumed: bool,
}

impl ReplayEntry {
    /// Builds an ordered replay entry.
    pub fn new(match_key: impl Into<String>, consume: ConsumeMode, response: MockResponse) -> Self {
        Self {
            match_key: match_key.into(),
            consume,
            response,
            consumed: false,
        }
    }
}

/// Ordered collection of replay entries from the loaded case.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ReplayCase {
    entries: Vec<ReplayEntry>,
}

impl ReplayCase {
    /// Builds a replay case with deterministic entry order.
    pub fn new(entries: Vec<ReplayEntry>) -> Self {
        Self { entries }
    }

    /// Returns the next matching response for the given key.
    pub fn next_response(&mut self, match_key: &str) -> Option<MockResponse> {
        for entry in &mut self.entries {
            if entry.match_key != match_key {
                continue;
            }

            match entry.consume {
                ConsumeMode::Many => return Some(entry.response.clone()),
                ConsumeMode::Once => {
                    if entry.consumed {
                        continue;
                    }
                    entry.consumed = true;
                    return Some(entry.response.clone());
                }
            }
        }

        None
    }
}

/// Fallback decision when no mock or replay rule matches.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Decision {
    Passthrough,
    Error { message: String },
}

/// Returns the explicit passthrough decision.
pub fn passthrough_decision() -> Decision {
    Decision::Passthrough
}

/// Returns the explicit error decision with a stable message.
pub fn error_decision(message: impl Into<String>) -> Decision {
    Decision::Error {
        message: message.into(),
    }
}

/// Returns the miss decision for strict or permissive policy.
pub fn miss_decision(strict_policy: bool) -> Decision {
    if strict_policy {
        error_decision("strict policy requires a mock or replay match")
    } else {
        passthrough_decision()
    }
}

/// Inject lookup result returned to the proxy.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum InjectResult {
    Hit {
        status_code: u16,
        attributes: Vec<Attribute>,
    },
    Miss,
}

/// Converts an optional mock response into an inject lookup result.
pub fn inject_result_from_mock(response: Option<&MockResponse>) -> InjectResult {
    match response {
        Some(response) => InjectResult::Hit {
            status_code: 200,
            attributes: mock_response_attributes(response),
        },
        None => InjectResult::Miss,
    }
}

/// Selects the winning rule across policy, case, and session layers.
///
/// The caller provides each layer separately so the layer precedence remains
/// explicit and testable.
pub fn resolve_winner(policy_rules: Vec<Rule>, case_rules: Vec<Rule>, session_rules: Vec<Rule>) -> Option<Rule> {
    policy_rules
        .into_iter()
        .chain(case_rules)
        .chain(session_rules)
        .max_by_key(|rule| (rule.priority, rule.layer, rule.order))
}

/// Converts a mock response into OTLP-style `http.response.*` attributes.
pub fn mock_response_attributes(response: &MockResponse) -> Vec<Attribute> {
    let mut attributes = vec![Attribute {
        key: "http.response.status_code".to_string(),
        value: AttributeValue::Int(response.status_code as i64),
    }];

    for (name, value) in &response.headers {
        attributes.push(Attribute {
            key: format!("http.response.header.{}", name),
            value: AttributeValue::String(value.clone()),
        });
    }

    if let Some(body) = &response.body {
        if !body.is_empty() {
            attributes.push(Attribute {
                key: "http.response.body".to_string(),
                value: AttributeValue::String(body.clone()),
            });
        }
    }

    attributes
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn higher_priority_beats_later_layer() {
        let selected = resolve_winner(
            vec![Rule::new("policy", 20, RuleLayer::SessionPolicy, 0)],
            vec![Rule::new("case", 80, RuleLayer::CaseEmbedded, 0)],
            vec![Rule::new("session", 50, RuleLayer::SessionRules, 0)],
        )
        .expect("a rule should be selected");

        assert_eq!(selected.id, "case");
    }

    #[test]
    fn later_layer_beats_equal_priority() {
        let selected = resolve_winner(
            vec![Rule::new("policy", 50, RuleLayer::SessionPolicy, 0)],
            vec![Rule::new("case", 50, RuleLayer::CaseEmbedded, 0)],
            vec![Rule::new("session", 50, RuleLayer::SessionRules, 0)],
        )
        .expect("a rule should be selected");

        assert_eq!(selected.id, "session");
    }

    #[test]
    fn later_entry_beats_earlier_entry_within_layer() {
        let selected = resolve_winner(
            vec![],
            vec![
                Rule::new("case-first", 50, RuleLayer::CaseEmbedded, 0),
                Rule::new("case-second", 50, RuleLayer::CaseEmbedded, 1),
            ],
            vec![],
        )
        .expect("a rule should be selected");

        assert_eq!(selected.id, "case-second");
    }
}
