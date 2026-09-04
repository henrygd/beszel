//go:build testing

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/henrygd/beszel/internal/hub/config"

	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	_ "github.com/henrygd/beszel/internal/migrations"
)

func writeConfigYAML(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0644))
}

func newConfigTestApp(t *testing.T) (*pbtests.TestApp, string) {
	t.Helper()
	app, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { app.Cleanup() })
	dir := app.DataDir()

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "yaml@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))
	return app, dir
}

func serveEventFor(app *pbtests.TestApp) *core.ServeEvent {
	return &core.ServeEvent{App: app}
}

func TestSyncMonitors_CreatesFromYAML(t *testing.T) {
	app, dir := newConfigTestApp(t)
	writeConfigYAML(t, dir, `
monitors:
  - name: web
    type: http
    target: https://example.com
    users: [yaml@example.com]
`)
	require.NoError(t, config.SyncMonitors(serveEventFor(app)))

	total, err := app.CountRecords("monitors")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)

	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	require.Equal(t, "web", recs[0].GetString("name"))
	require.Equal(t, "pending", recs[0].GetString("status"))
}

func TestSyncMonitors_NeverDeletesUIData(t *testing.T) {
	app, dir := newConfigTestApp(t)

	// UI-created monitor absent from YAML must survive the sync.
	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	u, err := app.FindAuthRecordByEmail("users", "yaml@example.com")
	require.NoError(t, err)
	_ = users
	ui := core.NewRecord(col)
	ui.Set("name", "ui-only")
	ui.Set("type", "ping")
	ui.Set("target", "example.com")
	ui.Set("interval", 60)
	ui.Set("timeout", 10)
	ui.Set("status", "up")
	ui.Set("users", []string{u.Id})
	require.NoError(t, app.Save(ui))

	writeConfigYAML(t, dir, `
monitors:
  - name: yaml-one
    type: dns
    target: example.com
    users: [yaml@example.com]
`)
	require.NoError(t, config.SyncMonitors(serveEventFor(app)))

	total, err := app.CountRecords("monitors")
	require.NoError(t, err)
	require.EqualValues(t, 2, total, "UI-created monitors must never be deleted by YAML sync")
}

func TestSyncMonitors_UpdatesExisting(t *testing.T) {
	app, dir := newConfigTestApp(t)
	writeConfigYAML(t, dir, `
monitors:
  - name: web
    type: http
    target: https://example.com
    interval: 60
    users: [yaml@example.com]
`)
	require.NoError(t, config.SyncMonitors(serveEventFor(app)))

	writeConfigYAML(t, dir, `
monitors:
  - name: web
    type: http
    target: https://example.com
    interval: 120
    users: [yaml@example.com]
`)
	require.NoError(t, config.SyncMonitors(serveEventFor(app)))

	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.EqualValues(t, 120, recs[0].GetFloat("interval"))
}
