package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeVirtFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestDetectVirtualGuest(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"bare metal", map[string]string{
			"proc/cpuinfo":                "flags\t\t: fpu vme sse sse2\n",
			"sys/class/dmi/id/sys_vendor": "ASUSTeK COMPUTER INC.\n",
		}, false},
		{"lxc via systemd", map[string]string{"run/systemd/container": "lxc\n"}, true},
		{"lxc via environ", map[string]string{"proc/1/environ": "PATH=/bin\x00container=lxc\x00"}, true},
		{"docker is not lxc", map[string]string{"run/systemd/container": "docker\n"}, false},
		{"hypervisor cpu flag", map[string]string{"proc/cpuinfo": "flags\t\t: fpu sse2 hypervisor lahf_lm\n"}, true},
		{"hypervisor flag at end", map[string]string{"proc/cpuinfo": "flags\t\t: fpu hypervisor\n"}, true},
		{"proxmox qemu dmi", map[string]string{"sys/class/dmi/id/sys_vendor": "QEMU\n"}, true},
		{"hyper-v dmi", map[string]string{
			"sys/class/dmi/id/sys_vendor":   "Microsoft Corporation\n",
			"sys/class/dmi/id/product_name": "Virtual Machine\n",
		}, true},
		{"microsoft surface is not a vm", map[string]string{
			"sys/class/dmi/id/sys_vendor":   "Microsoft Corporation\n",
			"sys/class/dmi/id/product_name": "Surface Pro\n",
		}, false},
		{"xen dom0 is host", map[string]string{
			"proc/xen/capabilities": "control_d\n",
			"proc/cpuinfo":          "flags\t\t: fpu hypervisor\n",
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tt.files {
				writeVirtFile(t, root, rel, content)
			}
			assert.Equal(t, tt.want, detectVirtualGuest(root))
		})
	}
}

func TestNewSensorConfigVirtualGuest(t *testing.T) {
	orig := isVirtualGuest
	t.Cleanup(func() { isVirtualGuest = orig })
	isVirtualGuest = func() bool { return true }

	t.Run("skips temps and fans by default", func(t *testing.T) {
		config := (&Agent{}).newSensorConfig()
		assert.True(t, config.skipCollection)
		assert.True(t, config.skipFans)
	})

	t.Run("SENSORS does not override", func(t *testing.T) {
		t.Setenv("SENSORS", "*")
		config := (&Agent{}).newSensorConfig()
		assert.True(t, config.skipCollection)
		assert.True(t, config.skipFans)
	})

	t.Run("SYS_SENSORS overrides", func(t *testing.T) {
		t.Setenv("SYS_SENSORS", t.TempDir())
		config := (&Agent{}).newSensorConfig()
		assert.False(t, config.skipCollection)
		assert.False(t, config.skipFans)
	})

	t.Run("updateFans is a no-op", func(t *testing.T) {
		a := &Agent{sensorConfig: &SensorConfig{skipFans: true}}
		var stats system.Stats
		a.updateFans(&stats)
		assert.Nil(t, stats.Fans)
	})
}
