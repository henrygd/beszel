package system

import (
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetailsCPUFrequencyWireCompatibility(t *testing.T) {
	// Key 11 was already used by agents and hubs before CPU frequency support.
	type legacyDetails struct {
		ZfsInterval time.Duration `cbor:"11,keyasint,omitempty"`
	}
	const interval = 30 * time.Minute

	t.Run("old agent to new hub", func(t *testing.T) {
		data, err := cbor.Marshal(legacyDetails{ZfsInterval: interval})
		require.NoError(t, err)
		var decoded Details
		require.NoError(t, cbor.Unmarshal(data, &decoded))
		assert.Equal(t, interval, decoded.ZfsInterval)
		assert.Zero(t, decoded.CpuMHz)
	})

	t.Run("new agent to old hub", func(t *testing.T) {
		data, err := cbor.Marshal(Details{ZfsInterval: interval, CpuMHz: 1800.5})
		require.NoError(t, err)
		var decoded legacyDetails
		require.NoError(t, cbor.Unmarshal(data, &decoded))
		assert.Equal(t, interval, decoded.ZfsInterval)
	})

	t.Run("new agent to new hub", func(t *testing.T) {
		original := Details{ZfsInterval: interval, CpuMHz: 1800.5}
		data, err := cbor.Marshal(original)
		require.NoError(t, err)
		var decoded Details
		require.NoError(t, cbor.Unmarshal(data, &decoded))
		assert.Equal(t, original, decoded)
	})
}
