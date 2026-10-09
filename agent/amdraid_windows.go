//go:build windows

package agent

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/smart"
	"golang.org/x/sys/windows"
)

// AMD RAIDXpert2 hides member drives from smartctl. CrystalDiskInfo's
// AMD_RC2t7x64.dll can read them. Users copy it from CrystalDiskInfo into a
// "dll" folder beside beszel-agent.exe (or point AMD_RC2_DLL at it). The DLL
// refuses to init (name_failed) unless it lives in a subfolder of the host exe.
const amdRc2DllName = "AMD_RC2t7x64.dll"

var amdRc2 struct {
	once sync.Once
	mu   sync.Mutex // serializes DLL calls
	ok   bool

	getDrives, reload, getIdentify, getSmartData *windows.LazyProc
}

var amdRc2InitStatus = []string{
	"uninitial", "loaded", "unloaded", "failed_signature", "driver_not_found",
	"cannot_open", "failed_memory_alloc", "offset_overflow", "driver_version_old",
	"not_admin", "name_failed",
}

func amdRc2DllPath() string {
	if path, ok := utils.GetEnv("AMD_RC2_DLL"); ok && path != "" {
		return path
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), "dll", amdRc2DllName)
}

// rcraidVersionOK reports whether the AMD RAID driver is >= 9.3. Older drivers
// (9.2.0.x with NVMe arrays) are known to bluescreen when queried.
func rcraidDriverPath() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "rcraid.sys")
}

func rcraidVersionOK() bool {
	path := rcraidDriverPath()
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return false
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return false
	}
	var info *windows.VS_FIXEDFILEINFO
	var infoLen uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&info), &infoLen); err != nil || info == nil {
		return false
	}
	major, minor := info.FileVersionMS>>16, info.FileVersionMS&0xffff
	return major > 9 || (major == 9 && minor >= 3)
}

// verifySignature checks the file carries a valid Authenticode signature.
// ponytail: any trusted signer is accepted; pin the "Gakuto Matsumura" subject
// (needs WTHelper* APIs missing from x/sys) if stricter checks are wanted.
func verifySignature(path string) error {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	file := &windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: path16}
	data := &windows.WinTrustData{
		Size:                            uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:                        windows.WTD_UI_NONE,
		RevocationChecks:                windows.WTD_REVOKE_NONE,
		UnionChoice:                     windows.WTD_CHOICE_FILE,
		StateAction:                     windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(file),
	}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	return err
}

func loadAmdRc2() bool {
	amdRc2.once.Do(func() {
		path := amdRc2DllPath()
		if path == "" {
			return
		}
		if _, err := os.Stat(rcraidDriverPath()); err != nil {
			return // no AMD RAID driver on this machine
		}
		if _, err := os.Stat(path); err != nil {
			// Path is relative to the exe as launched; for WinGet installs that is
			// the WinGet\Links symlink folder, not the package folder.
			slog.Info("AMD RAID driver found; copy AMD_RC2t7x64.dll from CrystalDiskInfo here to read member drives", "path", path)
			return
		}
		if !rcraidVersionOK() {
			slog.Warn("AMD RAID: rcraid driver older than 9.3, skipping", "dll", path)
			return
		}
		if err := verifySignature(path); err != nil {
			slog.Warn("AMD RAID: DLL signature invalid", "path", path, "err", err)
			return
		}

		dll := windows.NewLazyDLL(path)
		initProc := dll.NewProc("AMD_RC2_Init")
		amdRc2.getDrives = dll.NewProc("AMD_RC2_GetDrives")
		amdRc2.reload = dll.NewProc("AMD_RC2_Reload")
		amdRc2.getIdentify = dll.NewProc("AMD_RC2_GetIdentify")
		amdRc2.getSmartData = dll.NewProc("AMD_RC2_GetSmartData")
		for _, p := range []*windows.LazyProc{initProc, amdRc2.getDrives, amdRc2.reload, amdRc2.getIdentify, amdRc2.getSmartData} {
			if err := p.Find(); err != nil {
				slog.Warn("AMD RAID: DLL load failed", "path", path, "err", err)
				return
			}
		}

		status, _, _ := initProc.Call()
		if status != 1 {
			name := "unknown"
			if int(status) < len(amdRc2InitStatus) {
				name = amdRc2InitStatus[status]
			}
			exe, _ := os.Executable()
			slog.Warn("AMD RAID: init failed (DLL must be in a subfolder of the agent exe as launched)", "status", name, "exe", exe, "dll", path)
			return
		}
		amdRc2.ok = true
		slog.Debug("AMD RAID DLL loaded", "path", path)
	})
	return amdRc2.ok
}

