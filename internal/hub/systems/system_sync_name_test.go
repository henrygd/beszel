//go:build testing

package systems

import (
	"testing"

	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRecordsSyncSystemNames(t *testing.T) {
	for _, tc := range []struct {
		name     string
		env      string
		hostname string
		expected string
	}{
		{"disabled", "", "new-host", "test-system"},
		{"enabled", "true", "new-host", "new-host"},
		{"enabled with empty hostname", "true", "", "test-system"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SYNC_SYSTEM_NAMES", tc.env)
			sys, app := newTestSystemWithHub(t)
			_, err := sys.createRecords(&system.CombinedData{Details: &system.Details{Hostname: tc.hostname}})
			require.NoError(t, err)
			record, err := app.FindRecordById("systems", sys.Id)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, record.GetString("name"))
		})
	}
}
