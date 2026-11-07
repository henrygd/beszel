//go:build testing

package hub_test

import (
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stack and service columns arrive by migration, not in the 0.20.0 snapshot, so this checks
// that a hub starting on a fresh database ends up with them and with the index that makes
// "containers of this stack on this system" cheap.
func TestContainersCollectionHasStackFields(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	collection, err := hub.FindCollectionByNameOrId("containers")
	require.NoError(t, err)

	for _, name := range []string{"stack", "service"} {
		field := collection.Fields.GetByName(name)
		require.NotNil(t, field, "containers.%s is missing", name)
		assert.Equal(t, "text", field.Type())
	}
	assert.Contains(t, collection.Indexes, "CREATE INDEX `idx_containers_stack` ON `containers` (`system`, `stack`)")
}
