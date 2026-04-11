#[path = "src/proto.rs"]
mod proto;
#[path = "src/resolver.rs"]
mod resolver;
#[path = "src/inject_ingest.rs"]
mod inject_ingest;
#[path = "src/extract_ingest.rs"]
mod extract_ingest;
#[path = "src/case_writer.rs"]
mod case_writer;

fn main() {}

#[cfg(test)]
mod tests {
    use super::resolver::{
        error_decision, mock_response_attributes, miss_decision, passthrough_decision,
        inject_result_from_mock, resolve_winner, AttributeValue, ConsumeMode, InjectResult,
        MockResponse, ReplayCase, ReplayEntry, Rule, RuleLayer,
    };
    use super::inject_ingest::parse_inject_lookup_request;
    use super::extract_ingest::{accept_extract_upload, ExtractUploadResponse};
    use super::case_writer::build_case_file;
    use super::proto::opentelemetry::proto::common::v1::{any_value, AnyValue, KeyValue};
    use super::proto::opentelemetry::proto::resource::v1::Resource;
    use super::proto::opentelemetry::proto::trace::v1::{ResourceSpans, ScopeSpans, Span, TracesData};
    use prost::Message as _;

    #[test]
    fn test_rule_resolution_prefers_session_rules_on_equal_priority() {
        let selected = resolve_winner(
            vec![Rule::new("policy-default", 10, RuleLayer::SessionPolicy, 0)],
            vec![Rule::new("case-replay", 50, RuleLayer::CaseEmbedded, 0)],
            vec![
                Rule::new("session-overrides-case", 50, RuleLayer::SessionRules, 0),
                Rule::new("session-later-entry", 50, RuleLayer::SessionRules, 1),
            ],
        )
        .expect("a rule should be selected");

        assert_eq!(selected.id, "session-later-entry");
    }

    #[test]
    fn test_rule_resolution_prefers_higher_priority_even_from_lower_layer() {
        let selected = resolve_winner(
            vec![Rule::new("policy-default", 10, RuleLayer::SessionPolicy, 0)],
            vec![Rule::new("case-high", 80, RuleLayer::CaseEmbedded, 0)],
            vec![Rule::new("session-low", 50, RuleLayer::SessionRules, 0)],
        )
        .expect("a rule should be selected");

        assert_eq!(selected.id, "case-high");
    }

