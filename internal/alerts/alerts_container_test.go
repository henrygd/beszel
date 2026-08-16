//go:build testing

package alerts_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type containerAlertTestFixture struct {
	hub          *beszelTests.TestHub
	am           *alerts.AlertManager
	alertID      string
	systemRecord *core.Record
}

func newContainerAlertTestFixture(t *testing.T, min int) *containerAlertTestFixture {
	t.Helper()

	hub, user := beszelTests.GetHubWithUser(t)

	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "up")
	require.NoError(t, err)
	systemRecord := systems[0]

	userSettings, err := hub.FindFirstRecordByFilter("user_settings", "user={:user}", map[string]any{"user": user.Id})
	require.NoError(t, err)
	userSettings.Set("settings", `{"emails":["test@example.com"],"webhooks":[]}`)
	require.NoError(t, hub.Save(userSettings))

	alertRecord, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name":   "ContainerHealth",
		"system": systemRecord.Id,
		"user":   user.Id,
		"min":    min,
	})
	require.NoError(t, err)
	assert.False(t, alertRecord.GetBool("triggered"), "Alert should not be triggered initially")

	return &containerAlertTestFixture{
		hub:          hub,
		am:           alerts.NewTestAlertManagerWithoutWorker(hub),
		alertID:      alertRecord.Id,
		systemRecord: systemRecord,
	}
}

func (f *containerAlertTestFixture) cleanup() {
	f.hub.Cleanup()
}

func (f *containerAlertTestFixture) submit(t *testing.T, containers []*container.Stats, fetchLogs alerts.FetchContainerLogsFunc) {
	t.Helper()
	data := &system.CombinedData{Containers: containers}
	require.NoError(t, f.am.HandleContainerAlerts(f.systemRecord, data, fetchLogs))
}

func (f *containerAlertTestFixture) assertTriggered(t *testing.T, triggered bool, message string) {
	t.Helper()
	alertRecord, err := f.hub.FindRecordById("alerts", f.alertID)
	require.NoError(t, err)
	assert.Equal(t, triggered, alertRecord.GetBool("triggered"), message)
}

