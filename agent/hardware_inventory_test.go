//go:build testing && linux

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHardwareInventoryOptions(t *testing.T) {
	require.NotNil(t, hardwareInventoryOptions(), "on by default")
	assert.False(t, hardwareInventoryOptions().Identifiers, "identifiers stay opt-in")

	t.Setenv("HARDWARE_INVENTORY_IDENTIFIERS", "true")
	assert.True(t, hardwareInventoryOptions().Identifiers)

	t.Setenv("HARDWARE_INVENTORY", "false")
	assert.Nil(t, hardwareInventoryOptions())
}
