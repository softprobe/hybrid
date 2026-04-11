#[path = "src/logging.rs"]
mod logging;
#[path = "src/config.rs"]
mod config;
#[path = "src/http_helpers.rs"]
mod http_helpers;

fn main() {}

#[cfg(test)]
mod tests {
    use super::config::Config;
    use serde_json::json;

    #[test]
    fn default_backend_url_points_at_hosted_proxy_backend() {
        let config = Config::default();
        assert_eq!(config.sp_backend_url, "https://o.softprobe.ai");
    }

    #[test]
    fn custom_backend_url_overrides_the_default() {
        let mut config = Config::default();
        let payload = json!({
            "sp_backend_url": "https://proxy.example.com"
        });

        assert!(config.parse_from_json(serde_json::to_string(&payload).unwrap().as_bytes()));
        assert_eq!(config.sp_backend_url, "https://proxy.example.com");
    }
}
