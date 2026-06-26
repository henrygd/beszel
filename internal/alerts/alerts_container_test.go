//go:build testing

package alerts_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type containerAlertTestFixture struct {
	hub     *beszelTests.TestHub
	alertID string
	submit  func(containers []*container.Stats) error
}

func newContainerAlertTestFixture(t *testing.T, min int) *containerAlertTestFixture {
	return newContainerAlertTestFixtureFor(t, min, "")
}

func newContainerAlertTestFixtureFor(t *testing.T, min int, containerName string) *containerAlertTestFixture {
	t.Helper()

	hub, user := beszelTests.GetHubWithUser(t)

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	sysManagerSystem, err := hub.GetSystemManager().GetSystemFromStore(systemRecord.Id)
	require.NoError(t, err)
	require.NotNil(t, sysManagerSystem)
	sysManagerSystem.StopUpdater()

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	alertRecord, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":      "ContainerHealth",
		"system":    systemRecord.Id,
		"user":      user.Id,
		"min":       min,
		"value":     0,
		"container": containerName,
	})
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "Alert should not be triggered initially")

	return &containerAlertTestFixture{
		hub:     hub,
		alertID: alertRecord.Id,
		submit: func(containers []*container.Stats) error {
			_, err := sysManagerSystem.CreateRecords(&system.CombinedData{Containers: containers})
			return err
		},
	}
}

func (f *containerAlertTestFixture) assertTriggered(t *testing.T, triggered bool, msg string) {
	t.Helper()
	alertRecord, err := f.hub.FindRecordById("alerts", f.alertID)
	require.NoError(t, err)
	assert.Equal(t, triggered, alertRecord.GetBool("triggered"), msg)
}

func waitForContainerAlert(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func unhealthy(name string) *container.Stats {
	return &container.Stats{Name: name, Id: name + "id", Health: container.DockerHealthUnhealthy}
}

func healthy(name string) *container.Stats {
	return &container.Stats{Name: name, Id: name + "id", Health: container.DockerHealthHealthy}
}

// A container unhealthy for longer than the configured duration should trigger,
// and resolve once it recovers.
func TestContainerHealthAlertTriggersAndResolves(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newContainerAlertTestFixture(t, 1)
		defer fixture.hub.Cleanup()

		// first observation: unhealthy but duration is 0, should not trigger yet
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert should not trigger on first unhealthy observation")
		assert.Equal(t, 0, fixture.hub.TestMailer.TotalSend(), "No email yet")

		// still unhealthy after more than a minute: should trigger
		waitForContainerAlert(time.Minute + time.Second)
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, true, "Alert should trigger after unhealthy longer than 1 minute")
		assert.Equal(t, 1, fixture.hub.TestMailer.TotalSend(), "Triggered email should be sent")
		assert.Contains(t, fixture.hub.TestMailer.LastMessage().Text, "web", "Message should name the unhealthy container")

		// container recovers: should resolve
		require.NoError(t, fixture.submit([]*container.Stats{healthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert should resolve after container becomes healthy")
		assert.Equal(t, 2, fixture.hub.TestMailer.TotalSend(), "Resolved email should be sent")

		waitForContainerAlert(time.Minute)
	})
}

// A container that recovers before the duration elapses should never trigger.
func TestContainerHealthAlertDoesNotTriggerBeforeDuration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newContainerAlertTestFixture(t, 5)
		defer fixture.hub.Cleanup()

		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("web")}))
		waitForContainerAlert(time.Minute + time.Second)
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert should not trigger before 5 minutes")

		// recovers well before threshold
		require.NoError(t, fixture.submit([]*container.Stats{healthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert should remain untriggered after early recovery")
		assert.Equal(t, 0, fixture.hub.TestMailer.TotalSend(), "No email should be sent")

		waitForContainerAlert(time.Minute)
	})
}

// A per-container alert should only react to its own container, ignoring others.
func TestContainerHealthAlertSpecificContainer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newContainerAlertTestFixtureFor(t, 1, "web")
		defer fixture.hub.Cleanup()

		// a different container being unhealthy must not trigger the "web" alert
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("db"), healthy("web")}))
		waitForContainerAlert(time.Minute + time.Second)
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("db"), healthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert for 'web' should ignore other containers")
		assert.Equal(t, 0, fixture.hub.TestMailer.TotalSend(), "No email for unrelated container")

		// now "web" becomes unhealthy long enough -> triggers
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("db"), unhealthy("web")}))
		waitForContainerAlert(time.Minute + time.Second)
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("db"), unhealthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, true, "Alert should trigger when 'web' is unhealthy long enough")
		assert.Equal(t, 1, fixture.hub.TestMailer.TotalSend(), "One email for 'web'")
		assert.Contains(t, fixture.hub.TestMailer.LastMessage().Text, "web")

		// "web" recovers -> resolves (even though "db" is still unhealthy)
		require.NoError(t, fixture.submit([]*container.Stats{unhealthy("db"), healthy("web")}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert should resolve when 'web' recovers")
		assert.Equal(t, 2, fixture.hub.TestMailer.TotalSend(), "Resolved email for 'web'")

		waitForContainerAlert(time.Minute)
	})
}

// Stats without container Id (older agents, no health data) should be a no-op.
func TestContainerHealthAlertIgnoresStatsWithoutHealth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fixture := newContainerAlertTestFixture(t, 1)
		defer fixture.hub.Cleanup()

		// no Id => health unavailable
		require.NoError(t, fixture.submit([]*container.Stats{{Name: "web"}}))
		waitForContainerAlert(time.Minute + time.Second)
		require.NoError(t, fixture.submit([]*container.Stats{{Name: "web"}}))
		waitForContainerAlert(time.Second)
		fixture.assertTriggered(t, false, "Alert should never trigger without health data")
		assert.Equal(t, 0, fixture.hub.TestMailer.TotalSend(), "No email should be sent")

		waitForContainerAlert(time.Minute)
	})
}
