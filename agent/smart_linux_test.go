//go:build testing && linux

package agent

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureSlog routes slog output into a buffer for the duration of the test.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

// stubSmartctlDetection makes NewSmartManager see a host without smartctl.
func stubSmartctlDetection(t *testing.T) {
	t.Helper()
	original := detectSmartctl
	detectSmartctl = func() (string, error) {
		return "", errors.New("executable file not found in $PATH")
	}
	t.Cleanup(func() { detectSmartctl = original })
}

// When smartctl is missing but sysfs exposes eMMC/mdraid devices, the manager
// degrades to sysfs-only collection and must warn about it (#2736).
func TestNewSmartManagerWarnsWithoutSmartctl(t *testing.T) {
	logs := captureSlog(t)
	stubSmartctlDetection(t)

	original := scanSysfsDevices
	scanSysfsDevices = func() []smartDevice {
		return []smartDevice{{name: "/dev/md0", dtype: "mdraid"}}
	}
	t.Cleanup(func() { scanSysfsDevices = original })

	sm, err := NewSmartManager()
	require.NoError(t, err)
	require.NotNil(t, sm)
	assert.Empty(t, sm.smartctlPath)
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "smartctl not found")
}

// With neither smartctl nor sysfs devices the manager still fails fast, and
// the sysfs-only warning must not fire.
func TestNewSmartManagerNoWarnWithoutSmartctlOrSysfs(t *testing.T) {
	logs := captureSlog(t)
	stubSmartctlDetection(t)

	original := scanSysfsDevices
	scanSysfsDevices = func() []smartDevice { return nil }
	t.Cleanup(func() { scanSysfsDevices = original })

	sm, err := NewSmartManager()
	require.Error(t, err)
	assert.Nil(t, sm)
	assert.NotContains(t, logs.String(), "level=WARN")
}
