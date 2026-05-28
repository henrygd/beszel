//go:build freebsd

package agent

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/sensors"
	"golang.org/x/sys/unix"
)

var getSensorTemps getTempsFn = freebsdGetTemps

// freebsdGetTemps reads hardware temperatures from FreeBSD sysctls.
// CPU temps come from dev.cpu.N.temperature (coretemp/amdtemp drivers).
// System temps come from hw.acpi.thermal.tzN.temperature (ACPI thermal zones).
// Values are in deciKelvin; conversion: °C = val/10 - 273.15.
func freebsdGetTemps(_ context.Context) ([]sensors.TemperatureStat, error) {
	var temps []sensors.TemperatureStat

	for i := range 64 {
		val, err := unix.SysctlUint32(fmt.Sprintf("dev.cpu.%d.temperature", i))
		if err != nil {
			break
		}
		temps = append(temps, sensors.TemperatureStat{
			SensorKey:   fmt.Sprintf("cpu%d", i),
			Temperature: float64(val)/10.0 - 273.15,
		})
	}

	for i := range 16 {
		val, err := unix.SysctlUint32(fmt.Sprintf("hw.acpi.thermal.tz%d.temperature", i))
		if err != nil {
			break
		}
		temps = append(temps, sensors.TemperatureStat{
			SensorKey:   fmt.Sprintf("tz%d", i),
			Temperature: float64(val)/10.0 - 273.15,
		})
	}

	return temps, nil
}
