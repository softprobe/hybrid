package proxybackend

import (
	"fmt"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func resourceAttrString(res *resourcev1.Resource, key string) string {
	if res == nil {
		return ""
	}
	for _, attr := range res.Attributes {
		if attr.Key == key {
			return anyValueString(attr.Value)
		}
	}
	return ""
}

func spanAttrString(span *tracev1.Span, key string) string {
	if span == nil {
		return ""
	}
	for _, attr := range span.Attributes {
		if attr.Key == key {
			return anyValueString(attr.Value)
		}
	}
	return ""
}

func spanPrefixedStringAttrs(span *tracev1.Span, prefix string) [][2]string {
	if span == nil {
		return nil
	}
	var headers [][2]string
	for _, attr := range span.Attributes {
		if len(attr.Key) < len(prefix) || attr.Key[:len(prefix)] != prefix {
			continue
		}
		headers = append(headers, [2]string{attr.Key[len(prefix):], anyValueString(attr.Value)})
	}
	return headers
}

func anyValueString(v *commonv1.AnyValue) string {
	if v == nil {
		return ""
	}
	switch x := v.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return x.StringValue
	default:
		return ""
	}
}

func anyValueAsHTTPStatus(v *commonv1.AnyValue) (int, bool) {
	if v == nil {
		return 0, false
	}
	switch x := v.Value.(type) {
	case *commonv1.AnyValue_IntValue:
		return int(x.IntValue), true
	case *commonv1.AnyValue_StringValue:
		var code int
		_, err := fmt.Sscanf(x.StringValue, "%d", &code)
		if err != nil {
			return 0, false
		}
		return code, true
	default:
		return 0, false
	}
}
