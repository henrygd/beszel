//go:build testing

package systems

import (
	"errors"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/hub/expirymap"
	"github.com/stretchr/testify/assert"
)

func TestRecordSmartFetchResult(t *testing.T) {
	sm := &SystemManager{smartFetchMap: expirymap.New[bool](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	sys := &System{
		Id:            "system-1",
		manager:       sm,
		smartInterval: time.Hour,
	}

	// Successful fetch with devices
	sys.recordSmartFetchResult(nil, 5)
	succeeded, ok := sm.smartFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected smart fetch result to be stored")
	assert.True(t, succeeded, "expected successful fetch state to be recorded")

	// Failed fetch
	sys.recordSmartFetchResult(errors.New("failed"), 0)
	succeeded, ok = sm.smartFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected failed smart fetch state to be stored")
	assert.False(t, succeeded, "expected failed smart fetch state to be marked unsuccessful")

	// Successful fetch but no devices
	sys.recordSmartFetchResult(nil, 0)
	succeeded, ok = sm.smartFetchMap.GetOk(sys.Id)
	assert.True(t, ok, "expected fetch with zero devices to be stored")
	assert.False(t, succeeded, "expected fetch with zero devices to be marked unsuccessful")
}

func TestShouldFetchSmart(t *testing.T) {
	sm := &SystemManager{smartFetchMap: expirymap.New[bool](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	sys := &System{
		Id:            "system-1",
		manager:       sm,
		smartInterval: time.Hour,
	}

	assert.True(t, sys.shouldFetchSmart(), "expected initial smart fetch to be allowed")

	sys.recordSmartFetchResult(errors.New("failed"), 0)
	assert.False(t, sys.shouldFetchSmart(), "expected smart fetch to be blocked while interval entry exists")

	sm.smartFetchMap.Remove(sys.Id)
	assert.True(t, sys.shouldFetchSmart(), "expected smart fetch to be allowed after interval entry is cleared")
}

func TestResetFailedSmartFetchState(t *testing.T) {
	sm := &SystemManager{smartFetchMap: expirymap.New[bool](time.Hour)}
	t.Cleanup(sm.smartFetchMap.StopCleaner)

	sm.smartFetchMap.Set("system-1", false, time.Hour)
	sm.resetFailedSmartFetchState("system-1")
	_, ok := sm.smartFetchMap.GetOk("system-1")
	assert.False(t, ok, "expected failed smart fetch state to be cleared on reconnect")

	sm.smartFetchMap.Set("system-1", true, time.Hour)
	sm.resetFailedSmartFetchState("system-1")
	_, ok = sm.smartFetchMap.GetOk("system-1")
	assert.True(t, ok, "expected successful smart fetch state to be preserved")
}
