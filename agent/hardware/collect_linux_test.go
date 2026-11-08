//go:build linux

package hardware

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRoot builds a minimal sysfs/procfs tree describing one desktop.
func fakeRoot(t *testing.T) (root, smbios string) {
	root = t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	link := func(target, name string) {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.Symlink(target, path))
	}

	write("sys/class/dmi/id/board_vendor", "Gigabyte\n")
	write("sys/class/dmi/id/product_serial", "SN-ROOT-ONLY\n")
	write("proc/cpuinfo", "processor\t: 0\nvendor_id\t: AuthenticAMD\nmodel name\t: Ryzen\nphysical id\t: 0\ncpu cores\t: 1\n\n"+
		"processor\t: 1\nvendor_id\t: AuthenticAMD\nmodel name\t: Ryzen\nphysical id\t: 0\ncpu cores\t: 1\n")

	pci := "sys/devices/pci0000:00/0000:00:02.0/"
	write(pci+"vendor", "0x10de\n")
	write(pci+"device", "0x2484\n")
	write(pci+"class", "0x030000\n")
	link("../../../bus/pci/drivers/nvidia", pci+"driver")
	link("../../../devices/pci0000:00/0000:00:02.0", "sys/bus/pci/devices/0000:00:02.0")

	write("sys/bus/usb/devices/1-1/idVendor", "1b1c\n")
	write("sys/bus/usb/devices/1-1/idProduct", "1b4f\n")
	write("sys/bus/usb/devices/1-1/product", "K68 Keyboard\n")
	write("sys/bus/usb/devices/1-1/serial", "KB-SERIAL\n")
	write("sys/bus/usb/devices/1-1:1.0/bInterfaceClass", "03\n")
	write("sys/bus/usb/devices/1-1:1.0/bInterfaceProtocol", "01\n")
	write("sys/bus/usb/devices/usb1/idVendor", "1d6b\n") // root hub, skipped

	write("sys/devices/pci0000:00/ata1/host0/block/sda/size", "1953525168\n")
	write("sys/devices/pci0000:00/ata1/host0/block/sda/queue/rotational", "1\n")
	write("sys/devices/pci0000:00/ata1/host0/block/sda/device/model", "WDC WD10\n")
	link("../devices/pci0000:00/ata1/host0/block/sda", "sys/block/sda")
	write("sys/devices/virtual/block/loop0/size", "8\n")
	link("../devices/virtual/block/loop0", "sys/block/loop0") // no device link, skipped

	write("sys/class/net/eth0/address", "aa:bb:cc:dd:ee:ff\n")
	write("sys/class/net/eth0/speed", "2500\n")
	link("../../../devices/pci0000:00/0000:00:02.0", "sys/class/net/eth0/device")
	write("sys/class/net/lo/address", "00:00:00:00:00:00\n") // virtual, skipped

	edid := sampleEDID()
	write("sys/class/drm/card0-DP-1/status", "connected\n")
	write("sys/class/drm/card0-DP-1/edid", string(edid))
	write("sys/class/drm/card0-DP-2/status", "disconnected\n")

	write("sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c", "\x06\x00\x00\x00\x01")
	write("sys/class/tpm/tpm0/tpm_version_major", "2\n")
	write("sys/class/power_supply/BAT0/type", "Battery\n")
	write("sys/class/power_supply/BAT0/energy_full", "45000000\n")
	write("sys/class/power_supply/BAT0/energy_full_design", "50000000\n")
	write("sys/class/power_supply/hidpp_battery_0/type", "Battery\n")
	write("sys/class/power_supply/hidpp_battery_0/scope", "Device\n") // mouse, skipped

	smbios = filepath.Join(root, "smbios")
	require.NoError(t, os.WriteFile(smbios, sampleTable(), 0o600))
	return root, smbios
}

func TestCollectLinux(t *testing.T) {
	root, smbios := fakeRoot(t)
	inv := Collect(Options{Root: root, SMBIOS: smbios, Identifiers: true})

	assert.Empty(t, inv.Unavailable)
	assert.Equal(t, "Gigabyte", inv.System.Vendor, "SMBIOS wins over DMI attributes")
	assert.Equal(t, "SN123", inv.System.Serial)
	require.Len(t, inv.CPUs, 1)
	assert.Equal(t, 2, inv.CPUs[0].Threads)
	require.Len(t, inv.Memory, 2)

	require.Len(t, inv.PCI, 1)
	assert.Equal(t, "nvidia", inv.PCI[0].Driver)
	assert.Equal(t, "10de", inv.PCI[0].Vendor)
	require.Len(t, inv.USB, 1)
	assert.Equal(t, []string{"keyboard"}, inv.USB[0].Kinds)
	require.Len(t, inv.Disks, 1)
	assert.Equal(t, uint64(1953525168*512), inv.Disks[0].SizeBytes)
	assert.Equal(t, "sata", inv.Disks[0].Transport)
	require.Len(t, inv.NICs, 1)
	assert.Equal(t, "0000:00:02.0", inv.NICs[0].PCI)
	require.Len(t, inv.Displays, 1)
	assert.Equal(t, "DP-1", inv.Displays[0].Connector)

	assert.Equal(t, "enabled", inv.Security.SecureBoot)
	assert.Equal(t, "2.0", inv.Security.TPM)
	require.Len(t, inv.Batteries, 1)
	assert.Equal(t, 90, inv.Batteries[0].HealthPct)
}

func TestCollectLinuxDefaults(t *testing.T) {
	root, _ := fakeRoot(t)
	inv := Collect(Options{Root: root, SMBIOS: filepath.Join(root, "missing")})

	assert.Equal(t, "memory", inv.Unavailable[0].Section, "a missing SMBIOS table is reported, not fatal")
	assert.Equal(t, "Gigabyte", inv.Board.Vendor, "DMI attributes fill in without SMBIOS")
	assert.Empty(t, inv.System.Serial+inv.USB[0].Serial+inv.NICs[0].MAC, "identifiers are off by default")
}
