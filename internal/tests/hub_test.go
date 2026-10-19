//go:build testing

package tests

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

func TestClearCollection(t *testing.T) {
	hub, err := NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	_, err = CreateUser(hub, "clear@example.com", "password123")
	require.NoError(t, err)
	require.NoError(t, ClearCollection(t, hub, "users"))

	// An empty view can be counted, but SQLite rejects DELETE against it.
	view := core.NewViewCollection("empty_users")
	view.ViewQuery = "SELECT id FROM users"
	require.NoError(t, hub.Save(view))
	require.ErrorContains(t, ClearCollection(t, hub, "empty_users"), "cannot modify empty_users because it is a view")
}
