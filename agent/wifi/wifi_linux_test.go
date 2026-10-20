//go:build linux

package wifi

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	interfaceCalls                                 int
}

func (f *fakeLinuxClient) Interfaces() ([]*native.Interface, error) {
	f.interfaceCalls++
	return f.interfaces, f.interfacesErr
}
func (f *fakeLinuxClient) BSS(i *native.Interface) (*native.BSS, error) {
	return f.bss[i.Name], f.bssErr
}
func (f *fakeLinuxClient) Station(i *native.Interface, mac net.HardwareAddr) (*native.StationInfo, error) {
	f.stationCalls++
	if f.stationErr != nil {
		return nil, f.stationErr
	}
	for _, station := range f.stations[i.Name] {
		if bytes.Equal(station.HardwareAddr, mac) {
			return station, nil
		}
	}
	return nil, errors.New("no such station")
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
			got, _ := collectLinux(ctx, f)
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
	if got, _ := collectLinux(context.Background(), f); len(got) != 1 {
		t.Fatal("initial")
	}
	f.bss["wlan0"].Status = native.BSSStatusNotAssociated
	if got, _ := collectLinux(context.Background(), f); len(got) != 0 {
		t.Fatal("stale association")
	}
	f.bss["wlan0"].Status = native.BSSStatusAssociated
	f.bss["wlan0"].SSID = "new"
	if got, _ := collectLinux(context.Background(), f); got["wlan0"].SSID != "new" {
		t.Fatal("stale SSID")
	}
}

func TestLinuxInvalidSSID(t *testing.T) {
	f := connectedClient()
	f.bss["wlan0"].SSID = "raw\xff"
	got, _ := collectLinux(context.Background(), f)
	if len(got) != 1 || got["wlan0"].SSID != "" || got["wlan0"].Signal == nil {
		t.Fatal(got)
	}
}

func TestHasWirelessDevice(t *testing.T) {
	dir := t.TempDir()
	if hasWirelessDevice(filepath.Join(dir, "missing")) {
		t.Fatal("missing class directory")
	}
	if hasWirelessDevice(dir) {
		t.Fatal("empty class directory")
	}
	if err := os.Mkdir(filepath.Join(dir, "phy0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hasWirelessDevice(dir) {
		t.Fatal("registered device not detected")
	}
}

func TestLinuxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f := connectedClient()
	if got, _ := collectLinux(ctx, f); len(got) != 0 || f.stationCalls != 0 {
		t.Fatal("ignored cancellation")
	}
}

func TestLinuxSkipsIwinfoWithoutAPs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*fakeLinuxClient)
	}{
		{"connected", func(f *fakeLinuxClient) {}},
		{"disconnected", func(f *fakeLinuxClient) { f.bss = nil }},
		{"BSS unavailable", func(f *fakeLinuxClient) { f.bssErr = errors.New("denied") }},
		{"signal unavailable", func(f *fakeLinuxClient) { f.stationErr = errors.New("denied") }},
		{"monitor", func(f *fakeLinuxClient) { f.interfaces[0].Type = native.InterfaceTypeMonitor }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := connectedClient()
			tc.modify(f)
			collectLinuxWithAP(context.Background(), f, func(context.Context, ...string) ([]byte, error) {
				t.Fatal("ran iwinfo without an AP in the netlink inventory")
				return nil, nil
			})
			if f.interfaceCalls != 1 {
				t.Fatalf("queried inventory %d times", f.interfaceCalls)
			}
		})
	}
}

