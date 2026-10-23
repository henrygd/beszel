//go:build testing

package records_test

import (
	"testing"

	"github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

func TestPodCollectionsCreated(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	podStats, err := hub.FindCollectionByNameOrId("pod_stats")
	require.NoError(t, err)
	created, _ := podStats.Fields.GetByName("created").(*core.AutodateField)
	require.NotNil(t, created)
	require.True(t, created.OnCreate)
	require.NotEmpty(t, podStats.GetIndex("idx_pod_stats01"))

	user, err := tests.CreateUser(hub, "pods@example.com", "testtesttest")
	require.NoError(t, err)
	system, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":   "bazzite",
		"host":   "127.0.0.1",
		"port":   "45876",
		"status": "up",
		"users":  []string{user.Id},
	})
	require.NoError(t, err)

	_, err = tests.CreateRecord(hub, "pod_stats", map[string]any{
		"system": system.Id,
		"stats":  map[string]any{"pods": []any{}},
		"type":   "1m",
	})
	require.NoError(t, err)

	pods, err := hub.FindCollectionByNameOrId("pods")
	require.NoError(t, err)
	require.NotEmpty(t, pods.GetIndex("idx_pod_system"))
	_, err = tests.CreateRecord(hub, "pods", map[string]any{
		"system":    system.Id,
		"name":      "beszel",
		"namespace": "beszel",
		"status":    "Running",
		"updated":   1,
	})
	require.NoError(t, err)
}
