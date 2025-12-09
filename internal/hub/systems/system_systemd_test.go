//go:build testing

package systems

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSystemdServiceId(t *testing.T) {
	t.Run("deterministic output", func(t *testing.T) {
		systemId := "sys-123"
		serviceName := "nginx.service"

		// Call multiple times and ensure same result
		id1 := makeStableHashId(systemId, serviceName)
		id2 := makeStableHashId(systemId, serviceName)
		id3 := makeStableHashId(systemId, serviceName)

		assert.Equal(t, id1, id2)
		assert.Equal(t, id2, id3)
		assert.NotEmpty(t, id1)
	})

	t.Run("different inputs produce different ids", func(t *testing.T) {
		systemId1 := "sys-123"
		systemId2 := "sys-456"
		serviceName1 := "nginx.service"
		serviceName2 := "apache.service"

		id1 := makeStableHashId(systemId1, serviceName1)
		id2 := makeStableHashId(systemId2, serviceName1)
		id3 := makeStableHashId(systemId1, serviceName2)
		id4 := makeStableHashId(systemId2, serviceName2)

		// All IDs should be different
		assert.NotEqual(t, id1, id2)
		assert.NotEqual(t, id1, id3)
		assert.NotEqual(t, id1, id4)
		assert.NotEqual(t, id2, id3)
		assert.NotEqual(t, id2, id4)
		assert.NotEqual(t, id3, id4)
	})

	t.Run("consistent length", func(t *testing.T) {
		testCases := []struct {
			systemId    string
			serviceName string
		}{
			{"short", "short.service"},
			{"very-long-system-id-that-might-be-used-in-practice", "very-long-service-name.service"},
			{"", "empty-system.service"},
			{"empty-service", ""},
			{"", ""},
		}

		for _, tc := range testCases {
			id := makeStableHashId(tc.systemId, tc.serviceName)
			// FNV-32 produces 8 hex characters
			assert.Len(t, id, 8, "ID should be 8 characters for systemId='%s', serviceName='%s'", tc.systemId, tc.serviceName)
		}
	})

	t.Run("hexadecimal output", func(t *testing.T) {
		id := makeStableHashId("test-system", "test-service")
		assert.NotEmpty(t, id)

		// Should only contain hexadecimal characters
		for _, char := range id {
			assert.True(t, (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'),
				"ID should only contain hexadecimal characters, got: %s", id)
		}
	})
}
