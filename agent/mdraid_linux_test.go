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

	// Simulate two healthy member devices (no faulty state).
	for _, dev := range []string{"dev-sda", "dev-sdb"} {
		devPath := filepath.Join(mdDir, dev)
		if err := os.MkdirAll(devPath, 0o755); err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(devPath, "state"), "in_sync\n")
	}
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

func TestCountMdraidMemberStates(t *testing.T) {
	tmp := t.TempDir()

	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mdDir := filepath.Join(tmp, "block", "md0", "md")

	// No dev-* entries: zero faulty, zero populated.
	if faulty, populated := countMdraidMemberStates("md0", tmp); faulty != 0 || populated != 0 {
		t.Fatalf("no members: got (faulty=%d populated=%d), want (0,0)", faulty, populated)
	}

	// Two healthy members.
	write(filepath.Join(mdDir, "dev-sda", "state"), "in_sync\n")
	write(filepath.Join(mdDir, "dev-sdb", "state"), "in_sync\n")
	if faulty, populated := countMdraidMemberStates("md0", tmp); faulty != 0 || populated != 2 {
		t.Fatalf("all in_sync: got (faulty=%d populated=%d), want (0,2)", faulty, populated)
	}

	// One faulty member.
	write(filepath.Join(mdDir, "dev-sdb", "state"), "faulty\n")
	if faulty, populated := countMdraidMemberStates("md0", tmp); faulty != 1 || populated != 2 {
		t.Fatalf("one faulty: got (faulty=%d populated=%d), want (1,2)", faulty, populated)
	}

	// QNAP-style: 28 degraded slots but no dev-* entries for them, 4 in_sync.
	write(filepath.Join(mdDir, "dev-sdb", "state"), "in_sync\n")
	write(filepath.Join(mdDir, "dev-sdc", "state"), "in_sync\n")
	write(filepath.Join(mdDir, "dev-sdd", "state"), "in_sync\n")
	if faulty, populated := countMdraidMemberStates("md0", tmp); faulty != 0 || populated != 4 {
		t.Fatalf("qnap sparse: got (faulty=%d populated=%d), want (0,4)", faulty, populated)
	}
}

func TestMdraidSmartStatus(t *testing.T) {
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "inactive"}); got != "FAILED" {
		t.Fatalf("mdraidSmartStatus(inactive) = %q, want FAILED", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "active", degraded: 1, faultyDisks: 1, syncAction: "recover"}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(degraded+recover) = %q, want WARNING", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "active", degraded: 1, faultyDisks: 1}); got != "FAILED" {
		t.Fatalf("mdraidSmartStatus(degraded+faulty) = %q, want FAILED", got)
	}
	// QNAP-style: raid_disks=32 but only 4 populated; degraded=28 but no faulty devices.
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", degraded: 28, faultyDisks: 0, raidDisks: 32, populatedDisks: 4}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(qnap sparse) = %q, want WARNING", got)
	}
	// A member disappearing from the same sparse array is indistinguishable
	// from another reserved slot, so it must not be reported as healthy.
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", degraded: 29, faultyDisks: 0, raidDisks: 32, populatedDisks: 3}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(qnap sparse missing member) = %q, want WARNING", got)
	}
	// A genuinely missing member (removed dev-* entry, not just an unpopulated
	// QNAP reserve slot) must still fail: raid_disks=4, only 3 populated, all
	// of them in_sync, so faultyDisks==0 but degraded==1.
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", degraded: 1, faultyDisks: 0, raidDisks: 4, populatedDisks: 3}); got != "FAILED" {
		t.Fatalf("mdraidSmartStatus(missing member) = %q, want FAILED", got)
	}
	// Degraded with no member-state info at all (e.g. sysfs read failed) must
	// still fail rather than being silently treated as a sparse QNAP array.
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", degraded: 1, faultyDisks: 0, raidDisks: 4, populatedDisks: 0}); got != "FAILED" {
		t.Fatalf("mdraidSmartStatus(degraded, no member info) = %q, want FAILED", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "active", syncAction: "recover"}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(recover) = %q, want WARNING", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", syncAction: "check"}); got != "PASSED" {
		t.Fatalf("mdraidSmartStatus(clean+check) = %q, want PASSED", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", syncAction: "check", mismatchCnt: 1}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(clean+check+mismatch) = %q, want WARNING", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", mismatchCnt: 1}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(clean+mismatch) = %q, want WARNING", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean", syncAction: "repair"}); got != "WARNING" {
		t.Fatalf("mdraidSmartStatus(repair) = %q, want WARNING", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "clean"}); got != "PASSED" {
		t.Fatalf("mdraidSmartStatus(clean) = %q, want PASSED", got)
	}
	if got := mdraidSmartStatus(mdraidHealth{arrayState: "unknown"}); got != "UNKNOWN" {
		t.Fatalf("mdraidSmartStatus(unknown) = %q, want UNKNOWN", got)
	}
}
