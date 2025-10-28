package smart

import (
	"encoding/json"
	"testing"
)

func TestSmartRawValueUnmarshalDuration(t *testing.T) {
	input := []byte(`{"value":"62312h+33m+50.907s","string":"62312h+33m+50.907s"}`)
	var raw RawValue
	if err := json.Unmarshal(input, &raw); err != nil {
		t.Fatalf("unexpected error unmarshalling raw value: %v", err)
	}

	if uint64(raw.Value) != 62312 {
		t.Fatalf("expected hours to be 62312, got %d", raw.Value)
	}
}

func TestSmartRawValueUnmarshalNumericString(t *testing.T) {
	input := []byte(`{"value":"7344","string":"7344"}`)
	var raw RawValue
	if err := json.Unmarshal(input, &raw); err != nil {
		t.Fatalf("unexpected error unmarshalling numeric string: %v", err)
	}

	if uint64(raw.Value) != 7344 {
		t.Fatalf("expected hours to be 7344, got %d", raw.Value)
	}
}
