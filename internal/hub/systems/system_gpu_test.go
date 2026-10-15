//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRecordsGPUUtilization(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	for _, tc := range []struct {
		name  string
		gpu   bool
		usage float64
	}{
		{"no GPU", false, 0},
		{"active GPU", true, 42.5},
		{"idle GPU", true, 0},
		{"GPU removed", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := &system.CombinedData{Info: system.Info{GpuPct: tc.usage, Cpu: 12.5}}
			if tc.gpu {
				data.Stats.GPUData = map[string]system.GPUData{"0": {Name: "GPU", Usage: tc.usage}}
			}
			_, err := sys.createRecords(data)
			require.NoError(t, err)
			record, err := app.FindRecordById("systems", sys.Id)
			require.NoError(t, err)
			var info map[string]any
			require.NoError(t, record.UnmarshalJSONField("info", &info))
			assert.Equal(t, 12.5, info["cpu"])
			if tc.gpu {
				assert.Equal(t, tc.usage, info["g"])
			} else {
				assert.NotContains(t, info, "g")
			}
		})
	}
}
