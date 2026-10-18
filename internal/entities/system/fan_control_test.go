package system

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFanControlTransport(t *testing.T) {
	original := Stats{Fans: map[string]uint16{"tach": 1200}, FanControl: map[string]float64{"off": 0, "pwm": 50.2}}
	for name, codec := range map[string]struct {
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{"json": {json.Marshal, json.Unmarshal}, "cbor": {cbor.Marshal, cbor.Unmarshal}} {
		t.Run(name, func(t *testing.T) {
			data, err := codec.marshal(original)
			require.NoError(t, err)
			var decoded Stats
			require.NoError(t, codec.unmarshal(data, &decoded))
			assert.Equal(t, original.FanControl, decoded.FanControl)
			assert.Equal(t, original.Fans, decoded.Fans)
			type legacyStats struct {
				Fans map[string]uint16 `json:"f,omitempty" cbor:"36,keyasint,omitempty"`
			}
			var oldHub legacyStats
			require.NoError(t, codec.unmarshal(data, &oldHub))
			assert.Equal(t, original.Fans, oldHub.Fans)
			data, err = codec.marshal(oldHub)
			require.NoError(t, err)
			var newHub Stats
			require.NoError(t, codec.unmarshal(data, &newHub))
			assert.Nil(t, newHub.FanControl)
			assert.Equal(t, original.Fans, newHub.Fans)
		})
	}
	data, err := json.Marshal(Stats{})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	assert.NotContains(t, fields, "fc")
	data, err = cbor.Marshal(Stats{})
	require.NoError(t, err)
	var absent map[uint64]any
	require.NoError(t, cbor.Unmarshal(data, &absent))
	assert.NotContains(t, absent, uint64(42))
	data, err = cbor.Marshal(original)
	require.NoError(t, err)
	var wire map[uint64]any
	require.NoError(t, cbor.Unmarshal(data, &wire))
	assert.Contains(t, wire, uint64(42))
	assert.NotContains(t, wire, uint64(41)) // reserved for the CPU-frequency proposal
}
