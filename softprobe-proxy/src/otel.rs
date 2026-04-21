use std::collections::HashMap;
// Note: SystemTime is not available in WASM runtime, will use proxy-wasm host functions
use prost::Message;
use proxy_wasm;
// use std::sync::atomic::{AtomicU64, Ordering};

// Include generated protobuf types
pub mod opentelemetry {
    pub mod proto {
        pub mod common {
            pub mod v1 {
                include!(concat!(env!("OUT_DIR"), "/opentelemetry.proto.common.v1.rs"));
            }
        }
        pub mod resource {
            pub mod v1 {
                include!(concat!(env!("OUT_DIR"), "/opentelemetry.proto.resource.v1.rs"));
            }
        }
        pub mod trace {
            pub mod v1 {
                include!(concat!(env!("OUT_DIR"), "/opentelemetry.proto.trace.v1.rs"));
            }
        }
    }
}

// Re-export commonly used types
pub use opentelemetry::proto::common::v1::{AnyValue, KeyValue, any_value};
pub use opentelemetry::proto::resource::v1::Resource;
pub use opentelemetry::proto::trace::v1::{TracesData, ResourceSpans, ScopeSpans, Span, Status, span};

use crate::headers::header_get_ci;

#[derive(Clone)]
pub struct SpanBuilder {
    trace_id: Vec<u8>,
    parent_span_id: Option<Vec<u8>>,
    current_span_id: Vec<u8>,  // 添加当前 span ID 字段
    service_name: String,
    traffic_direction: String,  // 添加traffic_direction字段
    public_key: String,
    session_id: String
}

impl SpanBuilder {
    pub fn new() -> Self {
        Self {
            // Leave trace_id empty until with_context parses W3C `traceparent` (OTel TraceContext).
            // A random default trace_id would skip the standard traceparent fallback and drop
            // parent_span_id for typical OTel clients.
            trace_id: Vec::new(),
            parent_span_id: None,
            current_span_id: generate_span_id(),  // 初始化当前 span ID
            service_name: "default-service".to_string(),
            traffic_direction: "outbound".to_string(),  // 默认值
            public_key: String::new(),
            session_id: String::new()
        }
    }
    // 添加设置service_name的方法
    pub fn with_service_name(mut self, service_name: String) -> Self {
        self.service_name = service_name;
        self
    }

    // 添加设置traffic_direction的方法
    pub fn with_traffic_direction(mut self, traffic_direction: String) -> Self {
        self.traffic_direction = traffic_direction;
        self
    }

    // 添加设置api_key的方法
    pub fn with_public_key(mut self, public_key: String) -> Self {
        self.public_key = public_key;
        self
    }

    /// Check if session_id is present and not empty
    pub fn has_session_id(&self) -> bool {
        !self.session_id.is_empty()
    }

    /// Get current session_id string (may be empty if not set)
    pub fn get_session_id(&self) -> &str {
        &self.session_id
    }

    /// Get trace_id as hex string
    pub fn get_current_span_id_hex(&self) -> String {
        self.current_span_id.iter().map(|b| format!("{:02x}", b)).collect::<String>()
    }

    pub fn get_trace_id_hex(&self) -> String {
        self.trace_id.iter().map(|b| format!("{:02x}", b)).collect::<String>()
    }

