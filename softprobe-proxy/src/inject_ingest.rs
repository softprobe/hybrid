use crate::proto::{any_value, TracesData};

/// Parsed inject lookup request extracted from OTLP traces.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct InjectLookupRequest {
    pub session_id: String,
    pub service_name: String,
    pub traffic_direction: String,
    pub url_host: Option<String>,
    pub url_path: Option<String>,
    pub request_headers: Vec<(String, String)>,
    pub request_body: Option<String>,
}

/// Parses the first inject span in the payload into an internal request model.
pub fn parse_inject_lookup_request(traces_data: &TracesData) -> Option<InjectLookupRequest> {
    for resource_span in &traces_data.resource_spans {
        for scope_span in &resource_span.scope_spans {
            for span in &scope_span.spans {
                if !is_inject_span(span) {
                    continue;
                }

                return Some(InjectLookupRequest {
                    session_id: extract_string(span, "sp.session.id").unwrap_or_default(),
                    service_name: extract_string(span, "sp.service.name").unwrap_or_default(),
                    traffic_direction: extract_string(span, "sp.traffic.direction")
                        .unwrap_or_default(),
                    url_host: extract_string(span, "url.host"),
                    url_path: extract_string(span, "url.path"),
                    request_headers: extract_prefixed_attributes(span, "http.request.header."),
                    request_body: extract_string(span, "http.request.body"),
                });
            }
        }
    }

    None
}

fn is_inject_span(span: &crate::proto::Span) -> bool {
    matches!(
        extract_string(span, "sp.span.type").as_deref(),
        Some("inject")
    )
}

fn extract_string(span: &crate::proto::Span, key: &str) -> Option<String> {
    span.attributes.iter().find_map(|attr| {
        if attr.key != key {
            return None;
        }

        let value = attr.value.as_ref()?;
        match &value.value {
            Some(any_value::Value::StringValue(value)) => Some(value.clone()),
            Some(any_value::Value::IntValue(value)) => Some(value.to_string()),
            Some(any_value::Value::BoolValue(value)) => Some(value.to_string()),
            Some(any_value::Value::DoubleValue(value)) => Some(value.to_string()),
            Some(any_value::Value::BytesValue(value)) => {
                Some(String::from_utf8_lossy(value).to_string())
            }
            _ => None,
        }
    })
}

fn extract_prefixed_attributes(span: &crate::proto::Span, prefix: &str) -> Vec<(String, String)> {
    let mut headers = Vec::new();

    for attr in &span.attributes {
        if !attr.key.starts_with(prefix) {
            continue;
        }

        let Some(value) = attr.value.as_ref() else {
            continue;
        };

        let Some(any_value::Value::StringValue(value)) = &value.value else {
            continue;
        };

        headers.push((attr.key[prefix.len()..].to_string(), value.clone()));
    }

    headers
}
