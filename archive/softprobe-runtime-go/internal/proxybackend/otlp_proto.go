package proxybackend

import (
	"errors"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// normalizeOTLPJSON converts protobuf OTLP payloads into the JSON profile used by the runtime.
// JSON payloads are parsed and re-encoded with protojson so a single code path uses official types.
func normalizeOTLPJSON(payload []byte) ([]byte, error) {
	var protoPayload tracev1.TracesData
	if err := proto.Unmarshal(payload, &protoPayload); err == nil {
		return protojson.MarshalOptions{UseProtoNames: false}.Marshal(&protoPayload)
	}

	var jsonPayload tracev1.TracesData
	if err := protojson.Unmarshal(payload, &jsonPayload); err == nil {
		return protojson.MarshalOptions{UseProtoNames: false}.Marshal(&jsonPayload)
	}

	return nil, errors.New("invalid otlp payload")
}

// encodeInjectResponseProto converts a mock response into an OTLP protobuf hit payload.
func encodeInjectResponseProto(response *MockResponse) ([]byte, error) {
	attrs := MockResponseAttributes(response)
	spanAttrs := make([]*commonv1.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		spanAttrs = append(spanAttrs, mockAttributeToProto(attr))
	}

	msg := tracev1.TracesData{
		ResourceSpans: []*tracev1.ResourceSpans{{
			ScopeSpans: []*tracev1.ScopeSpans{{
				Spans: []*tracev1.Span{{
					Attributes: spanAttrs,
				}},
			}},
		}},
	}

	return proto.Marshal(&msg)
}

func mockAttributeToProto(attr Attribute) *commonv1.KeyValue {
	value := &commonv1.AnyValue{}
	switch {
	case attr.Value.Int != nil:
		value.Value = &commonv1.AnyValue_IntValue{IntValue: *attr.Value.Int}
	case attr.Value.String != nil:
		value.Value = &commonv1.AnyValue_StringValue{StringValue: *attr.Value.String}
	}

	return &commonv1.KeyValue{
		Key:   attr.Key,
		Value: value,
	}
}
