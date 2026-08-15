//go:build testing

package container

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiStatsBlkioJSONAvailability(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectStats    bool
		expectEntries  bool
		expectedLength int
	}{
		{name: "blkio stats omitted", input: `{}`},
		{name: "blkio stats null", input: `{"blkio_stats":null}`},
		{name: "entries omitted", input: `{"blkio_stats":{}}`, expectStats: true},
		{name: "entries null", input: `{"blkio_stats":{"io_service_bytes_recursive":null}}`, expectStats: true},
		{name: "entries empty", input: `{"blkio_stats":{"io_service_bytes_recursive":[]}}`, expectStats: true, expectEntries: true},
		{
			name: "entries populated",
			input: `{"blkio_stats":{"io_service_bytes_recursive":[` +
				`{"major":8,"minor":0,"op":"Read","value":12345},` +
				`{"major":8,"minor":0,"op":"Write","value":67890}]}}`,
			expectStats:    true,
			expectEntries:  true,
			expectedLength: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stats ApiStats
			require.NoError(t, json.Unmarshal([]byte(test.input), &stats))

			if !test.expectStats {
				assert.Nil(t, stats.BlkioStats)
				return
			}
			require.NotNil(t, stats.BlkioStats)
			if test.expectEntries {
				assert.NotNil(t, stats.BlkioStats.IoServiceBytesRecursive)
			} else {
				assert.Nil(t, stats.BlkioStats.IoServiceBytesRecursive)
			}
			assert.Len(t, stats.BlkioStats.IoServiceBytesRecursive, test.expectedLength)
		})
	}
}

func TestApiStatsBlkioJSONValues(t *testing.T) {
	input := `{"blkio_stats":{"io_service_bytes_recursive":[{"major":8,"minor":16,"op":"Read","value":12345}]}}`
	var stats ApiStats
	require.NoError(t, json.Unmarshal([]byte(input), &stats))
	require.NotNil(t, stats.BlkioStats)
	require.Len(t, stats.BlkioStats.IoServiceBytesRecursive, 1)
	assert.Equal(t, BlkioStatEntry{Major: 8, Minor: 16, Op: "Read", Value: 12345}, stats.BlkioStats.IoServiceBytesRecursive[0])
}

func TestStatsDiskIOJSON(t *testing.T) {
	tests := []struct {
		name       string
		diskIO     *[2]uint64
		expectDisk bool
		expected   [2]uint64
	}{
		{name: "nil omitted"},
		{name: "zero included", diskIO: &[2]uint64{0, 0}, expectDisk: true, expected: [2]uint64{0, 0}},
		{name: "order preserved", diskIO: &[2]uint64{123, 456}, expectDisk: true, expected: [2]uint64{123, 456}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(Stats{DiskIO: test.diskIO})
			require.NoError(t, err)

			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			diskIO, exists := decoded["d"]
			assert.Equal(t, test.expectDisk, exists)
			if test.expectDisk {
				var values [2]uint64
				require.NoError(t, json.Unmarshal(diskIO, &values))
				assert.Equal(t, test.expected, values)
			}
		})
	}
}

func TestStatsDiskIOCBORKey(t *testing.T) {
	diskIO := [2]uint64{123, 456}
	stats := Stats{
		Name:        "container",
		Cpu:         1,
		Mem:         2,
		NetworkSent: 3,
		NetworkRecv: 4,
		Bandwidth:   [2]uint64{5, 6},
		Health:      DockerHealthHealthy,
		Status:      "running",
		Id:          "id",
		Image:       "image",
		Ports:       "80",
		DiskIO:      &diskIO,
	}

	encoded, err := cbor.Marshal(stats)
	require.NoError(t, err)
	var decoded map[uint64]cbor.RawMessage
	require.NoError(t, cbor.Unmarshal(encoded, &decoded))

	expectedKeys := []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	for _, key := range expectedKeys {
		assert.Contains(t, decoded, key)
	}
	assert.Len(t, decoded, len(expectedKeys))

	var decodedDiskIO [2]uint64
	require.NoError(t, cbor.Unmarshal(decoded[11], &decodedDiskIO))
	assert.Equal(t, diskIO, decodedDiskIO)

	stats.DiskIO = nil
	encoded, err = cbor.Marshal(stats)
	require.NoError(t, err)
	decoded = nil
	require.NoError(t, cbor.Unmarshal(encoded, &decoded))
	assert.NotContains(t, decoded, uint64(11))
}

func TestStatsDiskIOCBORCompatibility(t *testing.T) {
	type oldStats struct {
		Name string `cbor:"0,keyasint"`
	}
	type newStats struct {
		DiskIO *[2]uint64 `cbor:"11,keyasint,omitempty"`
	}

	oldPayload, err := cbor.Marshal(oldStats{Name: "old"})
	require.NoError(t, err)
	var decodedNew newStats
	require.NoError(t, cbor.Unmarshal(oldPayload, &decodedNew))
	assert.Nil(t, decodedNew.DiskIO)

	newPayload, err := cbor.Marshal(struct {
		Name  string `cbor:"0,keyasint"`
		Extra uint64 `cbor:"11,keyasint"`
	}{Name: "new", Extra: 1})
	require.NoError(t, err)
	var decodedOld oldStats
	require.NoError(t, cbor.Unmarshal(newPayload, &decodedOld))
	assert.Equal(t, "new", decodedOld.Name)

	zero := &[2]uint64{}
	encoded, err := cbor.Marshal(newStats{DiskIO: zero})
	require.NoError(t, err)
	var decodedZero newStats
	require.NoError(t, cbor.Unmarshal(encoded, &decodedZero))
	require.NotNil(t, decodedZero.DiskIO)
	assert.Equal(t, [2]uint64{}, *decodedZero.DiskIO)
}
