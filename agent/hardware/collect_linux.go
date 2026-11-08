//go:build linux

package hardware

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/hardware"
)

func collect(opts Options) hardware.Inventory {
	inv := hardware.Inventory{Collected: time.Now().Unix()}
	sys, err := openSysfs(opts.Root)
	if err != nil {
		inv.Unavailable = append(inv.Unavailable, hardware.Gap{Section: "all", Reason: err.Error()})
		return inv
	}
	defer sys.close()

	if d, err := readSMBIOS(sys, opts.SMBIOS); err == nil {
		inv.System, inv.Board, inv.BIOS, inv.Chassis = d.System, d.Board, d.BIOS, d.Chassis
		inv.Memory = d.Memory
	} else {
		inv.Unavailable = append(inv.Unavailable, hardware.Gap{Section: "memory", Reason: "SMBIOS table: " + err.Error()})
	}
	dmiFallback(sys, &inv)
	inv.CPUs = cpuInfo(sys)
	inv.PCI = pciDevices(sys)
	inv.USB = usbDevices(sys)
	inv.Disks = disks(sys)
	inv.NICs = nics(sys)
	inv.Displays = displays(sys)
	return inv
}

// readSMBIOS reads the raw table from the configured path, the systemd credential,
// or sysfs (readable only when the agent already runs as root).
func readSMBIOS(sys sysfs, configured string) (smbiosData, error) {
	var b []byte
	var err error
	switch {
	case configured != "":
		b, err = readFileLimited(configured)
	case os.Getenv("CREDENTIALS_DIRECTORY") != "":
		b, err = readFileLimited(filepath.Join(os.Getenv("CREDENTIALS_DIRECTORY"), "smbios"))
		if errors.Is(err, fs.ErrNotExist) {
			b, err = sys.bytes("sys/firmware/dmi/tables/DMI", maxSMBIOSTable+1)
		}
	default:
		b, err = sys.bytes("sys/firmware/dmi/tables/DMI", maxSMBIOSTable+1)
	}
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return smbiosData{}, errors.New("permission denied (hand it to the agent with LoadCredential=smbios:/sys/firmware/dmi/tables/DMI)")
		}
		return smbiosData{}, err
	}
	return parseSMBIOS(b)
}

// readFileLimited reads an operator-configured file, refusing anything oversized.
func readFileLimited(name string) ([]byte, error) {
	f, err := os.Open(name) // #nosec G304 -- path comes from the operator (--smbios) or systemd ($CREDENTIALS_DIRECTORY), never from the hub
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxSMBIOSTable+1))
}

// dmiFallback fills firmware fields from the world-readable DMI attributes;
// serials there are root-only and stay empty for an unprivileged agent.
func dmiFallback(sys sysfs, inv *hardware.Inventory) {
	dmi := func(name string) string { return sys.text("sys/class/dmi/id/" + name) }
	fill(&inv.System.Vendor, dmi("sys_vendor"))
	fill(&inv.System.Product, dmi("product_name"))
	fill(&inv.System.Version, dmi("product_version"))
	fill(&inv.System.Family, dmi("product_family"))
	fill(&inv.System.SKU, dmi("product_sku"))
	fill(&inv.System.Serial, dmi("product_serial"))
	fill(&inv.System.UUID, dmi("product_uuid"))
	fill(&inv.Board.Vendor, dmi("board_vendor"))
	fill(&inv.Board.Product, dmi("board_name"))
	fill(&inv.Board.Version, dmi("board_version"))
	fill(&inv.Board.Serial, dmi("board_serial"))
	fill(&inv.BIOS.Vendor, dmi("bios_vendor"))
	fill(&inv.BIOS.Version, dmi("bios_version"))
	fill(&inv.BIOS.Date, dmi("bios_date"))
	fill(&inv.Chassis.Vendor, dmi("chassis_vendor"))
	fill(&inv.Chassis.Serial, dmi("chassis_serial"))
	if t, err := strconv.Atoi(dmi("chassis_type")); err == nil && t > 0 && t < 256 {
		fill(&inv.Chassis.Type, chassisType(byte(t)))
	}
}

// cpuInfo summarises /proc/cpuinfo per physical package.
func cpuInfo(sys sysfs) []hardware.CPU {
	b, err := sys.bytes("proc/cpuinfo", 4<<20)
	if err != nil {
		return nil
	}
	packages := map[string]*hardware.CPU{}
	var order []string
	cur := map[string]string{}
	flush := func() {
		if len(cur) == 0 {
			return
		}
		id := cur["physical id"]
		c, ok := packages[id]
		if !ok {
			c = &hardware.CPU{Socket: id, Vendor: clean(cur["vendor_id"]), Model: clean(cur["model name"]), Microcode: clean(cur["microcode"])}
			c.Cores, _ = strconv.Atoi(cur["cpu cores"])
			packages[id] = c
			order = append(order, id)
		}
		c.Threads++
		cur = map[string]string{}
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			flush()
			continue
		}
		cur[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	flush()
	out := make([]hardware.CPU, 0, len(order))
	for _, id := range order {
		out = append(out, *packages[id])
	}
	return out
}
