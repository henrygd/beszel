//go:build testing

package agent

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
)

func TestDropNonFinite(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	data := &system.CombinedData{
		Stats: system.Stats{
			Cpu:          inf,
			Mem:          12.5,
			LoadAvg:      [3]float64{1, nan, 3},
			Temperatures: map[string]float64{"gpu": nan, "cpu": 40},
			ExtraFs:      map[string]*system.FsStats{"data": {DiskTotal: nan}},
			GPUData:      map[string]system.GPUData{"0": {Name: "A", Power: nan, Usage: 55}},
		},
		Info: system.Info{DashboardTemp: math.Inf(-1)},
	}
	if _, err := json.Marshal(data.Stats); err == nil {
		t.Fatal("precondition: NaN stats should not JSON-encode")
	}

	a := &Agent{}
	a.dropNonFinite(data)

	if _, err := json.Marshal(data); err != nil {
		t.Fatalf("stats still not encodable: %v", err)
	}
	s := data.Stats
	if s.Cpu != 0 || s.Mem != 12.5 || s.LoadAvg != [3]float64{1, 0, 3} {
		t.Errorf("scalar/array: cpu=%v mem=%v la=%v", s.Cpu, s.Mem, s.LoadAvg)
	}
	if s.Temperatures["gpu"] != 0 || s.Temperatures["cpu"] != 40 {
		t.Errorf("temperatures: %v", s.Temperatures)
	}
	if s.ExtraFs["data"].DiskTotal != 0 {
		t.Errorf("extra fs: %v", s.ExtraFs["data"].DiskTotal)
	}
	if g := s.GPUData["0"]; g.Power != 0 || g.Usage != 55 || g.Name != "A" {
		t.Errorf("gpu: %+v", g)
	}
	if data.Info.DashboardTemp != 0 {
		t.Errorf("info: %v", data.Info.DashboardTemp)
	}
	want := []string{
		"info.DashboardTemp",
		"stats.Cpu",
		"stats.ExtraFs[data].DiskTotal",
		"stats.GPUData[0].Power",
		"stats.LoadAvg[1]",
		"stats.Temperatures[gpu]",
	}
	if got := strings.Split(a.lastNonFinite, ","); !slices.Equal(got, want) {
		t.Errorf("logged fields = %v, want %v", got, want)
	}

	// A clean payload clears the remembered set so a recurrence logs again.
	a.dropNonFinite(&system.CombinedData{})
	if a.lastNonFinite != "" {
		t.Errorf("lastNonFinite = %q after clean payload", a.lastNonFinite)
	}
}
