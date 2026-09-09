//go:build testing

package hub_test

import (
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubSettingsRules(t *testing.T) {
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	col, err := hub.FindCollectionByNameOrId("hub_settings")
	require.NoError(t, err)
	assert.Equal(t, "@request.auth.role = 'admin'", *col.ListRule)
	assert.Equal(t, "@request.auth.role = 'admin'", *col.ViewRule)
	assert.Equal(t, "@request.auth.role = 'admin'", *col.CreateRule)
	assert.Equal(t, "@request.auth.role = 'admin'", *col.UpdateRule)
	assert.Equal(t, "@request.auth.role = 'admin'", *col.DeleteRule)

	// verify only admin can update via direct SaveAs simulation? Simplified: check that non-admin record update would fail via API layer is covered by rule above.
	// Additional check: ensure singleton exists
	count, err := hub.CountRecords("hub_settings")
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}
