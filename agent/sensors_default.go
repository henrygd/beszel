//go:build !windows

package agent

import (
	"github.com/shirou/gopsutil/v4/sensors"
)

var getSensorTemps = sensors.TemperaturesWithContext
