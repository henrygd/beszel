package smart

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSmartRawValueUnmarshalDuration(t *testing.T) {
	input := []byte(`{"value":"62312h+33m+50.907s","string":"62312h+33m+50.907s"}`)
	var raw RawValue
	err := json.Unmarshal(input, &raw)
	assert.NoError(t, err)

	assert.EqualValues(t, 62312, raw.Value)
}

func TestSmartRawValueUnmarshalNumericString(t *testing.T) {
	input := []byte(`{"value":"7344","string":"7344"}`)
	var raw RawValue
	err := json.Unmarshal(input, &raw)
	assert.NoError(t, err)

	assert.EqualValues(t, 7344, raw.Value)
}

func TestSmartRawValueUnmarshalParenthetical(t *testing.T) {
	input := []byte(`{"value":"39925 (212 206 0)","string":"39925 (212 206 0)"}`)
	var raw RawValue
	err := json.Unmarshal(input, &raw)
	assert.NoError(t, err)

	assert.EqualValues(t, 39925, raw.Value)
}

func TestSmartRawValueUnmarshalDurationWithFractions(t *testing.T) {
	input := []byte(`{"value":"2748h+31m+49.560s","string":"2748h+31m+49.560s"}`)
	var raw RawValue
	err := json.Unmarshal(input, &raw)
	assert.NoError(t, err)

	assert.EqualValues(t, 2748, raw.Value)
}

func TestSmartRawValueUnmarshalParentheticalRawValue(t *testing.T) {
	input := []byte(`{"value":57891864217128,"string":"39925 (212 206 0)"}`)
	var raw RawValue
	err := json.Unmarshal(input, &raw)
	assert.NoError(t, err)

	assert.EqualValues(t, 39925, raw.Value)
}

func TestSmartRawValueUnmarshalDurationRawValue(t *testing.T) {
	input := []byte(`{"value":57891864217128,"string":"2748h+31m+49.560s"}`)
	var raw RawValue
	err := json.Unmarshal(input, &raw)
	assert.NoError(t, err)

	assert.EqualValues(t, 2748, raw.Value)
}
