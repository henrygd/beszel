package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/henrygd/beszel/agent/utils"
)

// dmiGuestVendors are /sys/class/dmi/id/sys_vendor prefixes set by common hypervisors.
var dmiGuestVendors = []string{"QEMU", "VMware", "innotek GmbH", "Xen", "Parallels", "Bochs"}

// isVirtualGuest reports whether the agent runs inside an LXC container or a VM.
// Guests either see the host's hwmon tree (LXC) or synthetic sensors (VMs), so
// their temperature and fan readings don't describe the guest itself.
var isVirtualGuest = sync.OnceValue(func() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	return detectVirtualGuest("/")
})

func detectVirtualGuest(root string) bool {
	return isLxcGuest(root) || isVMGuest(root)
}

func isLxcGuest(root string) bool {
	// written by systemd inside the container; readable without root
	if utils.ReadStringFile(filepath.Join(root, "run/systemd/container")) == "lxc" {
		return true
	}
	environ, err := os.ReadFile(filepath.Join(root, "proc/1/environ"))
	return err == nil && strings.Contains(string(environ), "container=lxc")
}

func isVMGuest(root string) bool {
	// Xen dom0 exposes the hypervisor flag but is the physical host
	if caps, err := os.ReadFile(filepath.Join(root, "proc/xen/capabilities")); err == nil && strings.Contains(string(caps), "control_d") {
		return false
	}
	if cpuinfo, err := os.ReadFile(filepath.Join(root, "proc/cpuinfo")); err == nil {
		for line := range strings.SplitSeq(string(cpuinfo), "\n") {
			if strings.HasPrefix(line, "flags") && strings.Contains(line+" ", " hypervisor ") {
				return true
			}
		}
	}
	dmiDir := filepath.Join(root, "sys/class/dmi/id")
	vendor := utils.ReadStringFile(filepath.Join(dmiDir, "sys_vendor"))
	for _, v := range dmiGuestVendors {
		if strings.HasPrefix(vendor, v) {
			return true
		}
	}
	return vendor == "Microsoft Corporation" && utils.ReadStringFile(filepath.Join(dmiDir, "product_name")) == "Virtual Machine"
}
