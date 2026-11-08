// Package hwnames gives PCI and USB devices readable names.
//
// Agents report bare vendor/device IDs; the hub looks the names up in the
// pci.ids and usb.ids databases (the hwdata package on most distributions).
// Keeping the databases on the hub keeps agents small and gives every agent the
// same names. Without the databases, devices simply keep their IDs.
package hwnames

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/henrygd/beszel/internal/entities/hardware"
	"github.com/henrygd/beszel/internal/hub/utils"
)

// searchDirs are the usual hwdata locations; HWDATA_DIR overrides them.
var searchDirs = []string{"/usr/share/hwdata", "/usr/share/misc", "/usr/share"}

// names maps "vendor" and "vendor:device" IDs (lowercase hex) to names.
// For pci.ids it also maps "class:XXXX" (class + subclass) to class names.
type names map[string]string

var load = sync.OnceValues(func() (names, names) {
	dirs := searchDirs
	if dir, ok := utils.GetEnv("HWDATA_DIR"); ok {
		dirs = []string{dir}
	}
	return readFirst(dirs, "pci.ids"), readFirst(dirs, "usb.ids")
})

// Resolve fills the name fields of the inventory's PCI and USB devices.
func Resolve(inv *hardware.Inventory) {
	pci, usb := load()
	for i := range inv.PCI {
		d := &inv.PCI[i]
		d.VendorName = pci[d.Vendor]
		d.DeviceName = pci[d.Vendor+":"+d.Device]
		if len(d.Class) >= 4 {
			d.ClassName = pci["class:"+d.Class[:4]]
		}
	}
	for i := range inv.USB {
		d := &inv.USB[i]
		d.VendorName = usb[d.Vendor]
		d.ProductName = usb[d.Vendor+":"+d.Product]
	}
}

func readFirst(dirs []string, file string) names {
	for _, dir := range dirs {
		if f, err := os.Open(filepath.Join(dir, file)); err == nil {
			defer f.Close()
			return parse(bufio.NewScanner(f))
		}
	}
	return names{}
}

// parse reads the ids format: a top-level "vvvv  Vendor" line followed by
// tab-indented "dddd  Device" lines, and in pci.ids a "C cc  Class" section with
// tab-indented "ss  Subclass" lines. Deeper levels and other sections are skipped.
func parse(scanner *bufio.Scanner) names {
	out := names{}
	section, parent := "", ""
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		id, name, ok := strings.Cut(strings.TrimLeft(line, "\t"), "  ")
		if !ok {
			continue
		}
		switch depth := len(line) - len(strings.TrimLeft(line, "\t")); {
		case depth == 0 && strings.HasPrefix(id, "C "):
			section, parent = "class", strings.ToLower(id[2:])
		case depth == 0 && len(id) == 4:
			section, parent = "vendor", strings.ToLower(id)
			out[parent] = name
		case depth == 0:
			section = "" // other sections (usb.ids AT, HID, ...)
		case depth == 1 && section == "vendor":
			out[parent+":"+strings.ToLower(id)] = name
		case depth == 1 && section == "class":
			out["class:"+parent+strings.ToLower(id)] = name
		}
	}
	return out
}
