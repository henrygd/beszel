//go:build linux

package hardware

import (
	"slices"
	"strconv"
	"strings"

	"github.com/henrygd/beszel/internal/entities/hardware"
)

func pciDevices(sys sysfs) []hardware.PCIDevice {
	const dir = "sys/bus/pci/devices/"
	var out []hardware.PCIDevice
	for _, addr := range sys.list(dir) {
		p := dir + addr + "/"
		out = append(out, hardware.PCIDevice{
			Address: addr, Class: sys.hex(p + "class"),
			Vendor: sys.hex(p + "vendor"), Device: sys.hex(p + "device"),
			SubsystemVendor: sys.hex(p + "subsystem_vendor"), SubsystemDevice: sys.hex(p + "subsystem_device"),
			Driver: sys.linkBase(p + "driver"),
		})
	}
	return out
}

// usbKinds maps interface class (and HID protocol) codes to readable kinds.
var usbKinds = map[string]string{
	"01": "audio", "02": "communications", "06": "imaging", "07": "printer", "08": "storage",
	"09": "hub", "0a": "communications", "0b": "smartcard", "0e": "video", "e0": "wireless",
	"03/01": "keyboard", "03/02": "mouse", "03": "hid",
}

func usbDevices(sys sysfs) []hardware.USBDevice {
	const dir = "sys/bus/usb/devices/"
	names := sys.list(dir)
	var out []hardware.USBDevice
	for _, name := range names {
		p := dir + name + "/"
		vendor := sys.text(p + "idVendor")
		// Interfaces (names with ':') and Linux root hubs (vendor 1d6b) are not devices.
		if vendor == "" || vendor == "1d6b" || strings.Contains(name, ":") {
			continue
		}
		bus, _ := strconv.Atoi(sys.text(p + "busnum"))
		d := hardware.USBDevice{
			Bus: bus, Port: name, Vendor: vendor, Product: sys.text(p + "idProduct"),
			Manufacturer: sys.text(p + "manufacturer"), Name: sys.text(p + "product"),
			SpeedMbps: sys.text(p + "speed"), Serial: sys.text(p + "serial"),
		}
		for _, iface := range names {
			if !strings.HasPrefix(iface, name+":") {
				continue
			}
			ip := dir + iface + "/"
			class := sys.text(ip + "bInterfaceClass")
			kind := usbKinds[class+"/"+sys.text(ip+"bInterfaceProtocol")]
			if kind == "" {
				kind = usbKinds[class]
			}
			if kind != "" && !slices.Contains(d.Kinds, kind) {
				d.Kinds = append(d.Kinds, kind)
			}
		}
		slices.Sort(d.Kinds)
		out = append(out, d)
	}
	return out
}

func disks(sys sysfs) []hardware.Disk {
	const dir = "sys/block/"
	var out []hardware.Disk
	for _, name := range sys.list(dir) {
		p := dir + name + "/"
		// Only devices backed by hardware have a device link (skips loop, dm, zram, md).
		if !sys.exists(p + "device") {
			continue
		}
		sectors, _ := strconv.ParseUint(sys.text(p+"size"), 10, 64)
		firmware := sys.text(p + "device/firmware_rev")
		fill(&firmware, sys.text(p+"device/rev"))
		out = append(out, hardware.Disk{
			Name: name, Model: sys.text(p + "device/model"), Vendor: sys.text(p + "device/vendor"),
			SizeBytes: sectors * 512, Rotational: sys.text(p+"queue/rotational") == "1",
			Transport: transport(name, sys.link(dir+name)), Firmware: firmware,
			Serial: sys.text(p + "device/serial"),
		})
	}
	return out
}

// transport guesses the bus from the device name and its sysfs path.
func transport(name, target string) string {
	switch {
	case strings.HasPrefix(name, "nvme"):
		return "nvme"
	case strings.HasPrefix(name, "mmcblk"):
		return "mmc"
	case strings.Contains(target, "/usb"):
		return "usb"
	case strings.Contains(target, "/ata"):
		return "sata"
	case strings.Contains(target, "/virtio"):
		return "virtio"
	}
	return ""
}

func nics(sys sysfs) []hardware.NIC {
	const dir = "sys/class/net/"
	var out []hardware.NIC
	for _, name := range sys.list(dir) {
		p := dir + name + "/"
		if !sys.exists(p + "device") { // virtual interfaces have no device
			continue
		}
		speed, _ := strconv.Atoi(sys.text(p + "speed")) // -1 or unreadable when down
		pci := sys.linkBase(p + "device")
		if strings.Count(pci, ":") != 2 {
			pci = ""
		}
		out = append(out, hardware.NIC{
			Name: name, Driver: sys.linkBase(p + "device/driver"), PCI: pci,
			SpeedMbps: max(speed, 0), MAC: sys.text(p + "address"),
		})
	}
	return out
}

func displays(sys sysfs) []hardware.Display {
	const dir = "sys/class/drm/"
	var out []hardware.Display
	for _, name := range sys.list(dir) {
		card, connector, ok := strings.Cut(name, "-")
		if !ok || !strings.HasPrefix(card, "card") || sys.text(dir+name+"/status") != "connected" {
			continue
		}
		edid, err := sys.bytes(dir+name+"/edid", maxEDID)
		if err != nil || len(edid) == 0 {
			continue
		}
		d, err := parseEDID(edid)
		if err != nil {
			continue
		}
		d.Connector = connector
		out = append(out, d)
	}
	return out
}
