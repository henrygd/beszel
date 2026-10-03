//go:build testing && linux

package lvm

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	vgUUID   = "Q34p3SP0kHuKfR0v1aRkFOK6cVBttXev"
	poolUUID = vgUUID + "fFUT2VmoS6HouZ7XEsSgza15P7uQPpeE"
)

// fakeProxmox recreates the block layout of a default Proxmox VE install:
// VG pve on PV nvme0n1p3, pve/data thin pool (dm-4) over _tmeta (dm-2) and
// _tdata (dm-3), the read-only -pool wrapper (dm-5) and one thin volume (dm-6).
// Device contents are served from the returned map, keyed by /dev path.
func fakeProxmox(t *testing.T) (contents map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	oldSys, oldDev, oldOpen := sysClassBlock, devPath, openDevice
	t.Cleanup(func() { sysClassBlock, devPath, openDevice = oldSys, oldDev, oldOpen })
	sysClassBlock, devPath = filepath.Join(root, "sys"), "/dev"
	contents = map[string][]byte{"/dev/dm-2": readFixture(t), "/dev/nvme0n1p3": readPVFixture(t)}
	openDevice = func(path string) (deviceReader, error) {
		b, ok := contents[path]
		if !ok {
			return nil, os.ErrPermission
		}
		return nopCloser{bytes.NewReader(b)}, nil
	}

	dm := func(dev, name, uuid string, slaves ...string) {
		dir := filepath.Join(sysClassBlock, dev)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dm"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "slaves"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dm", "name"), []byte(name+"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dm", "uuid"), []byte(uuid+"\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dm", "suspended"), []byte("0\n"), 0o644))
		for _, slave := range slaves {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "slaves", slave), nil, 0o644))
		}
	}
	dm("dm-1", "pve-root", "LVM-"+vgUUID+"yhUCOwUSjTeriwcroTN6a7WcU01cH1vB", "nvme0n1p3")
	dm("dm-2", "pve-data_tmeta", "LVM-"+vgUUID+"0y6ARL9e0lexk2VdyXmeGZgpHH8jT1Gc-tmeta", "nvme0n1p3")
	dm("dm-3", "pve-data_tdata", "LVM-"+vgUUID+"0a9i4Dc9ndn4PeAq5hV25Ovwx2K2JJ6a-tdata", "nvme0n1p3")
	dm("dm-4", "pve-data-tpool", "LVM-"+poolUUID+"-tpool", "dm-2", "dm-3")
	dm("dm-5", "pve-data", "LVM-"+poolUUID+"-pool", "dm-4")
	dm("dm-6", "pve-vm--100--disk--0", "LVM-"+vgUUID+"O7aIJyLY81ejO88U2wtUi0Zvt8bkFAy1", "dm-4")
	stat := func(dev, content string) {
		require.NoError(t, os.MkdirAll(filepath.Join(sysClassBlock, dev), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(sysClassBlock, dev, "stat"), []byte(content), 0o644))
	}
	stat("dm-2", "10 0 8 0 20 0 16 0 0 0 0\n")
	stat("dm-3", "10 0 100 0 20 0 200 0 0 0 0\n")
	stat("nvme0n1p3", "10 0 1000 0 20 0 2000 0 0 0 0\n")
	stat("nvme0n1", "10 0 1000 0 20 0 2000 0 0 0 0\n")
	return contents
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

var pveVG = Pool{
	UUID: vgUUID, Name: "pve", Raw: true, Health: "ONLINE",
	Size: 499029901312, Alloc: 499029901312 - 17184063488,
	NRead: 1000 * 512, NWrite: 2000 * 512,
	Devices: []Device{{Name: "/dev/nvme0n1p3", State: "ONLINE"}},
}

func TestPools(t *testing.T) {
	fakeProxmox(t)

	pools, err := Pools()
	require.NoError(t, err)
	require.Len(t, pools, 2, "the -pool wrapper must not be counted twice")
	assert.Equal(t, pveVG, pools[0])
	assert.Equal(t, Pool{
		UUID:         poolUUID,
		Name:         "pve/data",
		Health:       "ONLINE",
		Size:         362777935872,
		Alloc:        214696 * 65536,
		MetadataSize: 903168 * 4096,
		MetadataUsed: (1589 + 4096) * 4096,
		NRead:        108 * 512,
		NWrite:       216 * 512,
		Devices: []Device{
			{Name: "pve/data_tmeta", State: "ONLINE"},
			{Name: "pve/data_tdata", State: "ONLINE"},
		},
	}, pools[1])
}

