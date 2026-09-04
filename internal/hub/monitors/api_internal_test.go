//go:build testing

package monitors

import (
	"testing"

	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"
)

func testSortApp(t *testing.T) (*pbtests.TestApp, *core.Record) {
	t.Helper()
	app, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "sort@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))

	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	mk := func(name string, paused bool) *core.Record {
		r := core.NewRecord(col)
		r.Set("name", name)
		r.Set("type", "ping")
		r.Set("target", "example.com")
		r.Set("interval", 60)
		r.Set("timeout", 10)
		r.Set("status", "up")
		r.Set("users", []string{user.Id})
		r.Set("paused", paused)
		require.NoError(t, app.Save(r))
		return r
	}
	_ = mk
	return app, user
}

func TestSortMonitorsByName(t *testing.T) {
	app, user := testSortApp(t)
	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	names := []string{"zeta", "alpha", "mid"}
	for _, n := range names {
		r := core.NewRecord(col)
		r.Set("name", n)
		r.Set("type", "ping")
		r.Set("target", "example.com")
		r.Set("interval", 60)
		r.Set("timeout", 10)
		r.Set("users", []string{user.Id})
		require.NoError(t, app.Save(r))
	}
	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	sortMonitorsByName(recs)
	want := []string{"alpha", "mid", "zeta"}
	for i, w := range want {
		if recs[i].GetString("name") != w {
			t.Fatalf("position %d: got %q, want %q", i, recs[i].GetString("name"), w)
		}
	}
}

func TestFilterMonitorRecords(t *testing.T) {
	app, user := testSortApp(t)
	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	mk := func(name string, users []string, paused bool) {
		r := core.NewRecord(col)
		r.Set("name", name)
		r.Set("type", "ping")
		r.Set("target", "example.com")
		r.Set("interval", 60)
		r.Set("timeout", 10)
		r.Set("users", users)
		r.Set("paused", paused)
		require.NoError(t, app.Save(r))
	}
	mk("mine", []string{user.Id}, false)
	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	other := core.NewRecord(users)
	other.Set("email", "other@example.com")
	other.Set("password", "password12345")
	require.NoError(t, app.Save(other))
	mk("theirs", []string{other.Id}, false)
	mk("paused-mine", []string{user.Id}, true)

	recs, err := app.FindAllRecords("monitors")
	require.NoError(t, err)
	if got := filterMonitorRecords(recs, user.Id, ""); len(got) != 2 {
		t.Fatalf("expected 2 records for member, got %d", len(got))
	}
	got := filterMonitorRecords(recs, user.Id, "true")
	if len(got) != 1 || got[0].GetString("name") != "paused-mine" {
		t.Fatalf("paused filter broken: %d records", len(got))
	}
	if got := filterMonitorRecords(recs, "nobody", ""); len(got) != 0 {
		t.Fatalf("expected 0 for stranger, got %d", len(got))
	}
}
