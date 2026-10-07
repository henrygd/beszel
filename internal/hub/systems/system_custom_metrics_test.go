//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Custom metric values are stored in custom_stats, one row per response that
// has them, and never in system_stats; their metadata is stored with the rest
// of the info in systems.info. A later response without them, from an older or
// unconfigured agent, writes no custom_stats row and must clear the agent's
// metadata, cmm. The info is saved through a
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
