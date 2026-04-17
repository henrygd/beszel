//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/records"
	"github.com/henrygd/beszel/internal/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordManagerCreation tests RecordManager creation
func TestRecordManagerCreation(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	rm := records.NewRecordManager(hub)
	assert.NotNil(t, rm, "RecordManager should not be nil")
}

// TestTwoDecimals tests the twoDecimals helper function
func TestTwoDecimals(t *testing.T) {
	testCases := []struct {
		input    float64
		expected float64
	}{
		{1.234567, 1.23},
		{1.235, 1.24}, // Should round up
		{1.0, 1.0},
		{0.0, 0.0},
		{-1.234567, -1.23},
		{-1.235, -1.23}, // Negative rounding
	}

	for _, tc := range testCases {
		result := records.TwoDecimals(tc.input)
		assert.InDelta(t, tc.expected, result, 0.02, "twoDecimals(%f) should equal %f", tc.input, tc.expected)
	}
}