    pub fn with_context(mut self, headers: &HashMap<String, String>) -> Self {
        // Session id may appear in `tracestate` (merged by inject); trace ids use W3C `traceparent`.
        if let Some(tracestate) = header_get_ci(headers, "tracestate") {
            crate::sp_info!("with_context Found tracestate header {}", tracestate);
            for entry in tracestate.split(',') {
                let entry = entry.trim();
                if self.session_id.is_empty() {
                    if let Some((k, v)) = entry.split_once('=') {
                        match k.trim().to_ascii_lowercase().as_str() {
                            "x-softprobe-session-id" | "x-sp-session-id" => {
                                crate::sp_debug!("Found session id entry in tracestate");
                                self.session_id = v.trim().to_string();
                            }
                            _ => {}
                        }
                    }
                }
            }
        }

        // Standard W3C traceparent (OpenTelemetry TraceContext propagator).
        if let Some(traceparent) = header_get_ci(headers, "traceparent") {
            crate::sp_debug!("Found traceparent header {}", traceparent);
            if let Some((tid, pid)) = parse_traceparent(traceparent) {
                if self.trace_id.is_empty() {
                    self.trace_id = tid;
                    self.parent_span_id = Some(pid);
                    crate::sp_debug!("Parsed trace context from traceparent (trace + parent)");
                } else if self.parent_span_id.is_none() && tid == self.trace_id {
                    self.parent_span_id = Some(pid);
                    crate::sp_debug!("Filled parent_span_id from traceparent (matching trace id)");
                }
            }
        }

        // Look for a session id that was already set upstream. We must treat the
        // value as OPAQUE: the SDK can choose any format (e.g. `sess_<base64>`,
        // a UUID, an integer, …). The proxy never invents a session id of its
        // own — doing so would collide with the SDK's session-id namespace and
        // cause inject lookups to always miss. If no session id is present, we
        // leave it empty and skip both inject and extract dispatch downstream.
        crate::sp_debug!("Looking for session_id in headers");
        let session_id_found = header_get_ci(headers, "x-softprobe-session-id")
            .or_else(|| header_get_ci(headers, "x-sp-session-id"))
            .or_else(|| header_get_ci(headers, "sp_session_id"))
            .or_else(|| header_get_ci(headers, "x-session-id"));

        if let Some(session_id) = session_id_found {
            let masked = if session_id.len() > 4 { "****" } else { "" };
            crate::sp_debug!("Found session_id in headers: {}", masked);
            self.session_id = session_id.clone();
        } else if self.session_id.is_empty() {
            // Fall back to tracestate only when the tracestate branch above
            // didn't already populate it (it only did so if tracestate was
            // present). This keeps the reader format-agnostic.
            if let Some(tracestate) = header_get_ci(headers, "tracestate") {
                for entry in tracestate.split(',') {
                    let entry = entry.trim();
                    if let Some((k, v)) = entry.split_once('=') {
                        match k.trim().to_ascii_lowercase().as_str() {
                            "x-softprobe-session-id" | "x-sp-session-id" => {
                                crate::sp_debug!("Found session_id in tracestate: ****");
                                self.session_id = v.trim().to_string();
                                break;
                            }
                            _ => {}
                        }
                    }
                }
            }
        }
        if self.session_id.is_empty() {
            crate::sp_debug!(
                "No session_id found in headers or tracestate; leaving empty (inject/extract will be skipped)"
            );
        }

        // If no valid trace context found, generate new one
        if self.trace_id.is_empty() {
            self.trace_id = generate_trace_id();
        }

        // `generate_span_id` is time-derived and can equal the W3C traceparent span id (the
        // incoming parent for this hop). That makes span_id == parent_span_id; OTLP JSON then
        // omits parentSpanId. Regenerate until the proxy span id is distinct from the parent.
        if let Some(ref pid) = self.parent_span_id {
            if self.current_span_id == *pid {
                for _ in 0..32 {
                    self.current_span_id = generate_span_id();
                    if self.current_span_id != *pid {
                        break;
                    }
                }
            }
        }

        self
    }

