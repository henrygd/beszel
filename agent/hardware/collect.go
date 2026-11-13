// Package hardware collects the hardware inventory of the host the agent runs on.
//
// Collection only reads. Paths are fixed in this package; nothing a hub sends can
// select a path. Root-only sources (the SMBIOS table, DMI serial files) are never
// opened with elevated privileges: the service manager hands the table over as a
// read-only file (systemd LoadCredential=smbios:/sys/firmware/dmi/tables/DMI), or
// the agent runs without it and reports the affected sections as unavailable.
package hardware

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/henrygd/beszel/internal/entities/hardware"
)

// Options controls a collection.
type Options struct {
	// Root is the filesystem root sysfs and procfs are read under ("/" by default).
	Root string
	// SMBIOS is the path of a raw SMBIOS table. Empty means
	// $CREDENTIALS_DIRECTORY/smbios, then <Root>/sys/firmware/dmi/tables/DMI.
	SMBIOS string
	// Identifiers keeps serial numbers, UUIDs and MAC addresses. Off by default.
	Identifiers bool
}

// Collect gathers the inventory. Sections that cannot be read are listed in
// Inventory.Unavailable; collection itself does not fail.
func Collect(opts Options) hardware.Inventory {
	if opts.Root == "" {
		opts.Root = "/"
	}
	inv := collect(opts)
	normalize(&inv)
	limit(&inv)
	if !opts.Identifiers {
		inv.StripIdentifiers()
	}
	return inv
}

// normalize sorts every list so the same hardware always yields the same inventory.
func normalize(inv *hardware.Inventory) {
	slices.SortFunc(inv.CPUs, func(a, b hardware.CPU) int { return cmp.Compare(a.Socket, b.Socket) })
	slices.SortFunc(inv.Memory, func(a, b hardware.MemoryModule) int {
		return cmp.Or(cmp.Compare(a.Bank, b.Bank), cmp.Compare(a.Locator, b.Locator))
	})
	slices.SortFunc(inv.PCI, func(a, b hardware.PCIDevice) int { return cmp.Compare(a.Address, b.Address) })
	slices.SortFunc(inv.USB, func(a, b hardware.USBDevice) int {
		return cmp.Or(cmp.Compare(a.Bus, b.Bus), cmp.Compare(a.Port, b.Port))
	})
	slices.SortFunc(inv.Disks, func(a, b hardware.Disk) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(inv.NICs, func(a, b hardware.NIC) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(inv.Displays, func(a, b hardware.Display) int { return cmp.Compare(a.Connector, b.Connector) })
}

// fill copies src into *dst when *dst is empty.
func fill(dst *string, src string) {
	if *dst == "" {
		*dst = src
	}
}

// maxEntries caps each device list, so an unusual host (hundreds of SR-IOV
// virtual functions, say) cannot flood the hub or the UI.
const maxEntries = 512

// limit truncates oversized lists and records how much was left out.
func limit(inv *hardware.Inventory) {
	inv.PCI = capList(inv, "pci", inv.PCI)
	inv.USB = capList(inv, "usb", inv.USB)
	inv.Disks = capList(inv, "disks", inv.Disks)
	inv.NICs = capList(inv, "nics", inv.NICs)
	inv.Memory = capList(inv, "memory", inv.Memory)
	inv.Displays = capList(inv, "displays", inv.Displays)
	inv.Batteries = capList(inv, "batteries", inv.Batteries)
}

func capList[T any](inv *hardware.Inventory, section string, list []T) []T {
	if len(list) <= maxEntries {
		return list
	}
	inv.Unavailable = append(inv.Unavailable, hardware.Gap{
		Section: section,
		Reason:  fmt.Sprintf("%d more entries not shown", len(list)-maxEntries),
	})
	return list[:maxEntries]
}