func waitForContainerAlert(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func healthyContainer(name string) *container.Stats {
	return &container.Stats{Name: name, Id: "abc123def456", Health: container.DockerHealthHealthy}
}

func unhealthyContainer(name string) *container.Stats {
	return &container.Stats{Name: name, Id: "abc123def456", Health: container.DockerHealthUnhealthy}
}

func TestContainerHealthAlertTriggersAndResolves(t *testing.T) {
	fixture := newContainerAlertTestFixture(t, 1)
	defer fixture.cleanup()

	synctest.Test(t, func(t *testing.T) {
		fixture.submit(t, []*container.Stats{unhealthyContainer("web")}, nil)
		waitForContainerAlert(time.Minute + time.Second)

		fixture.assertTriggered(t, true, "Alert should be triggered once a container is unhealthy past the min delay")
		require.Equal(t, 1, fixture.hub.TestMailer.TotalSend(), "An email should have been sent")

		msg := fixture.hub.TestMailer.LastMessage()
		assert.Contains(t, msg.Subject, "web", "Subject should name the unhealthy container")
		assert.Contains(t, msg.Subject, "unhealthy")

		fixture.submit(t, []*container.Stats{healthyContainer("web")}, nil)
		waitForContainerAlert(time.Second)

		fixture.assertTriggered(t, false, "Alert should resolve once the container is healthy again")
		assert.Equal(t, 2, fixture.hub.TestMailer.TotalSend(), "A second email should have been sent for the recovery")
		assert.Contains(t, fixture.hub.TestMailer.LastMessage().Subject, "healthy again")
	})
}

func TestContainerHealthAlertResolvesBeforeMinDelayCancelsPending(t *testing.T) {
	fixture := newContainerAlertTestFixture(t, 5)
	defer fixture.cleanup()

	synctest.Test(t, func(t *testing.T) {
		fixture.submit(t, []*container.Stats{unhealthyContainer("db")}, nil)
		waitForContainerAlert(time.Minute)

		fixture.assertTriggered(t, false, "Alert should not fire until the min delay elapses")
		assert.Equal(t, 1, fixture.am.GetPendingContainerAlertsCount(), "Alert should be pending")
		assert.Equal(t, 0, fixture.hub.TestMailer.TotalSend())

		// container recovers before the 5 minute delay elapses
		fixture.submit(t, []*container.Stats{healthyContainer("db")}, nil)
		waitForContainerAlert(10 * time.Minute)

		fixture.assertTriggered(t, false, "Alert should remain untriggered")
		assert.Equal(t, 0, fixture.am.GetPendingContainerAlertsCount(), "Pending alert should have been cancelled")
		assert.Equal(t, 0, fixture.hub.TestMailer.TotalSend(), "No email should be sent for a container that recovered before the delay")
	})
}

func TestContainerHealthAlertIncludesLogExcerpt(t *testing.T) {
	fixture := newContainerAlertTestFixture(t, 1)
	defer fixture.cleanup()

	rawLogs := strings.Join([]string{
		"2026-08-16T10:00:00Z booting",
		"2026-08-16T10:00:01Z ERROR could not reach upstream",
		"2026-08-16T10:00:02Z FATAL giving up after 3 retries",
	}, "\n")
	fetchLogs := func(containerID string) (string, error) {
		assert.Equal(t, "abc123def456", containerID)
		return rawLogs, nil
	}

	synctest.Test(t, func(t *testing.T) {
		fixture.submit(t, []*container.Stats{unhealthyContainer("api")}, fetchLogs)
		waitForContainerAlert(time.Minute + time.Second)

		fixture.assertTriggered(t, true, "Alert should be triggered")
		require.Equal(t, 1, fixture.hub.TestMailer.TotalSend())

		body := fixture.hub.TestMailer.LastMessage().Text
		assert.Contains(t, body, "could not reach upstream")
		assert.Contains(t, body, "giving up after 3 retries")
		assert.NotContains(t, body, "booting", "non error/fatal lines should be dropped when matches exist")
	})
}

func TestContainerHealthAlertSkipsLogsOnFetchError(t *testing.T) {
	fixture := newContainerAlertTestFixture(t, 1)
	defer fixture.cleanup()

	fetchLogs := func(containerID string) (string, error) {
		return "", fmt.Errorf("agent unreachable")
	}

	synctest.Test(t, func(t *testing.T) {
		fixture.submit(t, []*container.Stats{unhealthyContainer("api")}, fetchLogs)
		waitForContainerAlert(time.Minute + time.Second)

		fixture.assertTriggered(t, true, "Alert should still be triggered even if logs can't be fetched")
		require.Equal(t, 1, fixture.hub.TestMailer.TotalSend())
	})
}

func TestBuildContainerLogExcerptPrefersErrorAndFatalLines(t *testing.T) {
	raw := strings.Join([]string{
		"2026-08-16T10:00:00Z starting up",
		"2026-08-16T10:00:01Z listening on :8080",
		"2026-08-16T10:00:02Z ERROR failed to connect to db",
		"2026-08-16T10:00:03Z retrying connection",
		"2026-08-16T10:00:04Z FATAL could not recover, exiting",
	}, "\n")

	excerpt := alerts.BuildContainerLogExcerpt(raw)
	assert.Contains(t, excerpt, "failed to connect to db")
	assert.Contains(t, excerpt, "could not recover, exiting")
	assert.NotContains(t, excerpt, "starting up", "non-matching lines should be dropped when error/fatal lines exist")
}

func TestBuildContainerLogExcerptFallsBackToTailWhenNoMatches(t *testing.T) {
	var lines []string
	for i := range 20 {
		lines = append(lines, fmt.Sprintf("line %d: all good here", i))
	}
	raw := strings.Join(lines, "\n")

	excerpt := alerts.BuildContainerLogExcerpt(raw)
	assert.Contains(t, excerpt, "line 19", "should keep the tail of the output")
	assert.NotContains(t, excerpt, "line 0:", "should not keep the very start when falling back to a short tail")
}

func TestBuildContainerLogExcerptEmpty(t *testing.T) {
	assert.Equal(t, "", alerts.BuildContainerLogExcerpt("   \n  \n"))
}
