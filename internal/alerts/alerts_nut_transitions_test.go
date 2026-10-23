//go:build testing

package alerts

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNutRecoveryTransitions(t *testing.T) {
	for _, previous := range []string{"ON_BATTERY", "LOW_BATTERY", "OVERLOAD", "FAULT"} {
		assert.True(t, shouldSendNutDeviceAlert(previous, "ONLINE"), previous)
	}
	for _, previous := range []string{"", "UNKNOWN", "ONLINE"} {
		assert.False(t, shouldSendNutDeviceAlert(previous, "ONLINE"), previous)
	}
	assert.False(t, shouldSendNutDeviceAlert("LOW_BATTERY", "ON_BATTERY"))
	assert.False(t, shouldSendNutDeviceAlert("ON_BATTERY", "UNKNOWN"))
	assert.False(t, shouldSendNutDeviceAlert("ON_BATTERY", "ON_BATTERY"))
}
