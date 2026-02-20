//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/internal/entities/smart"
)

func TestEmmcMockSysfsScanAndCollect(t *testing.T) {
	tmp := t.TempDir()
	prev := emmcSysfsRoot
	emmcSysfsRoot = tmp
	t.Cleanup(func() { emmcSysfsRoot = prev })

	// Fake: /sys/class/block/mmcblk0
	mmcDeviceDir := filepath.Join(tmp, "class", "block", "mmcblk0", "device")
	mmcQueueDir := filepath.Join(tmp, "class", "block", "mmcblk0", "queue")
	if err := os.MkdirAll(mmcDeviceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mmcQueueDir, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(mmcDeviceDir, "pre_eol_info"), "0x02\n")
	write(filepath.Join(mmcDeviceDir, "life_time"), "0x04 0x05\n")
	write(filepath.Join(mmcDeviceDir, "name"), "H26M52103FMR\n")
	write(filepath.Join(mmcDeviceDir, "serial"), "01234567\n")
	write(filepath.Join(mmcDeviceDir, "prv"), "0x08\n")
	write(filepath.Join(mmcQueueDir, "logical_block_size"), "512\n")
	write(filepath.Join(tmp, "class", "block", "mmcblk0", "size"), "1024\n") // sectors

	devs := scanEmmcDevices()
	if len(devs) != 1 {
		t.Fatalf("scanEmmcDevices() = %d devices, want 1", len(devs))
	}
	if devs[0].Name != "/dev/mmcblk0" || devs[0].Type != "emmc" {
		t.Fatalf("scanEmmcDevices()[0] = %+v, want Name=/dev/mmcblk0 Type=emmc", devs[0])
	}

	sm := &SmartManager{SmartDataMap: map[string]*smart.SmartData{}}
	ok, err := sm.collectEmmcHealth(devs[0])
	if err != nil || !ok {
		t.Fatalf("collectEmmcHealth() = (ok=%v, err=%v), want (true,nil)", ok, err)
	}
	if len(sm.SmartDataMap) != 1 {
		t.Fatalf("SmartDataMap len=%d, want 1", len(sm.SmartDataMap))
	}
	var got *smart.SmartData
	for _, v := range sm.SmartDataMap {
		got = v
		break
	}
	if got == nil {
		t.Fatalf("SmartDataMap value nil")
	}
	if got.DiskType != "emmc" || got.DiskName != "/dev/mmcblk0" {
		t.Fatalf("disk fields = (type=%q name=%q), want (emmc,/dev/mmcblk0)", got.DiskType, got.DiskName)
	}
	if got.SmartStatus != "WARNING" {
		t.Fatalf("SmartStatus=%q, want WARNING", got.SmartStatus)
	}
	if got.SerialNumber != "01234567" || got.ModelName == "" || got.Capacity == 0 {
		t.Fatalf("identity fields = (model=%q serial=%q cap=%d), want non-empty model, serial 01234567, cap>0", got.ModelName, got.SerialNumber, got.Capacity)
	}
	if len(got.Attributes) < 3 {
		t.Fatalf("attributes len=%d, want >= 3", len(got.Attributes))
	}
}
