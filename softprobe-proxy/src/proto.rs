#![allow(unused_imports)]

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

pub use opentelemetry::proto::common::v1::{any_value, AnyValue, KeyValue};
pub use opentelemetry::proto::resource::v1::Resource;
pub use opentelemetry::proto::trace::v1::{ResourceSpans, ScopeSpans, Span, TracesData};
