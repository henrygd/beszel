//go:build windows && testing

package agent

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nopWriteCloser struct{ strings.Builder }

func (*nopWriteCloser) Close() error { return nil }

func TestParseLHMFan(t *testing.T) {
	tests := []struct {
		name, value string
		wantName    string
		wantRPM     uint16
		wantOK      bool
	}{
		{"nct6687d_CPU Fan", "1200", "nct6687d_CPU Fan", 1200, true},
		{" nct6687d_Pump Fan ", " 2400.6 ", "nct6687d_Pump Fan", 2401, true},
		{"nct6687d_System Fan #1", "0", "nct6687d_System Fan #1", 0, true},
		{"nct6687d_Fan #7", "99999", "nct6687d_Fan #7", 65535, true},
		{"nct6687d_Fan #2", "-1", "", 0, false},
		{"nct6687d_Fan #3", "NaN", "", 0, false},
		{"nct6687d_Fan #4", "fast", "", 0, false},
		{"", "1200", "", 0, false},
	}
	for _, tt := range tests {
		name, rpm, ok := parseLHMFan(tt.name, tt.value)
		assert.Equal(t, tt.wantOK, ok, tt.value)
		assert.Equal(t, tt.wantName, name)
		assert.Equal(t, tt.wantRPM, rpm)
	}
}

func TestLHMGetTempsReadsFans(t *testing.T) {
	originalUseLHM := useLHM
	t.Cleanup(func() {
		useLHM = originalUseLHM
		lhmFans.take()
	})
	useLHM = true

	output := strings.Join([]string{
		"CPU Package|45.5",
		"fan|nct6687d_CPU Fan|1200",
		"fan|nct6687d_Pump Fan|2400.6",
		"fan|nct6687d_Fan #2|-1",
		"",
	}, "\n")
	lhm := &lhmProcess{
		isRunning: true,
		stdin:     &nopWriteCloser{},
		scanner:   bufio.NewScanner(strings.NewReader(output)),
	}

	temps, err := lhm.getTemps(context.Background())
	require.NoError(t, err)
	require.Len(t, temps, 1)
	assert.Equal(t, "CPU Package", temps[0].SensorKey)
	assert.Equal(t, 45.5, temps[0].Temperature)
	assert.Equal(t, map[string]uint16{"nct6687d_CPU Fan": 1200, "nct6687d_Pump Fan": 2401}, lhmFans.take())
}

func TestUpdateFansFromLHM(t *testing.T) {
	t.Cleanup(func() { lhmFans.take() })

	a := &Agent{sensorConfig: &SensorConfig{fanSensors: parseSensorFilter("-gpu-nvidia_*")}}
	lhmFans.set(map[string]uint16{"nct6687d_CPU Fan": 1200, "gpu-nvidia_GPU Fan": 1500})
	var stats system.Stats
	a.updateFans(&stats)
	assert.Equal(t, map[string]uint16{"nct6687d_CPU Fan": 1200}, stats.Fans)

	// readings are only reported once
	var next system.Stats
	a.updateFans(&next)
	assert.Nil(t, next.Fans)

	// FAN_SENSORS="" skips fans
	lhmFans.set(map[string]uint16{"nct6687d_CPU Fan": 1200})
	skip := &Agent{sensorConfig: &SensorConfig{skipFans: true}}
	var skipped system.Stats
	skip.updateFans(&skipped)
	assert.Nil(t, skipped.Fans)
}
