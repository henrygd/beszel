//go:build freebsd

package agent

import (
	"context"

	"github.com/shirou/gopsutil/v4/sensors"
	"golang.org/x/sys/unix"
)

func getSensorTemps(ctx context.Context) ([]sensors.TemperatureStat, error) {
	return getFreeBSDSensorTemps(ctx, unix.SysctlUint32)
}
