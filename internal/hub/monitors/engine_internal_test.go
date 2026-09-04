//go:build testing

package monitors

import (
	"testing"
	"time"

	_ "github.com/henrygd/beszel/internal/migrations"

	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineInternal_ResendRefiresAfterWindow(t *testing.T) {
	app, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "re@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))

	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	mon := core.NewRecord(col)
	mon.Set("name", "refire")
	mon.Set("type", "ping")
	mon.Set("target", "example.com")
	mon.Set("interval", 60)
	mon.Set("timeout", 10)
	mon.Set("max_retries", 0)
	mon.Set("notify", true)
	mon.Set("resend_after", 60)
	mon.Set("status", "down")
	mon.Set("consecutive_failures", 1)
	mon.Set("users", []string{user.Id})
	require.NoError(t, app.Save(mon))

	var count int
	eng := NewEngine(app, func(userID, title, message, link string) { count++ })
	// Pretend the last DOWN notice was 2h ago: the next steady DOWN must
	// renotify even without a transition.
	eng.sentAt[mon.Id] = time.Now().Add(-2 * time.Hour)
	mr := recordToMonitor(mon)
	res := CheckResult{Status: StatusDown, Message: "still down"}
	eng.persistAndNotify(mr, res, 2, false)
	assert.Equal(t, 1, count, "steady DOWN past the resend window must renotify")

	// Inside the window: silent.
	eng.persistAndNotify(mr, res, 3, false)
	assert.Equal(t, 1, count, "steady DOWN inside the window must stay silent")
}

func TestEngineInternal_NoResendWhenZero(t *testing.T) {
	app, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCachedCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(users)
	user.Set("email", "nore@example.com")
	user.Set("password", "password12345")
	require.NoError(t, app.Save(user))

	col, err := app.FindCachedCollectionByNameOrId("monitors")
	require.NoError(t, err)
	mon := core.NewRecord(col)
	mon.Set("name", "noresend")
	mon.Set("type", "ping")
	mon.Set("target", "example.com")
	mon.Set("interval", 60)
	mon.Set("timeout", 10)
	mon.Set("max_retries", 0)
	mon.Set("notify", true)
	mon.Set("resend_after", 0)
	mon.Set("status", "down")
	mon.Set("users", []string{user.Id})
	require.NoError(t, app.Save(mon))

	var count int
	eng := NewEngine(app, func(userID, title, message, link string) { count++ })
	mr := recordToMonitor(mon)
	eng.persistAndNotify(mr, CheckResult{Status: StatusDown, Message: "x"}, 1, false)
	eng.persistAndNotify(mr, CheckResult{Status: StatusDown, Message: "x"}, 2, false)
	assert.Equal(t, 0, count, "resend_after=0 must never renotify steady DOWN")
}
