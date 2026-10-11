//go:build testing && linux

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSysfs builds a sysfs tree with the given block devices and points
// sysRoot at it. files maps paths below /sys/class/block/<dev> to contents.
func fakeSysfs(t *testing.T, devices map[string]string, files map[string]map[string]string) {
	t.Helper()
	root := t.TempDir()
	for majMin, name := range devices {
		devDir := filepath.Join(root, "devices", "virtual", "block", name)
		require.NoError(t, os.MkdirAll(devDir, 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "dev", "block"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "class", "block"), 0o755))
		require.NoError(t, os.Symlink(filepath.Join("..", "..", "devices", "virtual", "block", name), filepath.Join(root, "dev", "block", majMin)))
		require.NoError(t, os.Symlink(devDir, filepath.Join(root, "class", "block", name)))
		for path, content := range files[name] {
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(devDir, path)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(devDir, path), []byte(content+"\n"), 0o644))
		}
	}
	old := sysRoot
	sysRoot = root
	t.Cleanup(func() { sysRoot = old })
}

func TestBlockDeviceName(t *testing.T) {
	fakeSysfs(t, map[string]string{"259:2": "nvme0n1p2", "253:0": "dm-0"}, nil)

	assert.Equal(t, "nvme0n1p2", blockDeviceName(259, 2))
	assert.Equal(t, "dm-0", blockDeviceName(253, 0))
	assert.Empty(t, blockDeviceName(8, 1))
}

func TestBlockDeviceID(t *testing.T) {
	fakeSysfs(t, map[string]string{
		"259:1": "nvme0n1p1",
		"253:0": "dm-0",
		"9:127": "md127",
		"8:0":   "sda",
		"8:16":  "sdb",
		"7:0":   "loop0",
	}, map[string]map[string]string{
		"nvme0n1p1": {"uevent": "MAJOR=259\nMINOR=1\nDEVNAME=nvme0n1p1\nPARTN=1\nPARTUUID=dde23082-0315-4032-b3c5-416feaf82afa"},
		"dm-0":      {"uevent": "MAJOR=253\nMINOR=0\nDEVNAME=dm-0", "dm/uuid": "LVM-abc123"},
		"md127":     {"md/uuid": "4f3e2d1c:00000000:11111111:22222222"},
		"sda":       {"device/wwid": "naa.500a0751e9b3d01d"},
		"sdb":       {"wwid": "eui.0025385b71b0aa1e"},
	})

	assert.Equal(t, "part:dde23082-0315-4032-b3c5-416feaf82afa", blockDeviceID("nvme0n1p1"))
	assert.Equal(t, "dm:LVM-abc123", blockDeviceID("dm-0"))
	assert.Equal(t, "md:4f3e2d1c:00000000:11111111:22222222", blockDeviceID("md127"))
	assert.Equal(t, "wwid:naa.500a0751e9b3d01d", blockDeviceID("sda"))
	assert.Equal(t, "wwid:eui.0025385b71b0aa1e", blockDeviceID("sdb"))
	assert.Empty(t, blockDeviceID("loop0"))
	assert.Empty(t, blockDeviceID("missing"))
}

func TestStatMountIdentifiesFilesystem(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))

	m := statMount(dir)
	require.NotEmpty(t, m.id)
	// Directories of one filesystem share its identity.
	assert.Equal(t, m.id, statMount(sub).id)
	assert.Equal(t, mountInfo{}, statMount(filepath.Join(dir, "missing")))
}
