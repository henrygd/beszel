package alerts

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/pocketbase/core"
)

// containerHealthAlertName is the alert type key for container health alerts.
const containerHealthAlertName = "ContainerHealth"

// containerHealthTracker records, per system, the time each container was first
// observed in an unhealthy state. State is kept in-memory and rebuilt naturally
// from incoming agent data, so it is reset (not restored) on hub restart. Access
// per system is already serialized by the system's update goroutine; the mutex
// guards concurrent access across different systems.
type containerHealthTracker struct {
	mu sync.Mutex
	// systemID -> containerName -> time first observed unhealthy
	unhealthySince map[string]map[string]time.Time
}

func newContainerHealthTracker() *containerHealthTracker {
	return &containerHealthTracker{unhealthySince: make(map[string]map[string]time.Time)}
}

// reconcile updates the tracked unhealthy containers for a system to match the
// current set of unhealthy container names, preserving the original "since" time
// for containers that remain unhealthy. It returns a map of currently unhealthy
// container names to the time they were first seen unhealthy.
func (t *containerHealthTracker) reconcile(systemID string, unhealthyNames []string, now time.Time) map[string]time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()

	prev := t.unhealthySince[systemID]
	current := make(map[string]time.Time, len(unhealthyNames))
	for _, name := range unhealthyNames {
		if since, ok := prev[name]; ok {
			current[name] = since
		} else {
			current[name] = now
		}
	}
	if len(current) == 0 {
		delete(t.unhealthySince, systemID)
	} else {
		t.unhealthySince[systemID] = current
	}
	return current
}

// clear removes all tracked container health state for a system.
func (t *containerHealthTracker) clear(systemID string) {
	t.mu.Lock()
	delete(t.unhealthySince, systemID)
	t.mu.Unlock()
}

// HandleContainerAlerts evaluates container health alerts for a system on each
// metrics update. It triggers an alert when any container has been unhealthy for
// at least the alert's configured duration (min minutes), and resolves it once
// no container on the system is unhealthy.
func (am *AlertManager) HandleContainerAlerts(systemRecord *core.Record, data *system.CombinedData) error {
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, containerHealthAlertName)
	if len(alerts) == 0 {
		am.containerHealth.clear(systemRecord.Id)
		return nil
	}

	// Health is only populated by CBOR-capable agents. Older agents send stats
	// without container Id/Health, in which case we skip evaluation.
	if len(data.Containers) == 0 || data.Containers[0].Id == "" {
		am.containerHealth.clear(systemRecord.Id)
		return nil
	}

	now := systemRecord.GetDateTime("updated").Time().UTC()

	var unhealthyNames []string
	for _, ctr := range data.Containers {
		if ctr.Health == container.DockerHealthUnhealthy {
			unhealthyNames = append(unhealthyNames, ctr.Name)
		}
	}

	unhealthy := am.containerHealth.reconcile(systemRecord.Id, unhealthyNames, now)
	systemName := systemRecord.GetString("name")

	for _, alertData := range alerts {
		min := max(1, int(alertData.Min))
		threshold := time.Duration(min) * time.Minute

		if alertData.Container == "" {
			// "any container" mode: trigger when any container has been unhealthy
			// long enough, resolve once every container has recovered.
			var offending []string
			for name, since := range unhealthy {
				if now.Sub(since) >= threshold {
					offending = append(offending, name)
				}
			}
			switch {
			case len(offending) > 0 && !alertData.Triggered:
				slices.Sort(offending)
				if err := am.sendContainerHealthAlert(systemRecord, systemName, alertData, offending, min, true); err != nil {
					am.hub.Logger().Error("Failed to send container alert", "err", err)
				}
			case len(unhealthy) == 0 && alertData.Triggered:
				if err := am.sendContainerHealthAlert(systemRecord, systemName, alertData, nil, min, false); err != nil {
					am.hub.Logger().Error("Failed to send container alert", "err", err)
				}
			}
			continue
		}

		// specific container mode: only watch the named container
		since, isUnhealthy := unhealthy[alertData.Container]
		longEnough := isUnhealthy && now.Sub(since) >= threshold
		switch {
		case longEnough && !alertData.Triggered:
			if err := am.sendContainerHealthAlert(systemRecord, systemName, alertData, []string{alertData.Container}, min, true); err != nil {
				am.hub.Logger().Error("Failed to send container alert", "err", err)
			}
		case !isUnhealthy && alertData.Triggered:
			// resolve once the container is healthy again or no longer present
			if err := am.sendContainerHealthAlert(systemRecord, systemName, alertData, []string{alertData.Container}, min, false); err != nil {
				am.hub.Logger().Error("Failed to send container alert", "err", err)
			}
		}
	}
	return nil
}

// sendContainerHealthAlert updates the alert's triggered state and notifies the user.
func (am *AlertManager) sendContainerHealthAlert(systemRecord *core.Record, systemName string, alertData CachedAlertData, offending []string, min int, triggered bool) error {
	if err := am.setAlertTriggered(alertData, triggered); err != nil {
		return err
	}

	minutesLabel := "minute"
	if min > 1 {
		minutesLabel += "s"
	}

	var title, message string
	if triggered {
		emoji := "\U0001F534" // red circle
		if alertData.Container != "" {
			title = fmt.Sprintf("%s container %s unhealthy %v", systemName, alertData.Container, emoji)
			message = fmt.Sprintf("%s unhealthy for over %d %s", alertData.Container, min, minutesLabel)
		} else {
			noun := "container"
			if len(offending) > 1 {
				noun = "containers"
			}
			title = fmt.Sprintf("%s has unhealthy %s %v", systemName, noun, emoji)
			message = fmt.Sprintf("Unhealthy for over %d %s: %s", min, minutesLabel, strings.Join(offending, ", "))
		}
	} else {
		emoji := "✅" // green checkmark
		if alertData.Container != "" {
			title = fmt.Sprintf("%s container %s healthy %v", systemName, alertData.Container, emoji)
			message = fmt.Sprintf("%s healthy", alertData.Container)
		} else {
			title = fmt.Sprintf("%s containers healthy %v", systemName, emoji)
			message = fmt.Sprintf("%s containers healthy", systemName)
		}
	}

	return am.SendAlert(AlertMessageData{
		UserID:   alertData.UserID,
		SystemID: systemRecord.Id,
		Title:    title,
		Message:  message,
		Link:     am.hub.MakeLink("system", systemRecord.Id),
		LinkText: "View " + systemName,
	})
}

// ClearContainerHealthState removes tracked container health state for a system.
// Called when a system is paused or removed to avoid leaking state.
func (am *AlertManager) ClearContainerHealthState(systemID string) {
	am.containerHealth.clear(systemID)
}