func TestLinuxMixedModes(t *testing.T) {
	for _, assocFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "signal available", true: "signal unavailable"}[assocFails], func(t *testing.T) {
			f := connectedClient()
			for _, name := range []string{"ap0", "ap1"} {
				f.interfaces = append(f.interfaces, &native.Interface{Name: name, Type: native.InterfaceTypeAP})
			}
			var calls []string
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got := collectLinuxWithAP(ctx, f, func(commandCtx context.Context, args ...string) ([]byte, error) {
				if commandCtx != ctx {
					t.Fatal("command did not share the collection context")
				}
				calls = append(calls, strings.Join(args, " "))
				if len(args) != 2 {
					t.Fatal("ran discovery despite having an inventory")
				}
				switch args[1] {
				case "info":
					return []byte(args[0] + " ESSID: \"TestAP\"\n         Mode: Master\n"), nil
				case "assoclist":
					if assocFails {
						return nil, errors.New("denied")
					}
					return []byte("02:00:00:00:00:01  -40 dBm / -95 dBm\n"), nil
				default:
					t.Fatalf("unexpected command: %v", args)
					return nil, nil
				}
			})
			wantCalls := []string{"ap0 info", "ap0 assoclist", "ap1 info", "ap1 assoclist"}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("commands: got %v, want %v", calls, wantCalls)
			}
			if f.interfaceCalls != 1 || f.stationCalls != 1 {
				t.Fatalf("netlink calls: interfaces=%d, stations=%d", f.interfaceCalls, f.stationCalls)
			}
			if len(got) != 3 || got["wlan0"].SSID != "home" || got["wlan0"].Signal == nil || *got["wlan0"].Signal != -52 {
				t.Fatalf("station reading lost: %v", got)
			}
			for _, name := range []string{"ap0", "ap1"} {
				reading := got[name]
				if reading.SSID != "TestAP" || (reading.Signal == nil) != assocFails {
					t.Fatalf("%s: %+v", name, reading)
				}
				if reading.Signal != nil && *reading.Signal != -40 {
					t.Fatalf("%s: signal %v", name, *reading.Signal)
				}
			}
		})
	}
}

func TestLinuxIwinfoDiscoveryFallback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client linuxClient
	}{
		{"no netlink client", nil},
		{"enumeration unsupported", &fakeLinuxClient{interfacesErr: errors.New("unsupported")}},
		{"empty inventory", &fakeLinuxClient{}},
		{"unusable inventory", &fakeLinuxClient{interfaces: []*native.Interface{nil, {}}}},
		{"deadline failure", &fakeLinuxClient{deadlineErr: errors.New("deadline")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got := collectLinuxWithAP(ctx, tc.client, func(_ context.Context, args ...string) ([]byte, error) {
				calls = append(calls, strings.Join(args, " "))
				if len(args) == 0 {
					return []byte(iwinfoMixed), nil
				}
				return []byte("02:00:00:00:00:01  -40 dBm / -95 dBm\n"), nil
			})
			if !reflect.DeepEqual(calls, []string{"", "phy1-ap0 assoclist"}) {
				t.Fatalf("unexpected commands: %v", calls)
			}
			if len(got) != 1 || got["phy1-ap0"].SSID != "TestAP" || got["phy1-ap0"].Signal == nil || *got["phy1-ap0"].Signal != -40 {
				t.Fatalf("unexpected fallback readings: %v", got)
			}
		})
	}
}

func TestLinuxIwinfoUnavailable(t *testing.T) {
	f := connectedClient()
	f.interfaces = append(f.interfaces, &native.Interface{Name: "ap0", Type: native.InterfaceTypeAP})
	unavailable := func(context.Context, ...string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	got := collectLinuxWithAP(context.Background(), f, unavailable)
	if len(got) != 1 || got["wlan0"].Signal == nil || *got["wlan0"].Signal != -52 {
		t.Fatalf("lost station reading when iwinfo is absent: %v", got)
	}
	if got := collectLinuxWithAP(context.Background(), nil, unavailable); got != nil {
		t.Fatalf("expected empty snapshot without either backend: %v", got)
	}
}

func TestLinuxAPCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := connectedClient()
	f.interfaces = append(f.interfaces, &native.Interface{Name: "ap0", Type: native.InterfaceTypeAP})
	calls := 0
	got := collectLinuxWithAP(ctx, f, func(context.Context, ...string) ([]byte, error) {
		calls++
		cancel()
		return []byte("ap0 ESSID: \"TestAP\"\n         Mode: Master\n"), nil
	})
	if calls != 1 || len(got) != 1 || got["wlan0"].Signal == nil {
		t.Fatalf("cancellation lost station data or started another command: calls=%d, readings=%v", calls, got)
	}
	collectLinuxWithAP(ctx, nil, func(context.Context, ...string) ([]byte, error) {
		t.Fatal("ran discovery after cancellation")
		return nil, nil
	})
}

