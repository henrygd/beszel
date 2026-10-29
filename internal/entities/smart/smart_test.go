package smart

import (
	"encoding/json"
	"math"
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

func TestNVMeCounterUnmarshal(t *testing.T) {
	var log NVMeSmartHealthInformationLog
	err := json.Unmarshal([]byte(`{
		"media_errors": 557325022338560884631142400,
		"power_on_hours": 18446744073709551615,
		"unsafe_shutdowns": 18446744073709551616,
		"host_reads": 42
	}`), &log)
	assert.NoError(t, err)
	assert.EqualValues(t, uint64(math.MaxUint64), log.MediaErrors)
	assert.EqualValues(t, uint64(math.MaxUint64), log.PowerOnHours)
	assert.EqualValues(t, uint64(math.MaxUint64), log.UnsafeShutdowns)
	assert.EqualValues(t, 42, log.HostReads)

	for _, input := range []string{`-1`, `1.5`, `"12"`} {
		var c NVMeCounter
		assert.Error(t, json.Unmarshal([]byte(input), &c), input)
	}
}
