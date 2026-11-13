//go:build linux

package hardware

import (
	"strconv"

	"github.com/henrygd/beszel/internal/entities/hardware"
)

// secureBootVar is the UEFI global variable holding the Secure Boot state. Its
// efivarfs file is world-readable: 4 attribute bytes, then 1 value byte.
const secureBootVar = "sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"

func security(sys sysfs) hardware.Security {
	var s hardware.Security
	switch b, err := sys.bytes(secureBootVar, 5); {
	case !sys.exists("sys/firmware/efi"):
		s.SecureBoot = "legacy"
	case err == nil && len(b) == 5 && b[4] == 1:
		s.SecureBoot = "enabled"
	case err == nil && len(b) == 5:
		s.SecureBoot = "disabled"
	}
	switch major := sys.text("sys/class/tpm/tpm0/tpm_version_major"); {
	case major == "2":
		s.TPM = "2.0"
	case major == "1":
		s.TPM = "1.2"
	case sys.exists("sys/class/tpm/tpm0"):
		s.TPM = "present" // older kernels do not report the version
	}
	return s
}

// batteries lists system batteries. Peripherals report scope "Device" and are skipped.
func batteries(sys sysfs) []hardware.Battery {
	const dir = "sys/class/power_supply/"
	var out []hardware.Battery
	for _, name := range sys.list(dir) {
		p := dir + name + "/"
		if sys.text(p+"type") != "Battery" || sys.text(p+"scope") == "Device" {
			continue
		}
		cycles, _ := strconv.Atoi(sys.text(p + "cycle_count"))
		out = append(out, hardware.Battery{
			Name: name, Manufacturer: sys.text(p + "manufacturer"), Model: sys.text(p + "model_name"),
			Technology: sys.text(p + "technology"), CycleCount: cycles,
			HealthPct: health(sys, p), Serial: sys.text(p + "serial_number"),
		})
	}
	return out
}

// health compares full and design capacity, reported either as energy (µWh)
// or as charge (µAh) depending on the battery.
func health(sys sysfs, p string) int {
	for _, unit := range []string{"energy", "charge"} {
		full, _ := strconv.ParseFloat(sys.text(p+unit+"_full"), 64)
		design, _ := strconv.ParseFloat(sys.text(p+unit+"_full_design"), 64)
		if full > 0 && design > 0 {
			return int(full / design * 100)
		}
	}
	return 0
}