// Sample bare `iwinfo` output in the format printed by OpenWrt (nl80211): one
// block per wireless interface, with an AP (Master) interface and a client
// interface. SSIDs and MAC addresses are anonymized.
const iwinfoMixed = `phy1-ap0 ESSID: "TestAP"
         Access Point: 02:00:00:00:00:01
         Mode: Master  Channel: 6 (2.437 GHz)  HT Mode: HT40
         Center Channel 1: 6 2: unknown
         Tx-Power: 30 dBm  Link Quality: 70/70
         Signal: -36 dBm  Noise: -96 dBm
         Bit Rate: 72.2 MBit/s
         Encryption: SAE (CCMP)
         Type: nl80211  HW Mode(s): 802.11ac/n/g/b
         Hardware: embedded [Qualcomm Atheros IPQ4019]
         TX power offset: none
         Channel offset: none
         Supports VAPs: yes  PHY name: phy1

lan_bridge ESSID: "TestUpstream"
         Access Point: 02:00:00:00:00:02
         Mode: Client  Channel: 149 (5.745 GHz)  HT Mode: VHT80
         Center Channel 1: 155 2: unknown
         Tx-Power: 30 dBm  Link Quality: 50/70
         Signal: -60 dBm  Noise: -99 dBm
         Bit Rate: 260.0 MBit/s
         Encryption: none
         Type: nl80211  HW Mode(s): 802.11ac/n
         Hardware: 0x168c:0x0056 0x0000:0x0000 [Qualcomm Atheros QCA9886]
         TX power offset: none
         Channel offset: none
         Supports VAPs: yes  PHY name: phy0
`

func TestSplitAndParseIwinfo(t *testing.T) {
	blocks := splitIwinfoBlocks(iwinfoMixed)
	if len(blocks) != 2 || blocks[0].name != "phy1-ap0" || blocks[1].name != "lan_bridge" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	if ssid, isAP := parseIwinfoInfo(blocks[0].text); !isAP || ssid != "TestAP" {
		t.Fatalf("ap block: got (%q, %v)", ssid, isAP)
	}
	if _, isAP := parseIwinfoInfo(blocks[1].text); isAP {
		t.Fatal("client-mode interface must not be reported as AP")
	}
	if got := splitIwinfoBlocks(""); len(got) != 0 {
		t.Fatalf("empty output: got %+v", got)
	}
}

func TestParseIwinfoInfoHiddenSSID(t *testing.T) {
	block := "wlan0 ESSID: unknown\n         Mode: Master  Channel: 6 (2.437 GHz)\n"
	if ssid, isAP := parseIwinfoInfo(block); !isAP || ssid != "" {
		t.Fatalf("got (%q, %v)", ssid, isAP)
	}
}

// Client MAC addresses are anonymized; the line format matches
// `iwinfo <device> assoclist`.
func TestParseAssocRSSI(t *testing.T) {
	for _, tc := range []struct {
		name   string
		in     string
		want   float64
		wantOK bool
	}{
		{"multiple clients",
			"02:00:00:00:00:01  -33 dBm / -97 dBm (SNR 64)  740 ms ago\n" +
				"        RX: 54.0 MBit/s   1191056 Pkts.\n" +
				"        TX: 72.2 MBit/s, MCS 7, unknownMHz     90627 Pkts.\n" +
				"        expected throughput: 65.0 MBit/s\n\n" +
				"02:00:00:00:00:02  -41 dBm / -97 dBm (SNR 56)  30150 ms ago\n" +
				"        RX: 72.2 MBit/s, MCS 7, unknownMHz     12378 Pkts.\n",
			-37, true},
		{"single wifi7 client",
			"02:00:00:00:00:03  -41 dBm / -100 dBm (SNR 59)  11320 ms ago\n" +
				"        RX: 2401.9 MBit/s, HE-MCS 11, 160MHz, HE-NSS 2, HE-GI 0, HE-DCM 0      8268 Pkts.\n",
			-41, true},
		{"no clients (empty output)", "", 0, false},
		{"malformed", "garbage\n02:00:00 not-a-mac -50 dBm\n", 0, false},
		{"missing rssi", "02:00:00:00:00:01  unknown signal  100 ms ago\n", 0, false},
		{"unknown rssi with noise",
			"02:00:00:00:00:01  unknown / -95 dBm (SNR 95)  100 ms ago\n",
			0, false},
		{"unknown rssi excluded from average",
			"02:00:00:00:00:01  unknown / -95 dBm (SNR 95)  100 ms ago\n" +
				"02:00:00:00:00:02  -40 dBm / -95 dBm (SNR 55)  100 ms ago\n",
			-40, true},
		{"invalid values ignored",
			"02:00:00:00:00:01  -40 dBm / -95 dBm (SNR 55)  100 ms ago\n" +
				"02:00:00:00:00:02  0 dBm / -95 dBm  100 ms ago\n" +
				"02:00:00:00:00:03  -200 dBm / -95 dBm  100 ms ago\n",
			-40, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseAssocRSSI(tc.in)
			if ok != tc.wantOK || (ok && got != tc.want) {
				t.Fatalf("got (%v, %v), want (%v, %v)", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
