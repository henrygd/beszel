//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/smart"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeNilEntries(t *testing.T) {
	data := &system.CombinedData{
		Containers: []*container.Stats{nil, {Id: "abc"}, nil},
		Stats: system.Stats{
			ExtraFs:  map[string]*system.FsStats{"bad": nil, "good": {DiskTotal: 1}},
			ZfsPools: map[string]*system.ZfsPool{"bad": nil},
		},
	}
	sanitizeNilEntries(data)
	assert.Len(t, data.Containers, 1)
	assert.Equal(t, "abc", data.Containers[0].Id)
	assert.Len(t, data.Stats.ExtraFs, 1)
	assert.Contains(t, data.Stats.ExtraFs, "good")
	assert.Empty(t, data.Stats.ZfsPools)

	// only nils: container slice becomes empty, so Containers[0] is never read
	data = &system.CombinedData{Containers: []*container.Stats{nil}}
	sanitizeNilEntries(data)
	assert.Empty(t, data.Containers)

	assert.NotPanics(t, func() { sanitizeNilEntries(nil) })
}

func TestExtractPowerMetricsNilAttribute(t *testing.T) {
	assert.NotPanics(t, func() {
		hours, cycles := extractPowerMetrics([]*smart.SmartAttribute{nil, {Name: "PowerOnHours", RawValue: 5}})
		assert.Equal(t, uint64(5), hours)
		assert.Equal(t, uint64(0), cycles)
	})
}
