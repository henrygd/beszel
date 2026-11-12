//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/henrygd/beszel/internal/records"

	"github.com/stretchr/testify/assert"
)

func TestAverageSystemStatsSlice_NilExtraFsEntry(t *testing.T) {
	input := []system.Stats{
		{ExtraFs: map[string]*system.FsStats{"bad": nil, "sda": {DiskTotal: 100, DiskUsed: 50}}},
		{ExtraFs: map[string]*system.FsStats{"sda": {DiskTotal: 100, DiskUsed: 70}}},
	}
	var result system.Stats
	assert.NotPanics(t, func() { result = records.AverageSystemStatsSlice(input) })
	assert.Equal(t, 60.0, result.ExtraFs["sda"].DiskUsed)
	assert.NotContains(t, result.ExtraFs, "bad")
}
