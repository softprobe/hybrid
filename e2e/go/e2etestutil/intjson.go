package e2etestutil

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// OtlpJSONInt64 unmarshals OTLP JSON intValue, which may be a number or a decimal string (protojson).
type OtlpJSONInt64 struct {
	V *int64
}

func (o *OtlpJSONInt64) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		o.V = nil
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err == nil {
		o.V = &n
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("intValue: %w", err)
	}
	parsed, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("intValue %q: %w", s, err)
	}
	o.V = &parsed
	return nil
}
