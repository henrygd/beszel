//go:build linux

package wifi

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	native "github.com/mdlayher/wifi"
	"golang.org/x/sys/unix"
)

type linuxClient interface {
	Interfaces() ([]*native.Interface, error)
	BSS(*native.Interface) (*native.BSS, error)
	Station(*native.Interface, net.HardwareAddr) (*native.StationInfo, error)
	SetDeadline(time.Time) error
	Close() error
}

// nl80211Client adds a targeted GET_STATION request, as used by `iw link`.
// mdlayher/wifi only dumps stations, which some full-MAC drivers (e.g.
// out-of-tree Realtek USB) answer with an empty list.
type nl80211Client struct {
	*native.Client
	conn   *genetlink.Conn
	family genetlink.Family
}

func newNL80211Client() (*nl80211Client, error) {
	client, err := native.New()
	if err != nil {
		return nil, err
	}
	conn, err := genetlink.Dial(nil)
	if err != nil {
		client.Close()
		return nil, err
	}
	family, err := conn.GetFamily(unix.NL80211_GENL_NAME)
	if err != nil {
		conn.Close()
		client.Close()
		return nil, err
	}
	return &nl80211Client{Client: client, conn: conn, family: family}, nil
}

func (c *nl80211Client) Station(ifi *native.Interface, mac net.HardwareAddr) (*native.StationInfo, error) {
	ae := netlink.NewAttributeEncoder()
	ae.Uint32(unix.NL80211_ATTR_IFINDEX, uint32(ifi.Index))
	ae.Bytes(unix.NL80211_ATTR_MAC, mac)
	data, err := ae.Encode()
	if err != nil {
		return nil, err
	}
	msgs, err := c.conn.Execute(genetlink.Message{
		Header: genetlink.Header{Command: unix.NL80211_CMD_GET_STATION, Version: c.family.Version},
		Data:   data,
	}, c.family.ID, netlink.Request)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, errors.New("no station info")
	}
	return native.ParseStationInfo(msgs[0].Data)
}

func (c *nl80211Client) SetDeadline(t time.Time) error {
	return errors.Join(c.Client.SetDeadline(t), c.conn.SetDeadline(t))
}

func (c *nl80211Client) Close() error {
	return errors.Join(c.conn.Close(), c.Client.Close())
}

// wiphyClassDir lists the wireless devices registered with cfg80211.
const wiphyClassDir = "/sys/class/ieee80211"

// hasWirelessDevice gates nl80211 access. Looking up the nl80211 family while
// cfg80211 is not loaded makes the kernel run modprobe, which loads the module
// on hosts without wireless hardware, or fails again on every poll. Not cached,
// so devices registered after startup are still picked up.
func hasWirelessDevice(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	names, _ := f.Readdirnames(1)
	return len(names) > 0
}

func collect(ctx context.Context) map[string]system.WiFi {
	results := make(map[string]system.WiFi)

	if hasWirelessDevice(wiphyClassDir) {
		if client, err := newNL80211Client(); err == nil {
			defer client.Close()
			for k, v := range collectLinux(ctx, client) {
				results[k] = v
			}
		}
	}

	if _, err := exec.LookPath("iwinfo"); err == nil {
		for k, v := range collectOpenWrtAP() {
			results[k] = v
		}
	}

	if len(results) == 0 {
		return nil
	}
	return results
}

func collectOpenWrtAP() map[string]system.WiFi {
	results := make(map[string]system.WiFi)

	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return results
	}

	reMode := regexp.MustCompile(`Mode:\s*Master`)
	reSsid := regexp.MustCompile(`ESSID:\s*"([^"]+)"`)
	reAssoc := regexp.MustCompile(`(?m)^([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}.*?(-?\d+)\s+dBm`)

	for _, entry := range entries {
		iface := entry.Name()

		if iface == "lo" || strings.HasPrefix(iface, "eth") || strings.HasPrefix(iface, "br-") ||
			strings.HasPrefix(iface, "docker") || strings.HasPrefix(iface, "veth") || strings.HasPrefix(iface, "pppoe") {
			continue
		}

		infoOut, err := exec.Command("iwinfo", iface, "info").Output()
		if err != nil {
			continue
		}
		infoStr := string(infoOut)

		if !reMode.MatchString(infoStr) {
			continue
		}

		ssid := "Unknown"
		if m := reSsid.FindStringSubmatch(infoStr); len(m) > 1 {
			ssid = m[1]
		}

		var avgRssi float64 = -100
		assocOut, err := exec.Command("iwinfo", iface, "assoclist").Output()
		if err == nil {
			matches := reAssoc.FindAllStringSubmatch(string(assocOut), -1)
			sum, count := 0, 0
			for _, m := range matches {
				if len(m) > 2 {
					if rssi, err := strconv.Atoi(m[2]); err == nil {
						sum += rssi
						count++
					}
				}
			}
			if count > 0 {
				avgRssi = float64(sum) / float64(count)
			}
		}

		reading := system.WiFi{SSID: validSSID(ssid)}
		if avgRssi >= -150 && avgRssi < 0 {
			sig := avgRssi 
			reading.Signal = &sig
		}

		results[iface] = reading
	}

	return results
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
		if len(bss.BSSID) > 0 {
			if station, err := client.Station(iface, bss.BSSID); err == nil && station != nil {
				signal := float64(station.Signal)
				if signal >= -150 && signal < 0 {
					reading.Signal = &signal
				}
			}
		}
		result[iface.Name] = reading
	}
	return result
}
