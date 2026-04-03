//go:build windows

// Most of the Windows battery code is based on
// distatus/battery by Karol 'Kenji Takahashi' Woźniak

package battery

import (
	"errors"
	"log/slog"
	"math"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type batteryQueryInformation struct {
	BatteryTag       uint32
	InformationLevel int32
	AtRate           int32
}

type batteryInformation struct {
	Capabilities        uint32
	Technology          uint8
	Reserved            [3]uint8
	Chemistry           [4]uint8
	DesignedCapacity    uint32
	FullChargedCapacity uint32
	DefaultAlert1       uint32
	DefaultAlert2       uint32
	CriticalBias        uint32
	CycleCount          uint32
}

type batteryWaitStatus struct {
	BatteryTag   uint32
	Timeout      uint32
	PowerState   uint32
	LowCapacity  uint32
	HighCapacity uint32
}

type batteryStatus struct {
	PowerState uint32
	Capacity   uint32
	Voltage    uint32
	Rate       int32
}

type winGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type spDeviceInterfaceData struct {
	cbSize             uint32
	InterfaceClassGuid winGUID
	Flags              uint32
	Reserved           uint
}

var guidDeviceBattery = winGUID{
	0x72631e54,
	0x78A4,
	0x11d0,
	[8]byte{0xbc, 0xf7, 0x00, 0xaa, 0x00, 0xb7, 0xb3, 0x2a},
}

var (
	setupapi                         = &windows.LazyDLL{Name: "setupapi.dll", System: true}
	setupDiGetClassDevsW             = setupapi.NewProc("SetupDiGetClassDevsW")
	setupDiEnumDeviceInterfaces      = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	setupDiGetDeviceInterfaceDetailW = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	setupDiDestroyDeviceInfoList     = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

// winBatteryGet reads one battery by index. Returns (fullCapacity, currentCapacity, state, error).
// Returns error == errNotFound when there are no more batteries.
var errNotFound = errors.New("no more batteries")

func setupDiSetup(proc *windows.LazyProc, nargs, a1, a2, a3, a4, a5, a6 uintptr) (uintptr, error) {
	_ = nargs
	r1, _, errno := syscall.SyscallN(proc.Addr(), a1, a2, a3, a4, a5, a6)
	if windows.Handle(r1) == windows.InvalidHandle {
		if errno != 0 {
			return 0, error(errno)
		}
		return 0, syscall.EINVAL
	}
	return r1, nil
}

func setupDiCall(proc *windows.LazyProc, nargs, a1, a2, a3, a4, a5, a6 uintptr) syscall.Errno {
	_ = nargs
	r1, _, errno := syscall.SyscallN(proc.Addr(), a1, a2, a3, a4, a5, a6)
	if r1 == 0 {
		if errno != 0 {
			return errno
		}
		return syscall.EINVAL
	}
	return 0
}

func readWinBatteryState(powerState uint32) uint8 {
	switch {
	case powerState&0x00000004 != 0:
		return stateCharging
	case powerState&0x00000008 != 0:
		return stateEmpty
	case powerState&0x00000002 != 0:
		return stateDischarging
	case powerState&0x00000001 != 0:
		return stateFull
	default:
		return stateUnknown
	}
}

func winBatteryGet(idx int) (full, current uint32, state uint8, err error) {
	hdev, err := setupDiSetup(
		setupDiGetClassDevsW,
		4,
		uintptr(unsafe.Pointer(&guidDeviceBattery)),
		0, 0,
		2|16, // DIGCF_PRESENT|DIGCF_DEVICEINTERFACE
		0, 0,
	)
	if err != nil {
		return 0, 0, stateUnknown, err
	}
	defer syscall.SyscallN(setupDiDestroyDeviceInfoList.Addr(), hdev)

	var did spDeviceInterfaceData
	did.cbSize = uint32(unsafe.Sizeof(did))
	errno := setupDiCall(
		setupDiEnumDeviceInterfaces,
		5,
		hdev, 0,
		uintptr(unsafe.Pointer(&guidDeviceBattery)),
		uintptr(idx),
		uintptr(unsafe.Pointer(&did)),
		0,
	)
	if errno == 259 { // ERROR_NO_MORE_ITEMS
		return 0, 0, stateUnknown, errNotFound
	}
	if errno != 0 {
		return 0, 0, stateUnknown, errno
	}

	var cbRequired uint32
	errno = setupDiCall(
		setupDiGetDeviceInterfaceDetailW,
		6,
		hdev,
		uintptr(unsafe.Pointer(&did)),
		0, 0,
		uintptr(unsafe.Pointer(&cbRequired)),
		0,
	)
	if errno != 0 && errno != 122 { // ERROR_INSUFFICIENT_BUFFER
		return 0, 0, stateUnknown, errno
	}
	didd := make([]uint16, cbRequired/2)
	cbSize := (*uint32)(unsafe.Pointer(&didd[0]))
	if unsafe.Sizeof(uint(0)) == 8 {
		*cbSize = 8
	} else {
		*cbSize = 6
	}
	errno = setupDiCall(
		setupDiGetDeviceInterfaceDetailW,
		6,
		hdev,
		uintptr(unsafe.Pointer(&did)),
		uintptr(unsafe.Pointer(&didd[0])),
		uintptr(cbRequired),
		uintptr(unsafe.Pointer(&cbRequired)),
		0,
	)
	if errno != 0 {
		return 0, 0, stateUnknown, errno
	}
	devicePath := &didd[2:][0]

	handle, err := windows.CreateFile(
		devicePath,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return 0, 0, stateUnknown, err
	}
	defer windows.CloseHandle(handle)

	var dwOut uint32
	var dwWait uint32
	var bqi batteryQueryInformation
	err = windows.DeviceIoControl(
		handle,
		2703424, // IOCTL_BATTERY_QUERY_TAG
		(*byte)(unsafe.Pointer(&dwWait)),
		uint32(unsafe.Sizeof(dwWait)),
		(*byte)(unsafe.Pointer(&bqi.BatteryTag)),
		uint32(unsafe.Sizeof(bqi.BatteryTag)),
		&dwOut, nil,
	)
	if err != nil || bqi.BatteryTag == 0 {
		return 0, 0, stateUnknown, errors.New("battery tag not returned")
	}

	var bi batteryInformation
	if err = windows.DeviceIoControl(
		handle,
		2703428, // IOCTL_BATTERY_QUERY_INFORMATION
		(*byte)(unsafe.Pointer(&bqi)),
		uint32(unsafe.Sizeof(bqi)),
		(*byte)(unsafe.Pointer(&bi)),
		uint32(unsafe.Sizeof(bi)),
		&dwOut, nil,
	); err != nil {
		return 0, 0, stateUnknown, err
	}

	bws := batteryWaitStatus{BatteryTag: bqi.BatteryTag}
	var bs batteryStatus
	if err = windows.DeviceIoControl(
		handle,
		2703436, // IOCTL_BATTERY_QUERY_STATUS
		(*byte)(unsafe.Pointer(&bws)),
		uint32(unsafe.Sizeof(bws)),
		(*byte)(unsafe.Pointer(&bs)),
		uint32(unsafe.Sizeof(bs)),
		&dwOut, nil,
	); err != nil {
		return 0, 0, stateUnknown, err
	}

	if bs.Capacity == 0xffffffff { // BATTERY_UNKNOWN_CAPACITY
		return 0, 0, stateUnknown, errors.New("battery capacity unknown")
	}

	return bi.FullChargedCapacity, bs.Capacity, readWinBatteryState(bs.PowerState), nil
}

// HasReadableBattery checks if the system has a battery and returns true if it does.
var HasReadableBattery = sync.OnceValue(func() bool {
	systemHasBattery := false
	full, _, _, err := winBatteryGet(0)
	if err == nil && full > 0 {
		systemHasBattery = true
	}
	if !systemHasBattery {
		slog.Debug("No battery found", "err", err)
	}
	return systemHasBattery
})

// GetBatteryStats returns the current battery percent and charge state.
func GetBatteryStats() (batteryPercent uint8, batteryState uint8, err error) {
	if !HasReadableBattery() {
		return batteryPercent, batteryState, errors.ErrUnsupported
	}

	totalFull := uint32(0)
	totalCurrent := uint32(0)
	batteryState = math.MaxUint8

	for i := 0; ; i++ {
		full, current, state, bErr := winBatteryGet(i)
		if errors.Is(bErr, errNotFound) {
			break
		}
		if bErr != nil || full == 0 {
			continue
		}
		totalFull += full
		totalCurrent += min(current, full)
		batteryState = state
	}

	if totalFull == 0 || batteryState == math.MaxUint8 {
		return batteryPercent, batteryState, errors.New("no battery capacity")
	}

	batteryPercent = uint8(float64(totalCurrent) / float64(totalFull) * 100)
	return batteryPercent, batteryState, nil
}
