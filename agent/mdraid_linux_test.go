//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/internal/entities/smart"
)

func TestMdraidMockSysfsScanAndCollect(t *testing.T) {
	tmp := t.TempDir()
	prev := mdraidSysfsRoot
	mdraidSysfsRoot = tmp
	t.Cleanup(func() { mdraidSysfsRoot = prev })

	mdDir := filepath.Join(tmp, "block", "md0", "md")
	queueDir := filepath.Join(tmp, "block", "md0", "queue")
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(mdDir, "array_state"), "active\n")
	write(filepath.Join(mdDir, "level"), "raid1\n")
	write(filepath.Join(mdDir, "raid_disks"), "2\n")
	write(filepath.Join(mdDir, "degraded"), "0\n")
	write(filepath.Join(mdDir, "sync_action"), "resync\n")
	write(filepath.Join(mdDir, "sync_completed"), "10%\n")
	write(filepath.Join(mdDir, "sync_speed"), "100M\n")
	write(filepath.Join(mdDir, "mismatch_cnt"), "0\n")
	write(filepath.Join(queueDir, "logical_block_size"), "512\n")
	write(filepath.Join(tmp, "block", "md0", "size"), "2048\n")

	devs := scanMdraidDevices()
	if len(devs) != 1 {
		t.Fatalf("scanMdraidDevices() = %d devices, want 1", len(devs))
	}
	if devs[0].Name != "/dev/md0" || devs[0].Type != "mdraid" {
		t.Fatalf("scanMdraidDevices()[0] = %+v, want Name=/dev/md0 Type=mdraid", devs[0])
	}

	sm := &SmartManager{SmartDataMap: map[string]*smart.SmartData{}}
	ok, err := sm.collectMdraidHealth(devs[0])
	if err != nil || !ok {
		t.Fatalf("collectMdraidHealth() = (ok=%v, err=%v), want (true,nil)", ok, err)
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
	if got.DiskType != "mdraid" || got.DiskName != "/dev/md0" {
		t.Fatalf("disk fields = (type=%q name=%q), want (mdraid,/dev/md0)", got.DiskType, got.DiskName)
	}
	if got.SmartStatus != "WARNING" {
		t.Fatalf("SmartStatus=%q, want WARNING", got.SmartStatus)
	}
	if got.ModelName == "" || got.Capacity == 0 {
		t.Fatalf("identity fields = (model=%q cap=%d), want non-empty model and cap>0", got.ModelName, got.Capacity)
	}
	if len(got.Attributes) < 5 {
		t.Fatalf("attributes len=%d, want >= 5", len(got.Attributes))
	}
}

func TestMdraidSmartStatus(t *testing.T) {
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "inactive"}); got != "FAILED" {
		t.Fatalf("mdraidSmartStatus(inactive) = %q, want FAILED", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "active", degraded: 1}); got != "FAILED" {
		t.Fatalf("mdraidSmartStatus(degraded) = %q, want FAILED", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "active", syncAction: "recover"}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(recover) = %q, want WARNING", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean"}); got != "PASSED" {
		t.Fatalf("mdraidSmartStatus(clean) = %q, want PASSED", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "unknown"}); got != "UNKNOWN" {
		t.Fatalf("mdraidSmartStatus(unknown) = %q, want UNKNOWN", got)
	}
}
