//go:build testing

package migrations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldShapes describes a collection's fields without their ids, which differ
// between collections even when the fields are the same.
func fieldShapes(t *testing.T, collection *core.Collection) []map[string]any {
	t.Helper()
	shapes := make([]map[string]any, 0, len(collection.Fields))
	for _, field := range collection.Fields {
		raw, err := json.Marshal(field)
		require.NoError(t, err)
		var shape map[string]any
		require.NoError(t, json.Unmarshal(raw, &shape))
		delete(shape, "id")
		shapes = append(shapes, shape)
	}
	return shapes
}

// custom_stats is shaped like container_stats, so the hub writes, rolls up and
// deletes it, and the browser fetches it, the same way.
func TestCustomStatsShapedLikeContainerStats(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	custom, err := hub.FindCollectionByNameOrId("custom_stats")
	require.NoError(t, err)
	containerStats, err := hub.FindCollectionByNameOrId("container_stats")
	require.NoError(t, err)

	assert.Equal(t, containerStats.Type, custom.Type)
	assert.Equal(t, fieldShapes(t, containerStats), fieldShapes(t, custom))
	require.Len(t, custom.Indexes, 1)
	assert.Contains(t, custom.Indexes[0], "(`system`, `type`, `created`)")
}

func TestCustomStatsMigrationDown(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	var migration *core.Migration
	for _, m := range core.AppMigrations.Items() {
		if strings.HasSuffix(m.File, "_custom_stats.go") {
			migration = m
		}
	}
	require.NotNil(t, migration)

	require.NoError(t, migration.Down(hub))
	_, err = hub.FindCollectionByNameOrId("custom_stats")
	assert.Error(t, err, "down removes the collection")

	require.NoError(t, migration.Up(hub))
	_, err = hub.FindCollectionByNameOrId("custom_stats")
	assert.NoError(t, err, "up restores it")
}