    #[allow(dead_code)]
    pub fn create_inject_span(
        &self,
        request_headers: &HashMap<String, String>,
        request_body: &[u8],
        url_host: Option<&str>,
        url_path: Option<&str>,
    ) -> TracesData {
        let span_id = self.current_span_id.clone();  // 使用 SpanBuilder 中的 current_span_id
        let mut attributes = Vec::new();

        // Add service name attribute
        let service_name = if self.service_name.is_empty() {
            "default-service".to_string()
        } else {
            self.service_name.clone()
        };

        attributes.push(KeyValue {
            key: "sp.service.name".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(service_name)),
            }),
        });

        // Add traffic direction attribute
        attributes.push(KeyValue {
            key: "sp.traffic.direction".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(self.traffic_direction.clone())),
            }),
        });

        // Add API key attribute if present
        log::debug!("DEBUG: public_key value: '{}'", self.public_key);
        if !self.public_key.is_empty() {
            log::debug!("DEBUG: Adding public_key attribute");
            attributes.push(KeyValue {
                key: "sp.public.key".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(self.public_key.clone())),
                }),
            });
        } else {
            log::debug!("DEBUG: public_key is empty, not adding attribute");
        }

        // Add span type attribute
        attributes.push(KeyValue {
            key: "sp.span.type".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue("inject".to_string())),
            }),
        });

        // Add session ID attribute if present
        if !self.session_id.is_empty() {
            attributes.push(KeyValue {
                key: "sp.session.id".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(self.session_id.clone())),
                }),
            });
        }
        
        // Add request headers as attributes
        for (key, value) in request_headers {
            if !should_skip_header(key) {
                attributes.push(KeyValue {
                    key: format!("http.request.header.{}", key.to_lowercase()),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(value.clone())),
                    }),
                });
            }
        }

        // Add url attributes if available
        if let Some(path) = url_path {
            attributes.push(KeyValue {
                key: "url.path".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(path.to_string())),
                }),
            });
        }
        if let Some(host) = url_host {
            if let Some(path) = url_path {
                attributes.push(KeyValue {
                    key: "url.full".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(format!("http://{}{}", host, path))),
                    }),
                });
            }
            attributes.push(KeyValue {
                key: "url.host".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(host.to_string())),
                }),
            });
        }

        // Add request body if present and text-based
        if !request_body.is_empty() {
            let body_value = if is_text_content(request_headers) {
                String::from_utf8_lossy(request_body).to_string()
            } else {
                use base64::{Engine as _, engine::general_purpose};
                general_purpose::STANDARD.encode(request_body)
            };

            attributes.push(KeyValue {
                key: "http.request.body".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(body_value)),
                }),
            });
        }

        let span = Span {
            trace_id: self.trace_id.clone(),
            span_id,
            parent_span_id: self.parent_span_id.clone().unwrap_or_default(),
            name: url_path.unwrap_or("unknown_path").to_string(),
            kind: span::SpanKind::Client as i32,
            start_time_unix_nano: get_current_timestamp_nanos(),
            end_time_unix_nano: get_current_timestamp_nanos(),
            attributes,
            flags: 0,
            ..Default::default()
        };

        self.create_traces_data(span)
    }

    pub fn create_extract_span(
        &self,
        request_headers: &HashMap<String, String>,
        request_body: &[u8],
        response_headers: &HashMap<String, String>,
        response_body: &[u8],
        url_host: Option<&str>,
        url_path: Option<&str>,
        request_start_time: Option<u64>,  // Add request start time parameter
    ) -> TracesData {
        let mut parent_span_id = self.parent_span_id.clone().unwrap_or_default();
        if parent_span_id.is_empty() {
            if let Some(p) = w3c_parent_span_id_for_trace(&self.trace_id, request_headers) {
                parent_span_id = p;
            }
        }
        let mut span_id = self.current_span_id.clone();
        ensure_span_id_distinct_from_parent(&mut span_id, &parent_span_id);

        let mut attributes = Vec::new();

        crate::sp_debug!("Building extract span: service_name set {}", self.service_name);
        attributes.push(KeyValue {
            key: "sp.service.name".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(self.service_name.clone())),
            }),
        });

        // Add traffic direction attribute
        crate::sp_debug!("Building extract span: traffic_direction set {}", self.traffic_direction);
        attributes.push(KeyValue {
            key: "sp.traffic.direction".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(self.traffic_direction.clone())),
            }),
        });

        // Add extract span type attribute
        attributes.push(KeyValue {
            key: "sp.span.type".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue("extract".to_string())),
            }),
        });

        // Add session ID attribute if present
        if !self.session_id.is_empty() {
            crate::sp_debug!("Building extract span: session_id present: {}", self.session_id);
            attributes.push(KeyValue {
                key: "sp.session.id".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(self.session_id.clone())),
                }),
            });
        } else {
            crate::sp_debug!("session_id is empty, not adding attribute");
        }

        // Add request headers
        for (key, value) in request_headers {
            if !should_skip_header(key) {
                attributes.push(KeyValue {
                    key: format!("http.request.header.{}", key.to_lowercase()),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(value.clone())),
                    }),
                });
            }
        }

        // Add url attributes if available
        if let Some(path) = url_path {
            attributes.push(KeyValue {
                key: "url.path".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(path.to_string())),
                }),
            });
        }
        if let Some(host) = url_host {
            if let Some(path) = url_path {
                attributes.push(KeyValue {
                    key: "url.full".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(format!("http://{}{}", host, path))),
                    }),
                });
            }
            attributes.push(KeyValue {
                key: "url.host".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(host.to_string())),
                }),
            });
        }

        // Add request body
        if !request_body.is_empty() {
            let body_value = if is_text_content(request_headers) {
                String::from_utf8_lossy(request_body).to_string()
            } else {
                use base64::{Engine as _, engine::general_purpose};
                general_purpose::STANDARD.encode(request_body)
            };

            attributes.push(KeyValue {
                key: "http.request.body".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(body_value)),
                }),
            });
        }

        // Add response headers
        for (key, value) in response_headers {
            if !should_skip_header(key) {
                attributes.push(KeyValue {
                    key: format!("http.response.header.{}", key.to_lowercase()),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(value.clone())),
                    }),
                });
            }
        }

        // Add response status code
        if let Some(status) = response_headers.get(":status") {
            if let Ok(status_code) = status.parse::<i64>() {
                attributes.push(KeyValue {
                    key: "http.response.status_code".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::IntValue(status_code)),
                    }),
                });
            }
        }

        // Add response body
        if !response_body.is_empty() {
            let body_value = if is_text_content(response_headers) {
                String::from_utf8_lossy(response_body).to_string()
            } else {
                use base64::{Engine as _, engine::general_purpose};
                general_purpose::STANDARD.encode(response_body)
            };

            attributes.push(KeyValue {
                key: "http.response.body".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(body_value)),
                }),
            });
        }

        let span = Span {
            trace_id: self.trace_id.clone(),
            span_id,
            trace_state: String::new(),
            parent_span_id,
            name: url_path.unwrap_or("unknown_path").to_string(),
            kind: span::SpanKind::Server as i32,
            start_time_unix_nano: request_start_time.unwrap_or_else(|| get_current_timestamp_nanos()),
            end_time_unix_nano: get_current_timestamp_nanos(),
            attributes,
            dropped_attributes_count: 0,
            events: vec![],
            dropped_events_count: 0,
            links: vec![],
            dropped_links_count: 0,
            status: Some(Status {
                code: 1, // STATUS_CODE_OK
                message: String::new(),
            }),
            flags: 0,
        };

        self.create_traces_data(span)
    }

    fn create_traces_data(&self, span: Span) -> TracesData {
        // Create resource with service.name attribute
        let service_name = if self.service_name.is_empty() {
            "default-service".to_string()
        } else {
            self.service_name.clone()
        };
        let mut attributes = Vec::new();

        log::debug!("DEBUG: public_key value: '{}'", self.public_key);
        if !self.public_key.is_empty() {
            log::debug!("DEBUG: Adding public_key attribute");
            attributes.push(KeyValue {
                key: "sp.public.key".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue(self.public_key.clone())),
                }),
            });
        } else {
            log::debug!("DEBUG: public_key is empty, not adding attribute");
        }

        attributes.push(KeyValue {
            key: "service.name".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(service_name)),
            }),
        });

        let resource_type_value = "sp-envoy-proxy".to_string();
        attributes.push(KeyValue {
            key: "sp.resource.type".to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(resource_type_value.clone())),
            }),
        });

        let resource = Resource {
            attributes,
            dropped_attributes_count: 0,
            entity_refs: vec![],
        };

        TracesData {
            resource_spans: vec![ResourceSpans {
                resource: Some(resource),
                scope_spans: vec![ScopeSpans {
                    spans: vec![span],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    /// Generate W3C traceparent header value
    /// Format: 00-{trace_id}-{span_id}-{trace_flags}
    pub fn generate_traceparent(&self, span_id: &[u8]) -> String {
        let version = "00";
        let trace_id_hex = hex_encode(&self.trace_id);
        let span_id_hex = hex_encode(span_id);
        let trace_flags = "01"; // sampled flag set

        format!("{}-{}-{}-{}", version, trace_id_hex, span_id_hex, trace_flags)
    }

    }


// 保留原有的protobuf序列化函数
pub fn serialize_traces_data(traces_data: &TracesData) -> Result<Vec<u8>, prost::EncodeError> {
    let mut buf = Vec::new();
    traces_data.encode(&mut buf)?;
    Ok(buf)
}

fn generate_trace_id() -> Vec<u8> {
    let mut trace_id = vec![0u8; 16];
    
    // Use current timestamp as source of randomness
    let now_nanos = get_current_timestamp_nanos();
    let secs = (now_nanos / 1_000_000_000) as u64;
    let nanos = (now_nanos % 1_000_000_000) as u64;
    
    // Fill first 8 bytes with seconds
    trace_id[0..8].copy_from_slice(&secs.to_be_bytes());
    // Fill last 8 bytes with nanoseconds
    trace_id[8..16].copy_from_slice(&nanos.to_be_bytes());
    
    trace_id
}

pub fn generate_span_id() -> Vec<u8> {
    let mut span_id = vec![0u8; 8];
    
    // Use current timestamp as source of randomness
    let now_nanos = get_current_timestamp_nanos();
    
    // Add some variation to make it different from trace ID
    let varied_nanos = now_nanos ^ 0xCAFEBABE;
    span_id.copy_from_slice(&varied_nanos.to_be_bytes());
    
    span_id
}

fn parse_traceparent(traceparent: &str) -> Option<(Vec<u8>, Vec<u8>)> {
    let parts: Vec<&str> = traceparent.split('-').collect();
    if parts.len() != 4 {
        return None;
    }
    
    let trace_id = hex_decode(parts[1])?;
    let span_id = hex_decode(parts[2])?;
    
    Some((trace_id, span_id))
}

/// Resolves the W3C parent span id for `trace_id` from the `traceparent` header.
/// Used as a fallback when `with_context` did not populate `parent_span_id` but headers still carry context.
fn w3c_parent_span_id_for_trace(trace_id: &[u8], headers: &HashMap<String, String>) -> Option<Vec<u8>> {
    let traceparent = header_get_ci(headers, "traceparent")?;
    let (tid, pid) = parse_traceparent(traceparent)?;
    if tid.as_slice() == trace_id {
        Some(pid)
    } else {
        None
    }
}

fn ensure_span_id_distinct_from_parent(span_id: &mut Vec<u8>, parent_id: &[u8]) {
    if parent_id.is_empty() || span_id.as_slice() != parent_id {
        return;
    }
    for _ in 0..32 {
        *span_id = generate_span_id();
        if span_id.as_slice() != parent_id {
            break;
        }
    }
}

fn hex_decode(hex: &str) -> Option<Vec<u8>> {
    if hex.len() % 2 != 0 {
        return None;
    }
    
    let mut result = Vec::new();
    for i in (0..hex.len()).step_by(2) {
        if let Ok(byte) = u8::from_str_radix(&hex[i..i+2], 16) {
            result.push(byte);
        } else {
            return None;
        }
    }
    
    Some(result)
}

pub fn get_current_timestamp_nanos() -> u64 {
    match proxy_wasm::hostcalls::get_current_time() {
        Ok(system_time) => {
            // Convert SystemTime to nanoseconds
            system_time.duration_since(std::time::UNIX_EPOCH)
                .map(|duration| duration.as_nanos() as u64)
                .unwrap_or_else(|_| {
                    // If system_time is before UNIX_EPOCH, use fallback
                    use std::sync::atomic::{AtomicU64, Ordering};
                    static TIMESTAMP_COUNTER: AtomicU64 = AtomicU64::new(1609459200000000000_u64); // Start at Jan 1, 2021
                    TIMESTAMP_COUNTER.fetch_add(1000000, Ordering::Relaxed)
                })
        },
        Err(_) => {
            // Fallback to counter-based approach if host function fails
            use std::sync::atomic::{AtomicU64, Ordering};
            static TIMESTAMP_COUNTER: AtomicU64 = AtomicU64::new(1609459200000000000_u64); // Start at Jan 1, 2021
            TIMESTAMP_COUNTER.fetch_add(1000000, Ordering::Relaxed)
        }
    }
}

fn should_skip_header(key: &str) -> bool {
    matches!(key.to_lowercase().as_str(), 
        "authorization" | "cookie" | "set-cookie" | 
        "x-public-key" | "x-auth-token" | "bearer" |
        "proxy-authorization"
    )
}

fn is_text_content(headers: &HashMap<String, String>) -> bool {
    if let Some(content_type) = headers.get("content-type") {
        content_type.starts_with("text/") || 
        content_type.starts_with("application/json") ||
        content_type.starts_with("application/xml") ||
        content_type.starts_with("application/x-www-form-urlencoded")
    } else {
        false
    }
}

fn hex_encode(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{:02x}", b)).collect()
}

#[cfg(test)]
mod session_id_tests {
    //! Contract tests for session-id parsing.
    //!
    //! The proxy must treat the session id as an OPAQUE string — the SDK
    //! owns the format (today `sess_<base64url>`, tomorrow anything else).
    //! The proxy never synthesizes a session id when none is present:
    //! doing so would pollute the SDK's namespace and cause inject lookups
    //! to miss against the runtime. These tests pin that contract.
    //!
    //! Note: `SpanBuilder::new()` calls proxy-wasm host functions for
    //! timestamps/span-ids, so these tests exercise `with_context` on
    //! pre-constructed `SpanBuilder` values. They compile-check the
    //! reader logic and (when the crate is ever made native-testable)
    //! will also execute.
    use super::*;
    use std::collections::HashMap;

    fn builder_with_session(existing: &str) -> SpanBuilder {
        SpanBuilder {
            trace_id: vec![0u8; 16],
            parent_span_id: None,
            current_span_id: vec![0u8; 8],
            service_name: "svc".into(),
            traffic_direction: "outbound".into(),
            public_key: "pk".into(),
            session_id: existing.to_string(),
        }
    }

    #[test]
    fn session_id_from_header_is_opaque_regardless_of_format() {
        // Any format the SDK chooses must be accepted verbatim.
        for sid in ["sess_abcdef", "sp-session-123", "uuid-like-1234", "42", "hello/world=="] {
            let mut headers = HashMap::new();
            headers.insert("x-softprobe-session-id".to_string(), sid.to_string());
            let sb = builder_with_session("").with_context(&headers);
            assert_eq!(sb.get_session_id(), sid, "must preserve opaque session id {sid:?}");
            assert!(sb.has_session_id());
        }
    }

    #[test]
    fn session_id_from_tracestate_is_opaque() {
        let mut headers = HashMap::new();
        headers.insert(
            "tracestate".to_string(),
            "vendor1=value1,x-softprobe-session-id=sess_xYz==,vendor2=value2".to_string(),
        );
        let sb = builder_with_session("").with_context(&headers);
        assert_eq!(sb.get_session_id(), "sess_xYz==");
    }

    #[test]
    fn header_takes_precedence_over_tracestate() {
        let mut headers = HashMap::new();
        headers.insert("x-softprobe-session-id".to_string(), "sess_from_header".into());
        headers.insert(
            "tracestate".to_string(),
            "x-softprobe-session-id=sess_from_tracestate".into(),
        );
        let sb = builder_with_session("").with_context(&headers);
        assert_eq!(sb.get_session_id(), "sess_from_header");
    }

    #[test]
    fn proxy_does_not_synthesize_when_absent() {
        // The critical regression: the proxy used to fabricate a
        // `sp-session-...` id here, which then leaked into tracestate
        // and caused `/v1/inject` lookups to miss the SDK's session.
        let headers = HashMap::new();
        let sb = builder_with_session("").with_context(&headers);
        assert!(!sb.has_session_id(), "must NOT invent a session id");
        assert_eq!(sb.get_session_id(), "");
    }

    #[test]
    fn proxy_does_not_synthesize_even_with_traceparent_but_no_session() {
        let mut headers = HashMap::new();
        headers.insert(
            "traceparent".into(),
            "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01".into(),
        );
        let sb = builder_with_session("").with_context(&headers);
        assert!(!sb.has_session_id());
    }
}
