//go:build testing

package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTags(t *testing.T) {
	hub, testApp, err := createTestHub(t)
	require.NoError(t, err)
	defer cleanupTestHub(hub, testApp)
	bindTagsEvents(hub)

	t.Run("name is trimmed on create and update", func(t *testing.T) {
		tag, err := createTestRecord(hub, "tags", map[string]any{"name": "  Prod  ", "color": "red"})
		require.NoError(t, err)
		assert.Equal(t, "Prod", tag.GetString("name"))

		tag.Set("name", " Production ")
		require.NoError(t, hub.Save(tag))
		assert.Equal(t, "Production", tag.GetString("name"))
	})

	t.Run("names are unique ignoring case and whitespace", func(t *testing.T) {
		_, err := createTestRecord(hub, "tags", map[string]any{"name": "Staging"})
		require.NoError(t, err)
		_, err = createTestRecord(hub, "tags", map[string]any{"name": " staging "})
		assert.Error(t, err)
	})

	t.Run("color must be a known color", func(t *testing.T) {
		_, err := createTestRecord(hub, "tags", map[string]any{"name": "Colored", "color": "blue"})
		require.NoError(t, err)
		_, err = createTestRecord(hub, "tags", map[string]any{"name": "Bad color", "color": "#ff0000"})
		assert.Error(t, err)
	})

	t.Run("deleting a tag removes it from systems", func(t *testing.T) {
		user, err := createTestUser(hub)
		require.NoError(t, err)
		keep, err := createTestRecord(hub, "tags", map[string]any{"name": "Keep"})
		require.NoError(t, err)
		remove, err := createTestRecord(hub, "tags", map[string]any{"name": "Remove"})
		require.NoError(t, err)
		system, err := createTestRecord(hub, "systems", map[string]any{
			"name": "Tagged", "host": "localhost", "port": "45876", "status": "paused",
			"users": []string{user.Id}, "tags": []string{keep.Id, remove.Id},
		})
		require.NoError(t, err)

		require.NoError(t, hub.Delete(remove))

		system, err = hub.FindRecordById("systems", system.Id)
		require.NoError(t, err)
		assert.Equal(t, []string{keep.Id}, system.GetStringSlice("tags"))
	})
}
