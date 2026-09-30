//go:build windows

package agent

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/henrygd/beszel/internal/entities/smart"
	"golang.org/x/sys/windows"
)

// resetAmdRc2 clears the load-once state so each test starts fresh.
func resetAmdRc2(t *testing.T) {
	t.Helper()
	amdRc2.once = sync.Once{}
	amdRc2.ok = false
	t.Cleanup(func() {
		amdRc2.once = sync.Once{}
		amdRc2.ok = false
	})
}

func newTestSmartManager() *SmartManager {
	return &SmartManager{SmartDataMap: map[string]*smart.SmartData{}}
}

func TestAmdRaidWithoutDll(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"missing file": func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "dll", amdRc2DllName)
		},
		"unsigned file": func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "dll", amdRc2DllName)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("not a dll"), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		},
	}
	for name, dllPath := range cases {
		t.Run(name, func(t *testing.T) {
			resetAmdRc2(t)
			t.Setenv("AMD_RC2_DLL", dllPath(t))

			if devices := scanAmdRaidDevices(); len(devices) != 0 {
				t.Fatalf("expected no devices without a usable DLL, got %d", len(devices))
			}
			if amdRc2.ok {
				t.Fatal("DLL must not be marked loaded")
			}

			sm := newTestSmartManager()
			// An amdraid device (e.g. from SMART_DEVICES) is handled with an error, not a crash.
			handled, err := sm.collectAmdRaidHealth(&DeviceInfo{Name: "amdraid0", Type: amdRaidPrefix})
			if !handled || err == nil {
				t.Fatalf("expected handled error, got handled=%v err=%v", handled, err)
			}
			// Normal disks fall through to smartctl untouched.
			if handled, err := sm.collectAmdRaidHealth(&DeviceInfo{Name: "/dev/sda", Type: "sat"}); handled || err != nil {
				t.Fatalf("non-AMD device should not be handled, got handled=%v err=%v", handled, err)
			}
			if err := sm.CollectSmart(&DeviceInfo{Name: "amdraid0", Type: amdRaidPrefix}); err == nil {
				t.Fatal("CollectSmart should report an error for amdraid without the DLL")
			}
			if len(sm.SmartDataMap) != 0 {
				t.Fatalf("no data expected, got %d entries", len(sm.SmartDataMap))
			}
		})
	}
}

// TestAmdRaidWithDll runs against real hardware. It needs AMD RAIDXpert2 >= 9.3,
// admin rights, and AMD_RC2t7x64.dll from CrystalDiskInfo (or AMD_RC2_TEST_DLL).
func TestAmdRaidWithDll(t *testing.T) {
	src := os.Getenv("AMD_RC2_TEST_DLL")
	if src == "" {
		src = `C:\Program Files\CrystalDiskInfo\CdiResource\dll\` + amdRc2DllName
	}
	if _, err := os.Stat(src); err != nil {
		t.Skipf("AMD RAID DLL not found at %s", src)
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("requires administrator")
	}
	if !rcraidVersionOK() {
		t.Skip("AMD RAIDXpert2 driver >= 9.3 not present")
	}

	// The DLL only initializes from a subfolder of the host exe, which is also
	// the agent's default lookup path.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(filepath.Dir(exe), "dll", amdRc2DllName)
	copyFile(t, src, dst)

	resetAmdRc2(t)
	t.Setenv("AMD_RC2_DLL", "") // use the default <exe dir>\dll path

	devices := scanAmdRaidDevices()
	if !amdRc2.ok {
		t.Fatal("DLL failed to load")
	}
	if len(devices) == 0 {
		t.Skip("DLL loaded but no AMD RAID member drives found")
	}

	sm := newTestSmartManager()
	for _, d := range devices {
		if err := sm.CollectSmart(d); err != nil {
			t.Errorf("%s: %v", d.Name, err)
		}
	}
	if len(sm.SmartDataMap) != len(devices) {
		t.Fatalf("collected %d drives, scanned %d", len(sm.SmartDataMap), len(devices))
	}
	for key, data := range sm.SmartDataMap {
		t.Logf("%s %s %s %d°C %s", data.DiskName, data.ModelName, key, data.Temperature, data.SmartStatus)
		if data.ModelName == "" || data.SerialNumber == "" || data.Capacity == 0 || len(data.Attributes) == 0 {
			t.Errorf("%s: incomplete data %+v", data.DiskName, data)
		}
		if data.DiskType != "nvme" && data.DiskType != "sat" {
			t.Errorf("%s: unexpected disk type %q", data.DiskName, data.DiskType)
		}
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
