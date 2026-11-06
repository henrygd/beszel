package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAndroidCpuFallback simulates an unrooted Android environment where
// /proc/stat is unreadable but process ticks and cpufreq time_in_state work.
func TestAndroidCpuFallback(t *testing.T) {
	tmp := t.TempDir()

	// Fixture 1: two CPUs with time_in_state files (arbitrary units).
	sysRoot := filepath.Join(tmp, "sys")
	writeTis := func(lo0, hi0, lo1, hi1 int) {
		vals := [][2]int{{lo0, hi0}, {lo1, hi1}}
		for i, v := range vals {
			dir := filepath.Join(sysRoot, "cpu"+string(rune('0'+i)))
			if err := os.MkdirAll(filepath.Join(dir, "cpufreq", "stats"), 0o755); err != nil {
				t.Fatal(err)
			}
			content := "800000 " + itoa(v[0]) + "\n1800000 " + itoa(v[1]) + "\n"
			if err := os.WriteFile(filepath.Join(dir, "cpufreq", "stats", "time_in_state"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Initial state (baseline is re-seeded later, values are arbitrary).
	writeTis(600, 400, 1200, 800)

	// Fixture 2: a readable /proc/<pid>/stat with utime=300 stime=100.
	procRoot := filepath.Join(tmp, "proc")
	pidDir := filepath.Join(procRoot, "1234")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statLine := "1234 (bash) R 1 1 1 0 0 0 0 0 0 0 0 300 100 0 0 0 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(pidDir, "stat"), []byte(statLine), 0o644); err != nil {
		t.Fatal(err)
	}

	// /proc/stat unreadable.
	if err := os.WriteFile(filepath.Join(procRoot, "stat"), []byte(""), 0o000); err != nil {
		t.Fatal(err)
	}

	// Swap in fixture paths / fake clock.
	oldStatPath, oldSysRoot, oldProcRoot := androidProcStatPath, androidSysCpuRoot, androidProcPidRoot
	oldNow := androidClockNow
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tick := 0
	androidClockNow = func() time.Time {
		tick++
		return baseTime.Add(time.Duration(tick) * time.Minute)
	}
	resetState := func() {
		androidMu.Lock()
		androidTotal = androidCpuState{}
		androidPerCore = nil
		androidLastRaw = androidRawSamples{}
		androidCumBusy = nil
		androidMu.Unlock()
	}
	defer func() {
		androidProcStatPath, androidSysCpuRoot, androidProcPidRoot = oldStatPath, oldSysRoot, oldProcRoot
		androidClockNow = oldNow
		resetState()
	}()
	androidProcStatPath, androidSysCpuRoot, androidProcPidRoot = filepath.Join(procRoot, "stat"), sysRoot, procRoot

	// ---- Phase 1: process ticks are the primary source ----
	// First call seeds the baseline (empty result).
	first, err := androidCpuTimes(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("expected empty baseline, got %d", len(first))
	}

	// Advance process ticks? No — the fixture pid stat is static; utime+stime
	// stays 400 jiffies = 4s busy. One minute later (wall = 60s x 2 cores):
	second, err := androidCpuTimes(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("expected 1 aggregate, got %d", len(second))
	}
	cur := second[0]
	if cur.User <= 0 || cur.Idle <= 0 {
		t.Fatalf("expected positive busy/idle, got %+v", cur)
	}
	total := cur.User + cur.Idle
	pct := cur.User / total * 100
	// 4s busy of 120s wall = ~3.33%. The cpufreq fixture below (40% busy)
	// must NOT leak into this number, proving process ticks win.
	if pct < 2 || pct > 5 {
		t.Fatalf("expected ~3.3%% busy from process ticks, got %.2f%% (user=%.2fs idle=%.2fs)", pct, cur.User, cur.Idle)
	}

	// Per-core: two cores. First call seeds the baseline.
	if _, err := androidCpuTimes(true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	perCore, err := androidCpuTimes(true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(perCore) != 2 {
		t.Fatalf("expected 2 cores, got %d", len(perCore))
	}

	// ---- Phase 2: cpufreq residency-ratio fallback ----
	// Hide the /proc pid fixture (keep /proc/stat unreadable) so only
	// cpufreq remains. Advance counters: +1000 total per core, of which
	// +600 at the lowest frequency => 40% busy over the interval.
	procBackup := androidProcPidRoot
	androidProcPidRoot = filepath.Join(tmp, "missing-proc")
	tick = 0 // reset clock
	resetState()
	if _, err := androidCpuTimes(false); err != nil {
		t.Fatalf("cpufreq fallback failed: %v", err)
	}
	writeTis(1200, 800, 1800, 1200)
	fbTimes, err := androidCpuTimes(false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fbTimes) != 1 {
		t.Fatalf("expected 1 aggregate, got %d", len(fbTimes))
	}
	fb := fbTimes[0]
	if fb.User <= 0 || fb.Idle <= 0 {
		t.Fatalf("expected positive busy/idle, got %+v", fb)
	}
	fbPct := fb.User / (fb.User + fb.Idle) * 100
	if fbPct < 35 || fbPct > 45 {
		t.Fatalf("expected ~40%% busy from cpufreq ratios, got %.1f%% (user=%.1fs idle=%.1fs)", fbPct, fb.User, fb.Idle)
	}
	androidProcPidRoot = procBackup
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
