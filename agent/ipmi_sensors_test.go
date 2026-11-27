//go:build testing

package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/henrygd/beszel/agent/ipmi"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/shirou/gopsutil/v4/sensors"
	"github.com/stretchr/testify/assert"
)

type fakeIPMI struct {
	readings ipmi.Readings
	waits    []time.Duration
}

func (f *fakeIPMI) Get(wait time.Duration) ipmi.Readings {
	f.waits = append(f.waits, wait)
	return f.readings
}

func stubSensorTemps(t *testing.T, temps []sensors.TemperatureStat, err error) {
	t.Helper()
	original := getSensorTemps
	t.Cleanup(func() { getSensorTemps = original })
	getSensorTemps = func(ctx context.Context) ([]sensors.TemperatureStat, error) {
		return temps, err
	}
}

func TestUpdateTemperaturesIPMI(t *testing.T) {
	stubSensorTemps(t, []sensors.TemperatureStat{{SensorKey: "cpu_tctl", Temperature: 61}}, nil)
	bmc := &fakeIPMI{readings: ipmi.Readings{Temperatures: []ipmi.Reading{
		{Name: "Inlet Temp", Value: 24.5},
		{Name: "PSU1 Temp", Value: 38},
		{Name: "Bogus Temp", Value: 255},
	}}}
	a := &Agent{sensorConfig: &SensorConfig{
		context:       context.Background(),
		timeout:       2 * time.Second,
		sensors:       parseSensorFilter("-PSU*"),
		primarySensor: "Inlet Temp",
		ipmi:          bmc,
	}}

	stats := &system.Stats{}
	a.updateTemperatures(stats)

	assert.Equal(t, map[string]float64{"cpu_tctl": 61, "Inlet Temp": 24.5}, stats.Temperatures)
	assert.Equal(t, 24.5, a.systemInfo.DashboardTemp, "PRIMARY_SENSOR can select a BMC sensor")
	assert.Equal(t, []time.Duration{2 * time.Second}, bmc.waits, "waits at most SENSORS_TIMEOUT for the BMC")
}

func TestUpdateTemperaturesIPMIWhenHostReadFails(t *testing.T) {
	original := getSensorTemps
	t.Cleanup(func() { getSensorTemps = original })
	getSensorTemps = func(ctx context.Context) ([]sensors.TemperatureStat, error) {
		time.Sleep(50 * time.Millisecond)
		return nil, nil
	}
	a := &Agent{sensorConfig: &SensorConfig{
		context: context.Background(),
		timeout: 10 * time.Millisecond,
		ipmi:    &fakeIPMI{readings: ipmi.Readings{Temperatures: []ipmi.Reading{{Name: "Inlet Temp", Value: 22}}}},
	}}

	stats := &system.Stats{Temperatures: map[string]float64{"stale": 50}}
	a.updateTemperatures(stats)

	assert.Equal(t, map[string]float64{"Inlet Temp": 22}, stats.Temperatures)
	assert.Equal(t, 22.0, a.systemInfo.DashboardTemp)
}

func TestUpdateTemperaturesIPMISkipCollection(t *testing.T) {
	stubSensorTemps(t, nil, errors.New("should not be called"))
	bmc := &fakeIPMI{readings: ipmi.Readings{Temperatures: []ipmi.Reading{{Name: "Inlet Temp", Value: 22}}}}
	a := &Agent{sensorConfig: &SensorConfig{skipCollection: true, ipmi: bmc}}

	stats := &system.Stats{}
	a.updateTemperatures(stats)

	assert.Nil(t, stats.Temperatures)
	assert.Empty(t, bmc.waits)
}

func TestUpdateFansIPMI(t *testing.T) {
	original := getFanSensors
	t.Cleanup(func() { getFanSensors = original })
	getFanSensors = func() ([]fanSensor, error) { return nil, nil }

	bmc := &fakeIPMI{readings: ipmi.Readings{Fans: []ipmi.Reading{
		{Name: "Fan 1", Value: 7199.6},
		{Name: "Fan 2", Value: 90000},
		{Name: "PSU1 Fan", Value: 12000},
	}}}
	a := &Agent{sensorConfig: &SensorConfig{
		timeout:    time.Second,
		fanSensors: parseSensorFilter("-PSU*"),
		ipmi:       bmc,
	}}

	var stats system.Stats
	a.updateFans(&stats)

	assert.Equal(t, map[string]uint16{"Fan 1": 7200, "Fan 2": 65535}, stats.Fans, "rounded and clamped to uint16")
}

func TestAddIPMIFansKeepsHostReading(t *testing.T) {
	config := &SensorConfig{ipmi: &fakeIPMI{readings: ipmi.Readings{Fans: []ipmi.Reading{
		{Name: "fan1", Value: 1000},
		{Name: "Fan 2", Value: 2000},
	}}}}
	fans := config.addIPMIFans(map[string]uint16{"fan1": 900})
	assert.Equal(t, map[string]uint16{"fan1": 900, "Fan 2": 2000}, fans)

	assert.Nil(t, (&SensorConfig{}).addIPMIFans(nil), "no BMC, no fans")
}

func TestUpdateFansIPMISkipFans(t *testing.T) {
	bmc := &fakeIPMI{readings: ipmi.Readings{Fans: []ipmi.Reading{{Name: "Fan 1", Value: 7200}}}}
	a := &Agent{sensorConfig: &SensorConfig{skipFans: true, ipmi: bmc}}

	var stats system.Stats
	a.updateFans(&stats)

	assert.Nil(t, stats.Fans)
	assert.Empty(t, bmc.waits)
}

func TestNewIPMISourceDisabled(t *testing.T) {
	t.Setenv("IPMI", "")
	assert.Nil(t, newIPMISource())
	t.Setenv("IPMI", "false")
	assert.Nil(t, newIPMISource())
}
