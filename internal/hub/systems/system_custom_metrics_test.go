//go:build testing

package systems

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Custom metric values are stored in custom_stats, one row per response that
// has them, and never in system_stats; their metadata is stored with the rest
// of the info in systems.info. A later response without them, from an older or
// unconfigured agent, writes no custom_stats row and must clear the agent's
// metadata, cmm; the hub then keeps the names as cmr (see
// TestCreateRecordsRetainsCustomMetricNames). The info is saved through a
// wrapper struct that adds the GPU percentage only for systems with GPU data,
// so both shapes are checked.
func TestCreateRecordsCustomMetrics(t *testing.T) {
	values := map[string]float64{"pi_power_board_watts": 1.84, "myapp_ratio": 0.0012}
	meta := map[string]system.CustomMetricMeta{
		"pi_power_board_watts": {
			Unit: "watts", Help: "Board power.", DisplayName: "Board power",
			Chart: "Power consumption", ChartDescription: "Measured board power",
		},
		"myapp_ratio": {Unit: "ratio", DisplayName: "myapp_ratio", Chart: "Myapp ratio"},
	}
	for _, tt := range []struct {
		name string
		gpu  map[string]system.GPUData
	}{
		{"without GPU data", nil},
		{"with GPU data", map[string]system.GPUData{"0": {Name: "GPU 0"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sys, app := newTestSystemWithHub(t)

			_, err := sys.createRecords(&system.CombinedData{
				Stats: system.Stats{CustomMetrics: values, GPUData: tt.gpu},
				Info:  system.Info{CustomMetricsMeta: meta},
			})
			require.NoError(t, err)

			customRows, err := app.FindAllRecords("custom_stats")
			require.NoError(t, err)
			require.Len(t, customRows, 1)
			assert.Equal(t, sys.Id, customRows[0].GetString("system"))
			assert.Equal(t, "1m", customRows[0].GetString("type"))
			var stored map[string]float64
			require.NoError(t, customRows[0].UnmarshalJSONField("stats", &stored))
			assert.Equal(t, values, stored, "unrounded")

			rows, err := app.FindAllRecords("system_stats")
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.NotContains(t, rows[0].GetString("stats"), `"cm"`, "values are not stored twice")

			record, err := app.FindRecordById("systems", sys.Id)
			require.NoError(t, err)
			var info system.Info
			require.NoError(t, record.UnmarshalJSONField("info", &info))
			assert.Equal(t, meta, info.CustomMetricsMeta)

			_, err = sys.createRecords(&system.CombinedData{
				Stats: system.Stats{Cpu: 1, GPUData: tt.gpu},
				Info:  system.Info{AgentVersion: "0.21.0"},
			})
			require.NoError(t, err)

			customRows, err = app.FindAllRecords("custom_stats")
			require.NoError(t, err)
			assert.Len(t, customRows, 1, "no custom_stats row without values")
			rows, err = app.FindAllRecords("system_stats")
			require.NoError(t, err)
			require.Len(t, rows, 2)
			for _, row := range rows {
				assert.NotContains(t, row.GetString("stats"), `"cm"`)
			}

			record, err = app.FindRecordById("systems", sys.Id)
			require.NoError(t, err)
			assert.NotContains(t, record.GetString("info"), `"cmm"`, "the agent's metadata cleared")
			assert.Contains(t, record.GetString("info"), `"v":"0.21.0"`, "the rest of the info saved")
			if tt.gpu != nil {
				assert.Contains(t, record.GetString("info"), `"g":`, "the GPU percentage still saved")
			}
		})
	}
}

// When the agent stops reporting a series, the hub keeps its metadata as cmr,
// so the series' stored history keeps its title and unit. A series reported
// again leaves cmr, and a system that never had custom metrics gets no cmr.
func TestCreateRecordsRetainsCustomMetricNames(t *testing.T) {
	board := system.CustomMetricMeta{Unit: "watts", DisplayName: "Board power", Chart: "Power consumption"}
	wall := system.CustomMetricMeta{Unit: "watts", DisplayName: "Wall power (est)", Chart: "Power consumption"}
	sys, app := newTestSystemWithHub(t)

	stored := func() (cmm map[string]system.CustomMetricMeta, cmr map[string]RetiredCustomMetric, raw string) {
		t.Helper()
		record, err := app.FindRecordById("systems", sys.Id)
		require.NoError(t, err)
		var info struct {
			Reported map[string]system.CustomMetricMeta `json:"cmm"`
			Retired  map[string]RetiredCustomMetric     `json:"cmr"`
		}
		require.NoError(t, record.UnmarshalJSONField("info", &info))
		return info.Reported, info.Retired, record.GetString("info")
	}
	report := func(meta map[string]system.CustomMetricMeta) {
		t.Helper()
		_, err := sys.createRecords(&system.CombinedData{Stats: system.Stats{Cpu: 1}, Info: system.Info{CustomMetricsMeta: meta}})
		require.NoError(t, err)
	}

	report(nil)
	_, _, raw := stored()
	assert.NotContains(t, raw, `"cmr"`, "no custom metrics, no cmr")

	report(map[string]system.CustomMetricMeta{"board": board, "wall": wall})
	cmm, cmr, _ := stored()
	assert.Len(t, cmm, 2)
	assert.Nil(t, cmr)

	before := time.Now().Unix()
	report(map[string]system.CustomMetricMeta{"board": board})
	cmm, cmr, _ = stored()
	assert.Equal(t, map[string]system.CustomMetricMeta{"board": board}, cmm)
	require.Contains(t, cmr, "wall")
	assert.Equal(t, wall, cmr["wall"].CustomMetricMeta)
	assert.GreaterOrEqual(t, cmr["wall"].LastReported, before)

	report(nil)
	_, cmr, _ = stored()
	assert.Equal(t, []string{"board", "wall"}, slices.Sorted(maps.Keys(cmr)), "kept across updates")
	assert.Equal(t, board, cmr["board"].CustomMetricMeta)

	report(map[string]system.CustomMetricMeta{"wall": wall})
	cmm, cmr, _ = stored()
	assert.Contains(t, cmm, "wall")
	assert.Equal(t, []string{"board"}, slices.Sorted(maps.Keys(cmr)), "reported again, so no longer kept")
}
