package smart

import (
	"encoding/json"
	"math"
	"strings"
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
		// Read exactly, not through float64 (review on #2732)
		{`200000000000000000000e-20`, 2, false},
		{`18446744073709551615e0`, math.MaxUint64, false},
		{`1844674407370955161.5e1`, math.MaxUint64, false},
		{`18446744073709551615.000`, math.MaxUint64, false},
		{`18446744073709551616e0`, 0, true},
		{`9007199254740993e0`, 9007199254740993, false},
		{`100000000000000000000e-10`, 10000000000, false},
		{`10.0`, 10, false},
		{`0.0e5`, 0, false},
		{`0e999999999`, 0, false},
		{`1e999999999`, 0, true},
		{`1e18446744073709551619`, 0, true}, // 2^64+3, must not wrap to 1e3
		{`1e20`, 0, true},
		{`0.000000000000000000001e21`, 1, false}, // leading zeros do not count as digits
		{`0.00000000000000000000018446744073709551615e41`, math.MaxUint64, false},
	} {
		c := NVMeCounter{Value: 7, Overflow: !tc.overflow} // stale state must be reset
		assert.NoError(t, json.Unmarshal([]byte(tc.input), &c), tc.input)
		assert.Equal(t, NVMeCounter{Value: tc.value, Overflow: tc.overflow}, c, tc.input)
	}

	for _, input := range []string{`-1`, `-0`, `-1e3`, `1.5`, `1.5e0`, `1.0000000000000001`, `1e-1`, `25e-1`, `1e-999999999`, `1e-99999999999999999999999`, `"12"`, `true`, `{}`, `[]`} {
		var c NVMeCounter
		assert.Error(t, json.Unmarshal([]byte(input), &c), input)
	}
	assert.Equal(t, "invalid (out of range)", NVMeCounterInvalid)

	// Not valid JSON, so json.Unmarshal never passes them on.
	for _, input := range []string{`Inf`, `+Inf`, `NaN`, `+5`, `1x`, ``, `.5`, `1.`, `1e`, `1e+`, `0x10`, `1_000`} {
		var c NVMeCounter
		assert.Error(t, c.UnmarshalJSON([]byte(input)), input)
	}

	// Huge inputs stay linear: a million digits, and an exponent that cancels them.
	million := strings.Repeat("9", 1_000_000)
	var c NVMeCounter
	assert.NoError(t, json.Unmarshal([]byte(million), &c))
	assert.Equal(t, NVMeCounter{Overflow: true}, c)
	assert.NoError(t, json.Unmarshal([]byte("1"+strings.Repeat("0", 1_000_000)+"e-1000000"), &c))
	assert.Equal(t, NVMeCounter{Value: 1}, c)
	assert.Error(t, json.Unmarshal([]byte("1."+million+"e3"), &c))
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
			{`200000000000000000000e-20`, NVMeCounter{Value: 2}},
			{`18446744073709551615e0`, NVMeCounter{Value: math.MaxUint64}},
			{`null`, NVMeCounter{}},
		} {
			var log NVMeSmartHealthInformationLog
			err := json.Unmarshal([]byte(`{"`+name+`": `+tc.input+`}`), &log)
			assert.NoError(t, err, name+" "+tc.input)
			assert.Equal(t, tc.want, get(&log), name+" "+tc.input)
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