    #[test]
    fn test_mock_response_emits_http_response_attributes() {
        let attributes = mock_response_attributes(&MockResponse::new(
            200,
            vec![("content-type".to_string(), "application/json".to_string())],
            Some(r#"{"ok":true}"#.to_string()),
        ));

        assert_eq!(attributes.len(), 3);
        assert_eq!(attributes[0].key, "http.response.status_code");
        assert_eq!(attributes[0].value, AttributeValue::Int(200));
        assert_eq!(attributes[1].key, "http.response.header.content-type");
        assert_eq!(
            attributes[1].value,
            AttributeValue::String("application/json".to_string())
        );
        assert_eq!(attributes[2].key, "http.response.body");
        assert_eq!(
            attributes[2].value,
            AttributeValue::String(r#"{"ok":true}"#.to_string())
        );
    }

    #[test]
    fn test_replay_consume_once_exhausts_after_first_hit() {
        let mut case = ReplayCase::new(vec![ReplayEntry::new(
            "checkout",
            ConsumeMode::Once,
            MockResponse::new(201, vec![], Some("created".to_string())),
        )]);

        let first = case
            .next_response("checkout")
            .expect("first matching response should exist");
        let second = case.next_response("checkout");

        assert_eq!(first.status_code, 201);
        assert_eq!(first.body.as_deref(), Some("created"));
        assert!(second.is_none());
    }

    #[test]
    fn test_replay_consume_many_repeats() {
        let mut case = ReplayCase::new(vec![ReplayEntry::new(
            "checkout",
            ConsumeMode::Many,
            MockResponse::new(202, vec![], Some("repeat".to_string())),
        )]);

        let first = case
            .next_response("checkout")
            .expect("first matching response should exist");
        let second = case
            .next_response("checkout")
            .expect("many consume should repeat");

        assert_eq!(first.status_code, 202);
        assert_eq!(second.status_code, 202);
        assert_eq!(first.body.as_deref(), Some("repeat"));
        assert_eq!(second.body.as_deref(), Some("repeat"));
    }

    #[test]
    fn test_passthrough_decision_is_explicit() {
        assert_eq!(passthrough_decision(), super::resolver::Decision::Passthrough);
    }

    #[test]
    fn test_strict_miss_becomes_error() {
        let decision = miss_decision(true);
        assert!(matches!(decision, super::resolver::Decision::Error { .. }));

        if let super::resolver::Decision::Error { message } = decision {
            assert_eq!(message, "strict policy requires a mock or replay match");
        } else {
            panic!("strict miss should resolve to error");
        }
    }

    #[test]
    fn test_error_decision_carries_message() {
        assert_eq!(
            error_decision("blocked by rule"),
            super::resolver::Decision::Error {
                message: "blocked by rule".to_string(),
            }
        );
    }

    #[test]
    fn test_inject_ingest_parses_session_and_http_identity() {
        let span = Span {
            trace_id: vec![1; 16],
            span_id: vec![2; 8],
            name: "inject".to_string(),
            attributes: vec![
                KeyValue {
                    key: "sp.span.type".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("inject".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.session.id".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("sess_123".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("checkout".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.traffic.direction".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("outbound".to_string())),
                    }),
                },
                KeyValue {
                    key: "url.host".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("api.stripe.com".to_string())),
                    }),
                },
                KeyValue {
                    key: "url.path".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("/v1/payment_intents".to_string())),
                    }),
                },
                KeyValue {
                    key: "http.request.header.content-type".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("application/json".to_string())),
                    }),
                },
                KeyValue {
                    key: "http.request.body".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(r#"{"amount":1000}"#.to_string())),
                    }),
                },
            ],
            ..Default::default()
        };
        let traces = TracesData {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource::default()),
                scope_spans: vec![ScopeSpans {
                    spans: vec![span],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };

        let encoded = traces.encode_to_vec();
        let decoded = TracesData::decode(encoded.as_slice()).expect("protobuf round-trip should decode");
        let parsed = parse_inject_lookup_request(&decoded).expect("inject request should parse");

        assert_eq!(parsed.session_id, "sess_123");
        assert_eq!(parsed.service_name, "checkout");
        assert_eq!(parsed.traffic_direction, "outbound");
        assert_eq!(parsed.url_host.as_deref(), Some("api.stripe.com"));
        assert_eq!(parsed.url_path.as_deref(), Some("/v1/payment_intents"));
        assert_eq!(
            parsed.request_headers,
            vec![("content-type".to_string(), "application/json".to_string())]
        );
        assert_eq!(parsed.request_body.as_deref(), Some(r#"{"amount":1000}"#));
    }

    #[test]
    fn test_inject_hit_returns_200_and_attributes() {
        let response = MockResponse::new(
            200,
            vec![("content-type".to_string(), "application/json".to_string())],
            Some(r#"{"ok":true}"#.to_string()),
        );

        let result = inject_result_from_mock(Some(&response));
        match result {
            InjectResult::Hit {
                status_code,
                attributes,
            } => {
                assert_eq!(status_code, 200);
                assert_eq!(attributes, mock_response_attributes(&response));
            }
            InjectResult::Miss => panic!("expected hit"),
        }
    }

    #[test]
    fn test_inject_miss_returns_404() {
        assert_eq!(inject_result_from_mock(None), InjectResult::Miss);
    }

    #[test]
    fn test_extract_ingest_accepts_extract_span_round_trip() {
        let span = Span {
            trace_id: vec![3; 16],
            span_id: vec![4; 8],
            name: "extract".to_string(),
            attributes: vec![
                KeyValue {
                    key: "sp.span.type".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("extract".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.session.id".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("sess_123".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("checkout".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.traffic.direction".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("outbound".to_string())),
                    }),
                },
                KeyValue {
                    key: "url.host".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("api.stripe.com".to_string())),
                    }),
                },
                KeyValue {
                    key: "url.path".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("/v1/payment_intents".to_string())),
                    }),
                },
                KeyValue {
                    key: "http.request.body".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(r#"{"amount":1000}"#.to_string())),
                    }),
                },
                KeyValue {
                    key: "http.response.header.content-type".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("application/json".to_string())),
                    }),
                },
                KeyValue {
                    key: "http.response.status_code".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::IntValue(200)),
                    }),
                },
                KeyValue {
                    key: "http.response.body".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(r#"{"ok":true}"#.to_string())),
                    }),
                },
            ],
            ..Default::default()
        };
        let traces = TracesData {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource::default()),
                scope_spans: vec![ScopeSpans {
                    spans: vec![span],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };

        let encoded = traces.encode_to_vec();
        let decoded = TracesData::decode(encoded.as_slice()).expect("protobuf round-trip should decode");
        let accepted = accept_extract_upload(&decoded);

        assert_eq!(accepted, ExtractUploadResponse::Accepted);
    }

    #[test]
    fn test_case_writer_aggregates_extract_traces_into_single_case_file() {
        let span = Span {
            trace_id: vec![3; 16],
            span_id: vec![4; 8],
            name: "extract".to_string(),
            attributes: vec![
                KeyValue {
                    key: "sp.span.type".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("extract".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.session.id".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("sess_123".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("checkout".to_string())),
                    }),
                },
                KeyValue {
                    key: "sp.traffic.direction".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("outbound".to_string())),
                    }),
                },
                KeyValue {
                    key: "url.host".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("api.stripe.com".to_string())),
                    }),
                },
                KeyValue {
                    key: "url.path".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("/v1/payment_intents".to_string())),
                    }),
                },
                KeyValue {
                    key: "http.request.body".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(r#"{"amount":1000}"#.to_string())),
                    }),
                },
                KeyValue {
                    key: "http.response.header.content-type".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue("application/json".to_string())),
                    }),
                },
                KeyValue {
                    key: "http.response.status_code".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::IntValue(200)),
                    }),
                },
                KeyValue {
                    key: "http.response.body".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(r#"{"ok":true}"#.to_string())),
                    }),
                },
            ],
            ..Default::default()
        };
        let traces = TracesData {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource::default()),
                scope_spans: vec![ScopeSpans {
                    spans: vec![span],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };

        let case = build_case_file("capture-001", &[traces]);
        let output_path = std::path::Path::new("target/p1.3a-generated.case.json");
        std::fs::write(output_path, serde_json::to_string_pretty(&case).unwrap())
            .expect("generated case file should be written");
        assert_eq!(case["version"], "1.0.0");
        assert_eq!(case["caseId"], "capture-001");
        assert_eq!(case["traces"].as_array().map(|v| v.len()), Some(1));
        assert_eq!(
            case["traces"][0]["resourceSpans"][0]["scopeSpans"][0]["spans"][0]["name"],
            "extract"
        );
        assert_eq!(
            case["traces"][0]["resourceSpans"][0]["scopeSpans"][0]["spans"][0]["attributes"][0]["key"],
            "sp.span.type"
        );
    }
}
