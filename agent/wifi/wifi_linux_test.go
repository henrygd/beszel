//go:build linux

package wifi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	native "github.com/mdlayher/wifi"
)

type fakeLinuxClient struct {
	interfaces                                     []*native.Interface
	bss                                            map[string]*native.BSS
	stations                                       map[string][]*native.StationInfo
	interfacesErr, bssErr, stationErr, deadlineErr error
	deadline                                       time.Time
	stationCalls                                   int
}

func (f *fakeLinuxClient) Interfaces() ([]*native.Interface, error) {
	return f.interfaces, f.interfacesErr
}
func (f *fakeLinuxClient) BSS(i *native.Interface) (*native.BSS, error) {
	return f.bss[i.Name], f.bssErr
}
func (f *fakeLinuxClient) StationInfo(i *native.Interface) ([]*native.StationInfo, error) {
	f.stationCalls++
	return f.stations[i.Name], f.stationErr
}
func (f *fakeLinuxClient) SetDeadline(d time.Time) error { f.deadline = d; return f.deadlineErr }
func (f *fakeLinuxClient) Close() error                  { return nil }

func connectedClient() *fakeLinuxClient {
	mac := net.HardwareAddr{1, 2, 3, 4, 5, 6}
	return &fakeLinuxClient{
		interfaces: []*native.Interface{{Name: "wlan0", Type: native.InterfaceTypeStation}},
		bss:        map[string]*native.BSS{"wlan0": {Status: native.BSSStatusAssociated, SSID: "home", BSSID: mac}},
		stations:   map[string][]*native.StationInfo{"wlan0": {{HardwareAddr: mac, Signal: -52}}},
	}
}

func TestLinuxSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name       string
		modify     func(*fakeLinuxClient)
		want       int
		wantSignal bool
	}{
		{"connected", func(f *fakeLinuxClient) {}, 1, true},
		{"multiple", func(f *fakeLinuxClient) {
			f.interfaces = append(f.interfaces, &native.Interface{Name: "wlan1", Type: native.InterfaceTypeStation})
			f.bss["wlan1"] = f.bss["wlan0"]
		}, 2, true},
		{"unsupported", func(f *fakeLinuxClient) { f.interfacesErr = errors.New("unsupported") }, 0, false},
		{"association denied", func(f *fakeLinuxClient) { f.bssErr = errors.New("denied") }, 0, false},
		{"disconnected", func(f *fakeLinuxClient) { f.bss["wlan0"] = nil }, 0, false},
		{"authenticated only", func(f *fakeLinuxClient) { f.bss["wlan0"].Status = native.BSSStatusAuthenticated }, 0, false},
		{"cached nearby BSS", func(f *fakeLinuxClient) { f.bss["wlan0"].Status = native.BSSStatusNotAssociated }, 0, false},
		{"access point", func(f *fakeLinuxClient) { f.interfaces[0].Type = native.InterfaceTypeAP }, 0, false},
		{"ad hoc", func(f *fakeLinuxClient) { f.bss["wlan0"].Status = native.BSSStatusIBSSJoined }, 0, false},
		{"station permission denied", func(f *fakeLinuxClient) {
			f.stationErr = errors.New("permission denied")
			f.bss["wlan0"].Signal = -4200
		}, 1, false},
		{"no station data", func(f *fakeLinuxClient) { f.stations = nil }, 1, false},
		{"different AP", func(f *fakeLinuxClient) { f.stations["wlan0"][0].HardwareAddr = net.HardwareAddr{9, 8, 7, 6, 5, 4} }, 1, false},
		{"missing signal", func(f *fakeLinuxClient) { f.stations["wlan0"][0].Signal = 0 }, 1, false},
		{"invalid signal", func(f *fakeLinuxClient) { f.stations["wlan0"][0].Signal = -151 }, 1, false},
		{"deadline failure", func(f *fakeLinuxClient) { f.deadlineErr = errors.New("deadline") }, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := connectedClient()
			tc.modify(f)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got := collectLinux(ctx, f)
			if len(got) != tc.want {
				t.Fatalf("got %#v", got)
			}
			if tc.want > 0 && (got["wlan0"].Signal != nil) != tc.wantSignal {
				t.Fatalf("signal: %#v", got["wlan0"])
			}
			if tc.wantSignal && *got["wlan0"].Signal != -52 {
				t.Fatal(got)
			}
			if tc.want == 0 && f.stationCalls != 0 {
				t.Fatal("queried station without association")
			}
			deadline, _ := ctx.Deadline()
			if f.deadline != deadline {
				t.Fatal("deadline not shared")
			}
		})
	}
}

func TestReconnect(t *testing.T) {
	f := connectedClient()
	if len(collectLinux(context.Background(), f)) != 1 {
		t.Fatal("initial")
	}
	f.bss["wlan0"].Status = native.BSSStatusNotAssociated
	if len(collectLinux(context.Background(), f)) != 0 {
		t.Fatal("stale association")
	}
	f.bss["wlan0"].Status = native.BSSStatusAssociated
	f.bss["wlan0"].SSID = "new"
	if collectLinux(context.Background(), f)["wlan0"].SSID != "new" {
		t.Fatal("stale SSID")
	}
}

func TestLinuxInvalidSSID(t *testing.T) {
	f := connectedClient()
	f.bss["wlan0"].SSID = "raw\xff"
	got := collectLinux(context.Background(), f)
	if len(got) != 1 || got["wlan0"].SSID != "" || got["wlan0"].Signal == nil {
		t.Fatal(got)
	}
}

func TestLinuxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := connectedClient()
	if len(collectLinux(ctx, f)) != 0 || f.stationCalls != 0 {
		t.Fatal("ignored cancellation")
	}
}
