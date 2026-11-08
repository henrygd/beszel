//go:build !linux

package hardware

import (
	"runtime"
	"time"

	"github.com/henrygd/beszel/internal/entities/hardware"
)

func collect(Options) hardware.Inventory {
	return hardware.Inventory{
		Collected:   time.Now().Unix(),
		Unavailable: []hardware.Gap{{Section: "all", Reason: "hardware inventory is not implemented on " + runtime.GOOS}},
	}
}
