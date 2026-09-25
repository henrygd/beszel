//go:build linux

package wifi

import (
	"bytes"
	"context"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	native "github.com/mdlayher/wifi"
)

type linuxClient interface {
	Interfaces() ([]*native.Interface, error)
	BSS(*native.Interface) (*native.BSS, error)
	StationInfo(*native.Interface) ([]*native.StationInfo, error)
	SetDeadline(time.Time) error
	Close() error
}

func collect(ctx context.Context) map[string]system.WiFi {
	client, err := native.New()
	if err != nil {
		return nil
	}
	defer client.Close()
	return collectLinux(ctx, client)
}

func collectLinux(ctx context.Context, client linuxClient) map[string]system.WiFi {
	result := make(map[string]system.WiFi)
	if ctx.Err() != nil {
		return result
	}
	if deadline, ok := ctx.Deadline(); ok {
		if client.SetDeadline(deadline) != nil {
			return result
		}
	}
	interfaces, err := client.Interfaces()
	if err != nil {
		return result
	}
	for _, iface := range interfaces {
		if ctx.Err() != nil {
			break
		}
		if iface == nil || iface.Type != native.InterfaceTypeStation || iface.Name == "" {
			continue
		}
		// GET_SCAN reads the kernel's BSS cache, without triggering a scan.
		// Only the explicit associated status proves a current connection.
		bss, err := client.BSS(iface)
		if err != nil || bss == nil || bss.Status != native.BSSStatusAssociated {
			continue
		}
		reading := system.WiFi{SSID: validSSID(bss.SSID)}
		// Station statistics may require permissions unavailable in default
		// containers. Keep association even when RSSI cannot be read. Do not
		// substitute cached scan signal, which may be arbitrarily old.
		stations, err := client.StationInfo(iface)
		if err == nil {
			for _, station := range stations {
				if station == nil || len(bss.BSSID) == 0 || !bytes.Equal(station.HardwareAddr, bss.BSSID) {
					continue
				}
				signal := float64(station.Signal)
				if signal >= -150 && signal < 0 {
					reading.Signal = &signal
				}
				break
			}
		}
		result[iface.Name] = reading
	}
	return result
}
