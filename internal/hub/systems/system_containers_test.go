//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRecordsContainerCount(t *testing.T) {
	sys, app := newTestSystemWithHub(t)
	for _, tc := range []struct {
		name  string
		count int
	}{
		{"no containers", 0},
		{"two containers", 2},
		{"containers removed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := &system.CombinedData{Info: system.Info{Cpu: 12.5}}
			for range tc.count {
				data.Containers = append(data.Containers, &container.Stats{Name: "c"})
			}
			_, err := sys.createRecords(data)
			require.NoError(t, err)
			record, err := app.FindRecordById("systems", sys.Id)
			require.NoError(t, err)
			var info map[string]any
			require.NoError(t, record.UnmarshalJSONField("info", &info))
			if tc.count > 0 {
				assert.EqualValues(t, tc.count, info["cn"])
			} else {
				assert.NotContains(t, info, "cn")
			}
		})
	}
}