func TestThinPoolsHealth(t *testing.T) {
	t.Run("needs check", func(t *testing.T) {
		contents := fakeProxmox(t)
		b := contents["/dev/dm-2"]
		binary.LittleEndian.PutUint32(b[offFlags:], needsCheckFlag)
		reseal(b)

		pools, err := Pools()
		require.NoError(t, err)
		require.Len(t, pools, 2)
		assert.Equal(t, "DEGRADED", pools[1].Health)
	})

	t.Run("data full", func(t *testing.T) {
		contents := fakeProxmox(t)
		b := contents["/dev/dm-2"]
		binary.LittleEndian.PutUint64(b[offDataSpaceMap+8:], binary.LittleEndian.Uint64(b[offDataSpaceMap:]))
		reseal(b)

		pools, err := Pools()
		require.NoError(t, err)
		require.Len(t, pools, 2)
		assert.Equal(t, "FULL", pools[1].Health)
	})
}

func TestPoolsUnreadableDevices(t *testing.T) {
	contents := fakeProxmox(t)
	delete(contents, "/dev/dm-2")

	pools, err := Pools()
	require.NoError(t, err, "a permission problem is not a transient failure")
	assert.Equal(t, []Pool{pveVG}, pools)

	delete(contents, "/dev/nvme0n1p3")
	pools, err = Pools()
	require.NoError(t, err)
	assert.Empty(t, pools)
}

func TestPoolsSuspended(t *testing.T) {
	fakeProxmox(t)
	require.NoError(t, os.WriteFile(filepath.Join(sysClassBlock, "dm-2", "dm", "suspended"), []byte("1\n"), 0o644))

	_, err := Pools()
	assert.ErrorIs(t, err, errSuspended, "the previous inventory must be kept")
}

func TestVolumeGroupsNewestCopyWins(t *testing.T) {
	contents := fakeProxmox(t)
	// A second PV of the same VG carrying an older copy of the metadata.
	contents["/dev/sdb"] = contents["/dev/nvme0n1p3"]
	dir := filepath.Join(sysClassBlock, "sdb")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("1 0 1 0 1 0 1 0 0 0 0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sysClassBlock, "dm-1", "slaves", "sdb"), nil, 0o644))
	older := bytes.Replace(readPVFixture(t), []byte("seqno = 55"), []byte("seqno = 54"), 1)
	older = bytes.Replace(older, []byte("pe_count = 118978"), []byte("pe_count = 100000"), 1)
	text := older[fixtureMda+mdaHeaderSize:]
	contents["/dev/sdb"] = setMetadata(older, 0xff000, mdaHeaderSize, text)

	pools, err := Pools()
	require.NoError(t, err)
	require.Len(t, pools, 2)
	assert.Equal(t, pveVG.Size, pools[0].Size)
	assert.Equal(t, pveVG.NRead+512, pools[0].NRead, "I/O covers every PV of the VG")
}

func TestPoolsNoSysfs(t *testing.T) {
	old := sysClassBlock
	t.Cleanup(func() { sysClassBlock = old })
	sysClassBlock = filepath.Join(t.TempDir(), "missing")

	pools, err := Pools()
	require.NoError(t, err)
	assert.Nil(t, pools)
}

func TestDecodeName(t *testing.T) {
	for in, want := range map[string]string{
		"pve-data":             "pve/data",
		"pve-data_tmeta":       "pve/data_tmeta",
		"pve-vm--100--disk--0": "pve/vm-100-disk-0",
		"my--vg-thin--pool":    "my-vg/thin-pool",
		"vg1-volume_1":         "vg1/volume_1",
		"noseparator":          "noseparator",
	} {
		assert.Equal(t, want, decodeName(in), in)
	}
}

func TestLvmUUID(t *testing.T) {
	id, ok := lvmUUID("LVM-"+poolUUID+"-tpool", "-tpool")
	assert.True(t, ok)
	assert.Equal(t, poolUUID, id)

	_, ok = lvmUUID("LVM-"+poolUUID+"-pool", "-tpool")
	assert.False(t, ok)
	_, ok = lvmUUID("CRYPT-LUKS2-abc-tpool", "-tpool")
	assert.False(t, ok)
	_, ok = lvmUUID("LVM-short-tpool", "-tpool")
	assert.False(t, ok)
}

func TestOpenDirect(t *testing.T) {
	// tmpfs rejects O_DIRECT; only check that errors surface.
	_, err := openDirect(filepath.Join(t.TempDir(), "missing"))
	assert.True(t, errors.Is(err, os.ErrNotExist))
}
