//go:build freebsd || testing

package agent

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/sensors"
)

const (
	freebsdZeroCelsiusDeciKelvin = 2731
	freebsdAcpiThermalZoneCount  = 16
)

type freebsdSysctlUintReader func(name string) (uint32, error)

func getFreeBSDSensorTemps(ctx context.Context, readSysctl freebsdSysctlUintReader) ([]sensors.TemperatureStat, error) {
	cpuCount, err := readSysctl("hw.ncpu")
	if err != nil {
		return nil, err
	}
	temps := make([]sensors.TemperatureStat, 0, int(cpuCount)+freebsdAcpiThermalZoneCount)
	for cpu := uint32(0); cpu < cpuCount; cpu++ {
		select {
		case <-ctx.Done():
			return temps, ctx.Err()
		default:
		}

		sensorName := fmt.Sprintf("dev.cpu.%d.temperature", cpu)
		value, err := readSysctl(sensorName)
		if err != nil {
			continue
		}
		temp, ok := freebsdDeciKelvinToCelsius(value)
		if !ok {
			continue
		}
		temps = append(temps, sensors.TemperatureStat{
			SensorKey:   sensorName,
			Temperature: temp,
		})
	}

	for zone := 0; zone < freebsdAcpiThermalZoneCount; zone++ {
		select {
		case <-ctx.Done():
			return temps, ctx.Err()
		default:
		}

		sensorName := fmt.Sprintf("hw.acpi.thermal.tz%d.temperature", zone)
		value, err := readSysctl(sensorName)
		if err != nil {
			continue
		}
		temp, ok := freebsdDeciKelvinToCelsius(value)
		if !ok {
			continue
		}
		temps = append(temps, sensors.TemperatureStat{
			SensorKey:   sensorName,
			Temperature: temp,
		})
	}

	return temps, nil
}

func freebsdDeciKelvinToCelsius(value uint32) (float64, bool) {
	if value <= freebsdZeroCelsiusDeciKelvin {
		return 0, false
	}
	temp := float64(int64(value)-freebsdZeroCelsiusDeciKelvin) / 10
	if temp <= 0 || temp >= 200 {
		return 0, false
	}
	return temp, true
}