// amdRc2GetIdentify must be called with amdRc2.mu held.
func amdRc2GetIdentify(idx int) (amdRc2Identify, bool) {
	id := amdRc2Identify{StructSize: uint32(unsafe.Sizeof(amdRc2Identify{})), StructVersion: 1, DiskNum: uint32(idx)}
	r, _, _ := amdRc2.getIdentify.Call(uintptr(unsafe.Pointer(&id)))
	return id, r != 0
}

// scanAmdRaidDevices lists member drives of AMD RAIDXpert2 arrays.
func scanAmdRaidDevices() []*DeviceInfo {
	if !loadAmdRc2() {
		return nil
	}
	amdRc2.mu.Lock()
	defer amdRc2.mu.Unlock()

	amdRc2.reload.Call()
	n, _, _ := amdRc2.getDrives.Call()
	devices := make([]*DeviceInfo, 0, n)
	for i := range int(n) {
		id, ok := amdRc2GetIdentify(i)
		if !ok {
			continue
		}
		protocol := "ATA"
		if id.IsNVMe != 0 {
			protocol = "NVMe"
		}
		name := amdRaidDeviceName(i)
		devices = append(devices, &DeviceInfo{
			Name:     name,
			Type:     amdRaidPrefix,
			InfoName: name + " [" + cString(id.Model[:]) + "]",
			Protocol: protocol,
		})
	}
	return devices
}

// collectAmdRaidHealth reads SMART data for an AMD RAID member drive.
func (sm *SmartManager) collectAmdRaidHealth(deviceInfo *DeviceInfo) (bool, error) {
	if deviceInfo == nil {
		return false, nil
	}
	idx, ok := amdRaidIndex(deviceInfo.Name)
	if !ok {
		return false, nil
	}
	if !loadAmdRc2() {
		return true, errors.New("AMD RAID DLL not available")
	}

	amdRc2.mu.Lock()
	id, ok := amdRc2GetIdentify(idx)
	data := make([]byte, 512)
	thresholds := make([]byte, 512)
	var r uintptr
	if ok {
		r, _, _ = amdRc2.getSmartData.Call(uintptr(idx),
			uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
			uintptr(unsafe.Pointer(&thresholds[0])), uintptr(len(thresholds)))
	}
	amdRc2.mu.Unlock()
	if r == 0 {
		return true, errNoValidSmartData
	}

	var (
		temp     uint8
		passed   bool
		attrs    []*smart.SmartAttribute
		diskType = "sat"
	)
	if id.IsNVMe != 0 {
		diskType = "nvme"
		temp, passed, attrs = parseAmdRc2Nvme(data)
	} else {
		temp, passed, attrs = parseAmdRc2Ata(data, thresholds)
	}
	if len(attrs) == 0 {
		return true, errNoValidSmartData
	}

	// ponytail: DLL size units undocumented; DriveSize64 assumed bytes, DriveSize MB.
	capacity := id.DriveSize64
	if capacity < 1<<30 {
		capacity = uint64(id.DriveSize) * 1000 * 1000
	}

	serial := cString(id.Serial[:])
	key := serial
	if key == "" {
		key = deviceInfo.Name
	}

	sm.Lock()
	defer sm.Unlock()
	sm.SmartDataMap[key] = &smart.SmartData{
		ModelName:       cString(id.Model[:]),
		SerialNumber:    serial,
		FirmwareVersion: cString(id.Firmware[:]),
		Capacity:        capacity,
		SmartStatus:     getSmartStatus(temp, passed),
		DiskName:        deviceInfo.Name,
		DiskType:        diskType,
		Temperature:     temp,
		Attributes:      attrs,
	}
	return true, nil
}
