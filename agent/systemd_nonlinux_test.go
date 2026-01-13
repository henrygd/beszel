//go:build !linux && testing

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSystemdManager(t *testing.T) {
	manager, err := newSystemdManager()
	assert.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestSystemdManagerGetServiceStats(t *testing.T) {
	manager, err := newSystemdManager()
	assert.NoError(t, err)

	// Test with refresh = true
	result := manager.getServiceStats("any-service", true)
	assert.Nil(t, result)

	// Test with refresh = false
	result = manager.getServiceStats("any-service", false)
	assert.Nil(t, result)
}

func TestSystemdManagerGetServiceDetails(t *testing.T) {
	manager, err := newSystemdManager()
	assert.NoError(t, err)

	result, err := manager.getServiceDetails("any-service")
	assert.Error(t, err)
	assert.Equal(t, "systemd manager unavailable", err.Error())
	assert.Nil(t, result)

	// Test with empty service name
	result, err = manager.getServiceDetails("")
	assert.Error(t, err)
	assert.Equal(t, "systemd manager unavailable", err.Error())
	assert.Nil(t, result)
}

func TestSystemdManagerFields(t *testing.T) {
	manager, err := newSystemdManager()
	assert.NoError(t, err)

	// The non-linux manager should be a simple struct with no special fields
	// We can't test private fields directly, but we can test the methods work
	assert.NotNil(t, manager)
}
