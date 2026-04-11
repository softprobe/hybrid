use base64::{engine::general_purpose, Engine as _};
use serde_json::{json, Value};

use crate::proto::{any_value, AnyValue, KeyValue, ResourceSpans, ScopeSpans, Span, TracesData};

/// Builds a single Softprobe case document from captured OTLP traces.
pub fn build_case_file(case_id: &str, traces: &[TracesData]) -> Value {
    json!({
        "version": "1.0.0",
        "caseId": case_id,
        "traces": traces.iter().map(trace_to_json).collect::<Vec<_>>(),
    })
}

fn trace_to_json(trace: &TracesData) -> Value {
    json!({
        "resourceSpans": trace.resource_spans.iter().map(resource_span_to_json).collect::<Vec<_>>(),
    })
}

fn resource_span_to_json(resource_span: &ResourceSpans) -> Value {
    let resource = resource_span.resource.as_ref().map(resource_to_json);
    let scope_spans = resource_span
        .scope_spans
        .iter()
        .map(scope_span_to_json)
        .collect::<Vec<_>>();

    let mut value = json!({
        "scopeSpans": scope_spans,
    });

    if let Some(resource) = resource {
        value["resource"] = resource;
    }

    value
}

fn resource_to_json(resource: &crate::proto::Resource) -> Value {
    json!({
        "attributes": resource.attributes.iter().map(key_value_to_json).collect::<Vec<_>>(),
    })
}

fn scope_span_to_json(scope_span: &ScopeSpans) -> Value {
    json!({
        "spans": scope_span.spans.iter().map(span_to_json).collect::<Vec<_>>(),
    })
}

fn span_to_json(span: &Span) -> Value {
    let mut value = json!({
        "traceId": hex_encode(&span.trace_id),
        "spanId": hex_encode(&span.span_id),
        "name": span.name,
        "attributes": span.attributes.iter().map(key_value_to_json).collect::<Vec<_>>(),
    });

    if !span.parent_span_id.is_empty() {
        value["parentSpanId"] = json!(hex_encode(&span.parent_span_id));
    }

    if span.kind != 0 {
        value["kind"] = json!(span.kind);
    }

    if span.start_time_unix_nano != 0 {
        value["startTimeUnixNano"] = json!(span.start_time_unix_nano.to_string());
    }

    if span.end_time_unix_nano != 0 {
        value["endTimeUnixNano"] = json!(span.end_time_unix_nano.to_string());
    }

    if let Some(status) = &span.status {
        value["status"] = status_to_json(status);
    }

    value
}

fn key_value_to_json(key_value: &KeyValue) -> Value {
    let value = key_value
        .value
        .as_ref()
        .map(any_value_to_json)
        .unwrap_or(Value::Null);

    json!({
        "key": key_value.key,
        "value": value,
    })
}

fn status_to_json(status: &crate::proto::opentelemetry::proto::trace::v1::Status) -> Value {
    json!({
        "code": status.code,
        "message": status.message,
    })
}

fn any_value_to_json(value: &AnyValue) -> Value {
    match value.value.as_ref() {
        Some(any_value::Value::StringValue(v)) => json!({ "stringValue": v }),
        Some(any_value::Value::IntValue(v)) => json!({ "intValue": v }),
        Some(any_value::Value::BoolValue(v)) => json!({ "boolValue": v }),
        Some(any_value::Value::DoubleValue(v)) => json!({ "doubleValue": v }),
        Some(any_value::Value::BytesValue(v)) => {
            json!({ "bytesValue": general_purpose::STANDARD.encode(v) })
        }
        Some(any_value::Value::ArrayValue(_)) => Value::Null,
        Some(any_value::Value::KvlistValue(_)) => Value::Null,
        None => Value::Null,
    }
}

fn hex_encode(bytes: &[u8]) -> String {
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        use std::fmt::Write as _;
        let _ = write!(&mut out, "{:02x}", byte);
    }
    out
}
