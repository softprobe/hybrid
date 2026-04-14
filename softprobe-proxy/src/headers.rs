use std::collections::HashMap;

/// Case-insensitive HTTP header lookup (Envoy / Go may use mixed casing).
pub fn header_get_ci<'a>(headers: &'a HashMap<String, String>, name: &str) -> Option<&'a String> {
    headers
        .iter()
        .find(|(k, _)| k.eq_ignore_ascii_case(name))
        .map(|(_, v)| v)
}

/// Detect service name from headers or configuration
pub fn detect_service_name(
    request_headers: &HashMap<String, String>,
    config_service_name: &str,
) -> String {
    // Use configured service_name if it's not default
    if !config_service_name.is_empty() && config_service_name != "default-service" {
        crate::sp_debug!("Using configured service_name: {}", config_service_name);
        return config_service_name.to_string();
    }

    let current_service_headers = vec!["x-sp-service-name"];
    for header_name in current_service_headers {
        if let Some(header_value) = request_headers.get(header_name) {
            if !header_value.is_empty() {
                crate::sp_debug!("Got service_name from header: {} -> {}", header_name, header_value);
                return header_value.clone();
            }
        }
    }
    config_service_name.to_string()
}

/// Merge Softprobe session correlation into W3C `tracestate` without duplicating `traceparent`.
///
/// Trace identity stays on the standard **`traceparent`** header (OpenTelemetry TraceContext).
/// Drops a legacy, non-W3C duplicate traceparent list member on `tracestate` if present on the inbound header.
/// Preserves other third-party `tracestate` entries and injects `x-softprobe-session-id` when missing.
pub fn build_new_tracestate(request_headers: &HashMap<String, String>, session_id: &str) -> String {
    let mut preserved: Vec<String> = Vec::new();
    let mut has_sp_session_id = false;

    if let Some(existing_tracestate) = header_get_ci(request_headers, "tracestate") {
        if !existing_tracestate.is_empty() {
            for entry in existing_tracestate.split(',') {
                let entry = entry.trim();
                if entry.is_empty() {
                    continue;
                }
                let lower = entry.to_ascii_lowercase();
                if lower.starts_with(concat!("x-sp-", "traceparent=")) {
                    continue;
                }
                if lower.starts_with("x-softprobe-session-id=")
                    || lower.starts_with("x-sp-session-id=")
                {
                    has_sp_session_id = true;
                }
                preserved.push(entry.to_string());
            }
        }
    }

    let mut out: Vec<String> = Vec::new();
    if !session_id.is_empty() && !has_sp_session_id {
        out.push(format!("x-softprobe-session-id={}", session_id));
    }
    out.extend(preserved);
    let new_tracestate = out.join(",");
    crate::sp_debug!("Merged tracestate (OTel traceparent unchanged): {}", new_tracestate);
    new_tracestate
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn test_detect_service_name_with_configured_name() {
        let headers = HashMap::new();
        let config_name = "my-service";
        
        let result = detect_service_name(&headers, config_name);
        assert_eq!(result, "my-service");
    }

    #[test]
    fn test_detect_service_name_with_default_config() {
        let headers = HashMap::new();
        let config_name = "default-service";
        
        let result = detect_service_name(&headers, config_name);
        assert_eq!(result, "default-service");
    }

    #[test]
    fn test_detect_service_name_from_header() {
        let mut headers = HashMap::new();
        headers.insert("x-sp-service-name".to_string(), "header-service".to_string());
        let config_name = "default-service";
        
        let result = detect_service_name(&headers, config_name);
        assert_eq!(result, "header-service");
    }

    #[test]
    fn test_detect_service_name_header_overrides_config() {
        let mut headers = HashMap::new();
        headers.insert("x-sp-service-name".to_string(), "header-service".to_string());
        let config_name = "my-service";
        
        let result = detect_service_name(&headers, config_name);
        assert_eq!(result, "my-service"); // Config takes precedence if not default
    }

    #[test]
    fn test_detect_service_name_empty_header() {
        let headers = HashMap::from([("x-sp-service-name".to_string(), "".to_string())]);
        let config_name = "default-service";
        
        let result = detect_service_name(&headers, config_name);
        assert_eq!(result, "default-service");
    }

    #[test]
    fn test_build_new_tracestate_with_no_existing() {
        let headers = HashMap::new();
        let result = build_new_tracestate(&headers, "");
        assert!(result.is_empty());
    }

    #[test]
    fn test_build_new_tracestate_with_existing_entries() {
        let mut headers = HashMap::new();
        headers.insert("tracestate".to_string(), "vendor1=value1,vendor2=value2".to_string());
        let result = build_new_tracestate(&headers, "");
        assert!(result.contains("vendor1=value1"));
        assert!(result.contains("vendor2=value2"));
    }

    #[test]
    fn test_build_new_tracestate_preserves_other_vendor_entries() {
        let mut headers = HashMap::new();
        headers.insert(
            "tracestate".to_string(),
            "acmevendor=key%3Dvalue,vendor1=value1".to_string(),
        );
        let result = build_new_tracestate(&headers, "");
        assert!(result.contains("vendor1=value1"));
        assert!(result.contains("acmevendor="));
    }

    #[test]
    fn test_build_new_tracestate_drops_legacy_duplicate_traceparent_list_member() {
        let mut headers = HashMap::new();
        let legacy = concat!("x-sp-", "traceparent=00-aaa-bbb-01");
        headers.insert(
            "tracestate".to_string(),
            format!("{legacy},vendor1=value1,x-softprobe-session-id=old"),
        );
        let result = build_new_tracestate(&headers, "sess_new");
        let needle = concat!("x-sp-", "traceparent");
        assert!(
            !result.to_ascii_lowercase().contains(needle),
            "got {result:?}"
        );
        assert!(result.contains("vendor1=value1"));
        assert!(result.contains("x-softprobe-session-id=sess_new"));
    }

    #[test]
    fn test_build_new_tracestate_adds_softprobe_session_id() {
        let headers = HashMap::new();
        let result = build_new_tracestate(&headers, "sess_123");
        assert_eq!(result, "x-softprobe-session-id=sess_123");
    }

    #[test]
    fn test_build_new_tracestate_handles_whitespace() {
        let mut headers = HashMap::new();
        headers.insert("tracestate".to_string(), " vendor1=value1 , vendor2=value2 ".to_string());
        let result = build_new_tracestate(&headers, "");
        assert!(result.contains("vendor1=value1"));
        assert!(result.contains("vendor2=value2"));
    }

    #[test]
    fn test_build_new_tracestate_empty_existing() {
        let mut headers = HashMap::new();
        headers.insert("tracestate".to_string(), "".to_string());
        let result = build_new_tracestate(&headers, "");
        assert!(result.is_empty());
    }

    #[test]
    fn test_header_get_ci() {
        let mut headers = HashMap::new();
        headers.insert("Traceparent".to_string(), "v".to_string());
        assert_eq!(header_get_ci(&headers, "traceparent").map(|s| s.as_str()), Some("v"));
    }
}
