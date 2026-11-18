package agent

import (
	"errors"
	"math"
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"

	libvirt "github.com/digitalocean/go-libvirt"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/entities/vm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibvirtParamsAndIndexedSum(t *testing.T) {
	params := makeLibvirtParams([]libvirt.TypedParam{
		{Field: "net.count", Value: *libvirt.NewTypedParamValueUint(2)},
		{Field: "net.0.tx.bytes", Value: *libvirt.NewTypedParamValueUllong(100)},
		{Field: "net.1.tx.bytes", Value: *libvirt.NewTypedParamValueUllong(250)},
		{Field: "state.state", Value: *libvirt.NewTypedParamValueInt(1)},
	})
	assert.Equal(t, uint64(350), params.indexedSum("net", ".tx.bytes"))
	assert.Equal(t, uint64(1), params.uint64("state.state"))
	assert.Zero(t, params.uint64("missing"))
}

func TestLibvirtIndexedSumSaturates(t *testing.T) {
	params := libvirtParams{
		"block.count":      {I: uint64(2)},
		"block.0.capacity": {I: uint64(math.MaxUint64)},
		"block.1.capacity": {I: uint64(1)},
	}
	assert.Equal(t, uint64(math.MaxUint64), params.indexedSum("block", ".capacity"))
}

func TestLibvirtCounterRate(t *testing.T) {
	assert.Equal(t, float64(50), counterRate(200, 100, 2*time.Second))
	assert.Zero(t, counterRate(50, 100, time.Second), "a reset must establish a new baseline")
	assert.Zero(t, counterRate(100, 100, 0))
}

func TestLibvirtMissingCountersDoNotCreateResumeSpike(t *testing.T) {
	rate, valid := validCounterRate(200, true, 0, false, time.Second)
	assert.False(t, valid)
	assert.Zero(t, rate, "the first sample after an inactive domain must establish a baseline")

	rate, valid = validCounterRate(300, true, 200, true, time.Second)
	assert.True(t, valid)
	assert.Equal(t, float64(100), rate)
}

func TestSetLibvirtMemoryRequiresFreshBalloonStats(t *testing.T) {
	now := time.Unix(1_000, 0)
	params := libvirtParams{
		"balloon.last-update": {I: uint64(990)},
		"balloon.available":   {I: uint64(4 * 1024)},
		"balloon.usable":      {I: uint64(3 * 1024)},
	}
	stat := &vm.Stats{}
	setLibvirtMemory(stat, params, now)
	assert.True(t, stat.MemoryFresh)
	assert.Equal(t, float64(4), stat.MemoryTotal)
	assert.Equal(t, float64(1), stat.Memory)

	params["balloon.last-update"] = libvirt.TypedParamValue{I: uint64(600)}
	stat = &vm.Stats{}
	setLibvirtMemory(stat, params, now)
	assert.False(t, stat.MemoryFresh)
	assert.Zero(t, stat.MemoryTotal)
	assert.Zero(t, stat.Memory)
}

func TestFormatLibvirtUUID(t *testing.T) {
	uuid := libvirt.UUID{0xd5, 0xcd, 0x3f, 0x23, 0xd5, 0xf5, 0x4e, 0x20, 0x95, 0x8e, 0x84, 0x0f, 0xf0, 0xf3, 0x7a, 0xa9}
	assert.Equal(t, "d5cd3f23-d5f5-4e20-958e-840ff0f37aa9", formatLibvirtUUID(uuid))
}

func TestSanitizedLibvirtURIHidesPassword(t *testing.T) {
	u, err := url.Parse("qemu+ssh://monitor:secret@example/system?socket=/run/libvirt/libvirt-sock-ro")
	assert.NoError(t, err)
	assert.Equal(t, "qemu+ssh://monitor@example/system?socket=/run/libvirt/libvirt-sock-ro", sanitizedLibvirtURI(u))
}

func TestLibvirtConnectionRetriesAfterStartupFailure(t *testing.T) {
	u, err := url.Parse("qemu:///system")
	require.NoError(t, err)
	manager := &libvirtManager{uri: u}
	attempts := 0
	manager.connect = func(*url.URL) (*libvirt.Libvirt, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("libvirt unavailable")
		}
		return &libvirt.Libvirt{}, nil
	}

	require.Error(t, manager.ensureConnected())
	require.NoError(t, manager.ensureConnected())
	assert.Equal(t, 2, attempts)
	assert.NotNil(t, manager.client)
}

func TestNewLibvirtManagerKeepsRetryStateAfterStartupFailure(t *testing.T) {
	for _, key := range []string{"LIBVIRT_URI", "BESZEL_AGENT_LIBVIRT_URI", "BESZEL_AGENT_LIBVIRT_SOCKET"} {
		value, exists := os.LookupEnv(key)
		require.NoError(t, os.Unsetenv(key))
		t.Cleanup(func() {
			if exists {
				require.NoError(t, os.Setenv(key, value))
			}
		})
	}
	t.Setenv("LIBVIRT_SOCKET", "/tmp/beszel-libvirt-does-not-exist.sock")

	manager, err := newLibvirtManager(&Agent{systemDetails: system.Details{Threads: 1}})
	require.Error(t, err)
	require.NotNil(t, manager)
	assert.Nil(t, manager.client)
}

func TestLibvirtLive(t *testing.T) {
	socket := os.Getenv("LIBVIRT_TEST_SOCKET")
	if socket == "" {
		t.Skip("set LIBVIRT_TEST_SOCKET to run against a real libvirt daemon")
	}
	t.Setenv("LIBVIRT_SOCKET", socket)
	manager, err := newLibvirtManager(&Agent{systemDetails: system.Details{Threads: runtime.NumCPU()}})
	require.NoError(t, err)
	if manager == nil {
		t.Fatal("libvirt manager was not enabled")
	}
	defer manager.close()

	first, err := manager.getStats(defaultDataCacheTimeMs)
	assert.NoError(t, err)
	assert.NotEmpty(t, first)
	time.Sleep(100 * time.Millisecond)
	second, err := manager.getStats(defaultDataCacheTimeMs)
	assert.NoError(t, err)
	assert.Len(t, second, len(first))
	for _, machine := range second {
		assert.NotEmpty(t, machine.UUID)
		assert.NotEmpty(t, machine.Name)
	}
}
