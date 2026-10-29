package container

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyStats is container.Stats as agents and hubs before the stack fields know it: cbor keys 0
// through 11. A parc runs mixed versions - agents are usually updated after the hub - so both
// directions have to keep working.
type legacyStats struct {
	Name            string    `cbor:"0,keyasint"`
	Cpu             float64   `cbor:"1,keyasint"`
	Mem             float64   `cbor:"2,keyasint"`
	NetworkSent     float64   `cbor:"3,keyasint,omitzero"`
	NetworkRecv     float64   `cbor:"4,keyasint,omitzero"`
	Health          int       `cbor:"5,keyasint"`
	Status          string    `cbor:"6,keyasint"`
	Id              string    `cbor:"7,keyasint"`
	Image           string    `cbor:"8,keyasint"`
	Bandwidth       [2]uint64 `cbor:"9,keyasint,omitzero"`
	Ports           string    `cbor:"10,keyasint"`
	UpdateAvailable bool      `cbor:"11,keyasint,omitzero"`
}

func newStats() Stats {
	return Stats{
		Name: "mentors_web.1.kz9", Cpu: 12.5, Mem: 256.5,
		Bandwidth: [2]uint64{1024, 2048},
		Health:    DockerHealthHealthy, Status: "Up 3 days",
		Id: "aaaaaaaaaaaa", Image: "registry.example.com/mentors/web:1.2.3",
		Ports: "8080:80", UpdateAvailable: true,
		Stack: "mentors", Service: "web",
	}
}

func TestStatsWireCompatibility(t *testing.T) {
	t.Run("new agent to new hub", func(t *testing.T) {
		data, err := cbor.Marshal(newStats())
		require.NoError(t, err)
		var decoded Stats
		require.NoError(t, cbor.Unmarshal(data, &decoded))
		assert.Equal(t, newStats(), decoded)
	})

	t.Run("new agent to old hub ignores the new keys", func(t *testing.T) {
		data, err := cbor.Marshal(newStats())
		require.NoError(t, err)
		var decoded legacyStats
		require.NoError(t, cbor.Unmarshal(data, &decoded), "an old hub must not fail on keys it does not know")
		assert.Equal(t, "mentors_web.1.kz9", decoded.Name)
		assert.Equal(t, 12.5, decoded.Cpu)
		assert.Equal(t, "Up 3 days", decoded.Status)
		assert.Equal(t, [2]uint64{1024, 2048}, decoded.Bandwidth)
		assert.True(t, decoded.UpdateAvailable)
	})

	t.Run("old agent to new hub leaves the group empty", func(t *testing.T) {
		data, err := cbor.Marshal(legacyStats{
			Name: "solo", Cpu: 1, Mem: 2, Status: "Up 1 hour", Id: "bbbbbbbbbbbb", Image: "nginx:1",
		})
		require.NoError(t, err)
		var decoded Stats
		require.NoError(t, cbor.Unmarshal(data, &decoded))
		assert.Equal(t, "solo", decoded.Name)
		assert.Equal(t, "nginx:1", decoded.Image)
		assert.Empty(t, decoded.Stack)
		assert.Empty(t, decoded.Service)
	})

	t.Run("a container with no group sends no extra keys", func(t *testing.T) {
		stats := newStats()
		stats.Stack, stats.Service = "", ""
		data, err := cbor.Marshal(stats)
		require.NoError(t, err)
		var keys map[int]cbor.RawMessage
		require.NoError(t, cbor.Unmarshal(data, &keys))
		assert.NotContains(t, keys, 12, "an empty stack must not be sent")
		assert.NotContains(t, keys, 13, "an empty service must not be sent")

		withGroup, err := cbor.Marshal(newStats())
		require.NoError(t, err)
		require.NoError(t, cbor.Unmarshal(withGroup, &keys))
		assert.Contains(t, keys, 12)
		assert.Contains(t, keys, 13)
	})

	t.Run("the keys are 12 and 13", func(t *testing.T) {
		data, err := cbor.Marshal(Stats{Stack: "mentors", Service: "web"})
		require.NoError(t, err)
		var keys map[int]cbor.RawMessage
		require.NoError(t, cbor.Unmarshal(data, &keys))
		var stack, service string
		require.NoError(t, cbor.Unmarshal(keys[12], &stack))
		require.NoError(t, cbor.Unmarshal(keys[13], &service))
		assert.Equal(t, "mentors", stack)
		assert.Equal(t, "web", service)
	})
}

// The hub stores container stats as JSON, and nothing reads the group from those records yet, so
// the fields stay off the stored shape like Health, Status, Id, Image and Ports. Storing them is a
// hub change of its own: the averaged records would need to carry them too, or grouping would work
// in the 1h view and vanish at 12h.
func TestStatsJSONKeepsStoredShape(t *testing.T) {
	data, err := json.Marshal(newStats())
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.NotContains(t, decoded, "st")
	assert.NotContains(t, decoded, "sv")
	assert.NotContains(t, decoded, "Stack")
	assert.NotContains(t, decoded, "Service")
	// the fields that are stored are untouched
	assert.Equal(t, "mentors_web.1.kz9", decoded["n"])
	assert.Equal(t, 12.5, decoded["c"])
}
