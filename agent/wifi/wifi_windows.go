//go:build windows

package wifi

import (
	"context"
	"unsafe"

	"github.com/henrygd/beszel/internal/entities/system"
	"golang.org/x/sys/windows"
)

var wlan = windows.NewLazySystemDLL("wlanapi.dll")
var wlanOpen = wlan.NewProc("WlanOpenHandle")
var wlanClose = wlan.NewProc("WlanCloseHandle")
var wlanEnum = wlan.NewProc("WlanEnumInterfaces")
var wlanQuery = wlan.NewProc("WlanQueryInterface")
var wlanFree = wlan.NewProc("WlanFreeMemory")

type wlanInterface struct {
	GUID        windows.GUID
	Description [256]uint16
	State       uint32
}

type wlanConnection struct {
	State      uint32
	Mode       uint32
	Profile    [256]uint16
	SSIDLength uint32
	SSID       [32]byte
	// Only the prefix through DOT11_SSID is read.
}

func collect(ctx context.Context) map[string]system.WiFi {
	result := make(map[string]system.WiFi)
	for _, proc := range []*windows.LazyProc{wlanOpen, wlanClose, wlanEnum, wlanQuery, wlanFree} {
		if proc.Find() != nil {
			return result
		}
	}
	var handle windows.Handle
	var version uint32
	if rc, _, _ := wlanOpen.Call(2, 0, uintptr(unsafe.Pointer(&version)), uintptr(unsafe.Pointer(&handle))); rc != 0 {
		return result
	}
	defer wlanClose.Call(uintptr(handle), 0)
	var list unsafe.Pointer
	if rc, _, _ := wlanEnum.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&list))); rc != 0 || list == nil {
		return result
	}
	defer wlanFree.Call(uintptr(list))
	count := *(*uint32)(list)
	if count > 1024 {
		return result
	}
	interfaces := unsafe.Slice((*wlanInterface)(unsafe.Add(list, 8)), int(count))
	for _, iface := range interfaces {
		if ctx.Err() != nil {
			break
		}
		if iface.State != 1 {
			continue
		} // wlan_interface_state_connected
		reading := system.WiFi{}
		// SSID access may be denied by location privacy policy. Association comes
		// from the interface state, so missing SSID does not suppress valid RSSI.
		if data, size := queryWLAN(handle, &iface.GUID, 7); data != nil {
			if size >= uint32(unsafe.Sizeof(wlanConnection{})) {
				connection := (*wlanConnection)(data)
				if connection.State == 1 && connection.SSIDLength <= 32 {
					reading.SSID = validSSID(string(connection.SSID[:connection.SSIDLength]))
				}
			}
			wlanFree.Call(uintptr(data))
		}
		// Native RSSI LONG, not the quality percentage in association attributes.
		if data, size := queryWLAN(handle, &iface.GUID, 0x10000102); data != nil {
			if size >= 4 {
				signal := float64(*(*int32)(data))
				if signal >= -150 && signal < 0 {
					reading.Signal = &signal
				}
			}
			wlanFree.Call(uintptr(data))
		}
		result[iface.GUID.String()] = reading
	}
	return result
}

func queryWLAN(handle windows.Handle, guid *windows.GUID, opcode uintptr) (unsafe.Pointer, uint32) {
	var data unsafe.Pointer
	var size uint32
	if rc, _, _ := wlanQuery.Call(uintptr(handle), uintptr(unsafe.Pointer(guid)), opcode, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), 0); rc != 0 {
		return nil, 0
	}
	return data, size
}
