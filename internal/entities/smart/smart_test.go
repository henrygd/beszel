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
	for _, tc := range []struct {
		input    string
		value    uint64
		overflow bool
	}{
		{`42`, 42, false},
		{`0`, 0, false},
		{`null`, 0, false},
		{`18446744073709551615`, math.MaxUint64, false},
		{`18446744073709551616`, 0, true},
		{`557325022338560884631142400`, 0, true},
		{`340282366920938463463374607431768211455`, 0, true},
		// Exponent form, as after the JSON was re-encoded
		{`1e3`, 1000, false},
		{`1.5E+3`, 1500, false},
		{`1e19`, 10000000000000000000, false},
		{`1.8446744073709552e19`, 0, true},
		{`5.573250223385609e+26`, 0, true},
		{`1e400`, 0, true},
	} {
		c := NVMeCounter{Value: 7, Overflow: !tc.overflow} // stale state must be reset
		assert.NoError(t, json.Unmarshal([]byte(tc.input), &c), tc.input)
		assert.Equal(t, NVMeCounter{Value: tc.value, Overflow: tc.overflow}, c, tc.input)
	}

	for _, input := range []string{`-1`, `-1e3`, `1.5`, `1.5e0`, `"12"`, `true`, `{}`} {
		var c NVMeCounter
		assert.Error(t, json.Unmarshal([]byte(input), &c), input)
	}
	assert.Equal(t, "invalid (out of range)", NVMeCounterInvalid)

	// Not valid JSON, so json.Unmarshal never passes them on; ParseFloat would accept them.
	for _, input := range []string{`Inf`, `+Inf`, `NaN`, `+5`, `1x`, ``} {
		var c NVMeCounter
		assert.Error(t, c.UnmarshalJSON([]byte(input)), input)
	}
}

func TestNVMeCounterEvery128BitField(t *testing.T) {
	fields := map[string]func(*NVMeSmartHealthInformationLog) NVMeCounter{
		"data_units_read":      func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.DataUnitsRead },
		"data_units_written":   func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.DataUnitsWritten },
		"host_reads":           func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.HostReads },
		"host_writes":          func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.HostWrites },
		"controller_busy_time": func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.ControllerBusyTime },
		"power_cycles":         func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.PowerCycles },
		"power_on_hours":       func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.PowerOnHours },
		"unsafe_shutdowns":     func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.UnsafeShutdowns },
		"media_errors":         func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.MediaErrors },
		"num_err_log_entries":  func(l *NVMeSmartHealthInformationLog) NVMeCounter { return l.NumErrLogEntries },
	}
	for name, get := range fields {
		for _, tc := range []struct {
			input string
			want  NVMeCounter
		}{
			{`18446744073709551616`, NVMeCounter{Overflow: true}},
			{`5.573250223385609e+26`, NVMeCounter{Overflow: true}},
			{`12345`, NVMeCounter{Value: 12345}},
			{`null`, NVMeCounter{}},
		} {
			var log NVMeSmartHealthInformationLog
			err := json.Unmarshal([]byte(`{"`+name+`": `+tc.input+`}`), &log)
			assert.NoError(t, err, name, tc.input)
			assert.Equal(t, tc.want, get(&log), name, tc.input)
		}
	}

	// TNVMCAP (nvme_total_capacity) is a 128-bit field too.
	for _, tc := range []struct {
		input string
		want  NVMeCounter
	}{
		{`18446744073709551616`, NVMeCounter{Overflow: true}},
		{`5.573250223385609e+26`, NVMeCounter{Overflow: true}},
		{`1000204886016`, NVMeCounter{Value: 1000204886016}},
		{`null`, NVMeCounter{}},
	} {
		var out SmartctlOutput
		assert.NoError(t, json.Unmarshal([]byte(`{"nvme_total_capacity": `+tc.input+`}`), &out), tc.input)
		assert.Equal(t, tc.want, out.NVMeTotalCapacity, tc.input)
	}
}
