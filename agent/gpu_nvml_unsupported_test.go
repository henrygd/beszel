//go:build testing && !(amd64 && (windows || (linux && glibc)))

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This fallback requires NVML initialisation to fail, as guaranteed by the unsupported implementation.
func TestNewGPUManagerPriorityNvmlFallbackToNvidiaSmi(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("BESZEL_AGENT_GPU_COLLECTOR", "nvml,nvidia-smi")

	gpuCommandFixture(t, dir, "nvidia-smi", `0, NVIDIA Fallback GPU, 41, 256, 1024, 8, 14`+"\n")

	gm, err := NewGPUManager()
	require.NoError(t, err)
	require.NotNil(t, gm)

	waitGPUs(t, gm, "0")
	gm.Lock()
	defer gm.Unlock()
	gpu, ok := gm.GpuDataMap["0"]
	require.True(t, ok)
	assert.Equal(t, "Fallback GPU", gpu.Name)
}
