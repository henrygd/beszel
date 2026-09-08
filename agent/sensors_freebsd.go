//go:build freebsd

package agent

import (
	"context"

	"github.com/shirou/gopsutil/v4/sensors"
	"golang.org/x/sys/unix"
)

var getSensorTemps = func(ctx context.Context) ([]sensors.TemperatureStat, error) {
	return getFreeBSDSensorTemps(ctx, unix.SysctlUint32)
}
