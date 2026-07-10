//go:build testing

package systems

import (
	"encoding/json"
	"testing"

	"github.com/henrygd/beszel/internal/entities/container"
	_ "github.com/henrygd/beszel/internal/migrations"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newContainerPersistenceTestApp(t *testing.T) *pbTests.TestApp {
	t.Helper()
	app, err := pbTests.NewTestAppWithConfig(core.BaseAppConfig{
		DataDir:       t.TempDir(),
		EncryptionEnv: "pb_test_env",
	})
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	return app
}

func createContainerPersistenceTestSystem(t *testing.T, app core.App) string {
	t.Helper()
	collection, err := app.FindCollectionByNameOrId("systems")
	require.NoError(t, err)
	record := core.NewRecord(collection)
	record.Set("name", "container persistence test")
	record.Set("host", "127.0.0.1")
	require.NoError(t, app.SaveNoValidate(record))
	return record.Id
}

func TestCreateContainerRecordsPersistsDiskIO(t *testing.T) {
	app := newContainerPersistenceTestApp(t)
	systemID := createContainerPersistenceTestSystem(t, app)

	diskIO := [2]uint64{123, 456}
	stats := &container.Stats{
		Id:        "abcdef",
		Name:      "nginx",
		Image:     "nginx:latest",
		Ports:     "80, 443",
		Status:    "Up 1 minute",
		Health:    container.DockerHealthHealthy,
		Cpu:       12.5,
		Mem:       256,
		Bandwidth: [2]uint64{1000, 2000},
		DiskIO:    &diskIO,
	}
	require.NoError(t, createContainerRecords(app, []*container.Stats{stats}, systemID))

	record, err := app.FindRecordById("containers", stats.Id)
	require.NoError(t, err)
	assert.True(t, record.GetBool("diskIo"))
	assert.Equal(t, float64(diskIO[0]), record.GetFloat("diskRead"))
	assert.Equal(t, float64(diskIO[1]), record.GetFloat("diskWrite"))
	assert.Equal(t, stats.Name, record.GetString("name"))
	assert.Equal(t, stats.Image, record.GetString("image"))
	assert.Equal(t, stats.Ports, record.GetString("ports"))
	assert.Equal(t, stats.Status, record.GetString("status"))
	assert.Equal(t, float64(stats.Health), record.GetFloat("health"))
	assert.Equal(t, stats.Cpu, record.GetFloat("cpu"))
	assert.Equal(t, stats.Mem, record.GetFloat("memory"))
	assert.Equal(t, float64(3000), record.GetFloat("net"))
}

func TestCreateContainerRecordsPersistsExplicitZeroDiskIO(t *testing.T) {
	app := newContainerPersistenceTestApp(t)
	systemID := createContainerPersistenceTestSystem(t, app)
	zeroDiskIO := [2]uint64{}
	stats := &container.Stats{Id: "abcdee", Name: "idle"}
	require.NoError(t, createContainerRecords(app, []*container.Stats{stats}, systemID))

	record, err := app.FindRecordById("containers", stats.Id)
	require.NoError(t, err)
	assert.False(t, record.GetBool("diskIo"))

	stats.DiskIO = &zeroDiskIO
	require.NoError(t, createContainerRecords(app, []*container.Stats{stats}, systemID))

	record, err = app.FindRecordById("containers", stats.Id)
	require.NoError(t, err)
	assert.True(t, record.GetBool("diskIo"))
	assert.Equal(t, float64(0), record.GetFloat("diskRead"))
	assert.Equal(t, float64(0), record.GetFloat("diskWrite"))
}

func TestCreateContainerRecordsNilDiskIOClearsPreviousValues(t *testing.T) {
	app := newContainerPersistenceTestApp(t)
	systemID := createContainerPersistenceTestSystem(t, app)
	diskIO := [2]uint64{100, 200}
	stats := &container.Stats{Id: "abcded", Name: "before", Cpu: 1, Mem: 2, DiskIO: &diskIO}
	require.NoError(t, createContainerRecords(app, []*container.Stats{stats}, systemID))

	stats.Name = "after"
	stats.Cpu = 3
	stats.Mem = 4
	stats.DiskIO = nil
	require.NoError(t, createContainerRecords(app, []*container.Stats{stats}, systemID))

	record, err := app.FindRecordById("containers", stats.Id)
	require.NoError(t, err)
	assert.False(t, record.GetBool("diskIo"))
	assert.Equal(t, float64(0), record.GetFloat("diskRead"))
	assert.Equal(t, float64(0), record.GetFloat("diskWrite"))
	assert.Equal(t, "after", record.GetString("name"))
	assert.Equal(t, float64(3), record.GetFloat("cpu"))
	assert.Equal(t, float64(4), record.GetFloat("memory"))
}

func TestCreateContainerRecordsAcceptsOldAgentPayload(t *testing.T) {
	app := newContainerPersistenceTestApp(t)
	systemID := createContainerPersistenceTestSystem(t, app)

	var stats container.Stats
	require.NoError(t, json.Unmarshal([]byte(`{"n":"old-agent","c":5,"m":10,"b":[20,30]}`), &stats))
	require.Nil(t, stats.DiskIO)
	stats.Id = "abcdec"
	require.NoError(t, createContainerRecords(app, []*container.Stats{&stats}, systemID))

	record, err := app.FindRecordById("containers", stats.Id)
	require.NoError(t, err)
	assert.Equal(t, "old-agent", record.GetString("name"))
	assert.False(t, record.GetBool("diskIo"))
	assert.Equal(t, float64(0), record.GetFloat("diskRead"))
	assert.Equal(t, float64(0), record.GetFloat("diskWrite"))
}
