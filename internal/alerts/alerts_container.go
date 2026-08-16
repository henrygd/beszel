package alerts

import (
	"fmt"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/pocketbase/core"
)

const (
	// containerAlertName is the value stored in the alerts.name field for this alert type.
	containerAlertName = "ContainerHealth"

	// containerLogMaxLines caps how many matched (error/fatal) log lines are kept.
	containerLogMaxLines = 12
	// containerLogFallbackLines is how many trailing raw log lines are used when no
	// line matches "error" or "fatal", so the notification still carries some context.
	containerLogFallbackLines = 6
	// containerLogExcerptMaxChars bounds a single container's log excerpt so a
	// handful of containers can't blow past Discord's message size limit.
	containerLogExcerptMaxChars = 500
	// containerAlertMaxLogged is the max number of unhealthy containers we fetch
	// and embed logs for in a single alert message.
	containerAlertMaxLogged = 2
	// containerAlertMessageMaxChars is a final safety cap on the whole message body.
	containerAlertMessageMaxChars = 1800
)

// FetchContainerLogsFunc retrieves recent logs for a container ID from its
// connected agent. Implementations should apply their own timeout. This is a
// type alias (not a defined type) so it satisfies the hubLike interface in
// internal/hub/systems, which declares the same func signature without
// importing this package.
type FetchContainerLogsFunc = func(containerID string) (string, error)

// pendingContainerAlert tracks a "ContainerHealth" alert that is waiting out its
// configured "min minutes" delay before being sent, mirroring the status-alert
// pending mechanism but keeping its own state (list of unhealthy containers).
type pendingContainerAlert struct {
	systemName string
	alertData  CachedAlertData
	containers []*container.Stats
	expireTime time.Time
	timer      *time.Timer
}

// HandleContainerAlerts checks configured "ContainerHealth" alerts for a system
// against the Docker container health data included in the latest agent update.
// It schedules a pending alert when containers become unhealthy (honoring the
// alert's "min" minutes delay) and resolves/cancels it once containers recover.
// fetchLogs is used when an alert actually fires so the notification can include
// a log excerpt (prioritizing lines containing "error"/"fatal") for context.
func (am *AlertManager) HandleContainerAlerts(systemRecord *core.Record, data *system.CombinedData, fetchLogs FetchContainerLogsFunc) error {
	alerts := am.alertsCache.GetAlertsByName(systemRecord.Id, containerAlertName)
	if len(alerts) == 0 {
		return nil
	}

	var unhealthy []*container.Stats
	for _, c := range data.Containers {
		if c.Health == container.DockerHealthUnhealthy {
			unhealthy = append(unhealthy, c)
		}
	}

	systemName := systemRecord.GetString("name")
	for _, alertData := range alerts {
		if len(unhealthy) > 0 {
			am.schedulePendingContainerAlert(systemName, alertData, unhealthy, fetchLogs)
			continue
		}

		// no unhealthy containers right now
		if am.cancelPendingContainerAlert(alertData.Id) {
			// alert never actually fired, nothing to resolve
			continue
		}
		if !alertData.Triggered {
			continue
		}
		if err := am.sendContainerHealthAlert(false, systemName, alertData, nil, fetchLogs); err != nil {
			am.hub.Logger().Error("Failed to send container health alert", "err", err)
		}
	}
	return nil
}

// schedulePendingContainerAlert sets up (or refreshes) a timer to send a
// "container unhealthy" alert after the alert's configured delay, as long as at
// least one container is still unhealthy when the timer fires.
func (am *AlertManager) schedulePendingContainerAlert(systemName string, alertData CachedAlertData, unhealthy []*container.Stats, fetchLogs FetchContainerLogsFunc) {
	min := max(1, int(alertData.Min))
	pending := &pendingContainerAlert{
		systemName: systemName,
		alertData:  alertData,
		containers: unhealthy,
		expireTime: time.Now().Add(time.Duration(min) * time.Minute),
	}

	stored, loaded := am.pendingContainerAlerts.LoadOrStore(alertData.Id, pending)
	p := stored.(*pendingContainerAlert)
	if loaded {
		// timer already running; just keep the unhealthy-container list current
		p.containers = unhealthy
		return
	}
	p.timer = time.AfterFunc(time.Until(p.expireTime), func() {
		am.processPendingContainerAlert(alertData.Id, fetchLogs)
	})
}

// cancelPendingContainerAlert stops and removes a pending container alert timer.
// Returns true if a pending alert was found and cancelled.
func (am *AlertManager) cancelPendingContainerAlert(alertID string) bool {
	value, loaded := am.pendingContainerAlerts.LoadAndDelete(alertID)
	if !loaded {
		return false
	}
	if p, ok := value.(*pendingContainerAlert); ok && p.timer != nil {
		p.timer.Stop()
	}
	return true
}

// CancelPendingContainerAlerts cancels all pending container-health alert timers
// for a given system. Called when a system is paused so a delayed alert doesn't
// fire after monitoring stops.
func (am *AlertManager) CancelPendingContainerAlerts(systemID string) {
	am.pendingContainerAlerts.Range(func(key, value any) bool {
		if p, ok := value.(*pendingContainerAlert); ok && p.alertData.SystemID == systemID {
			am.cancelPendingContainerAlert(key.(string))
		}
		return true
	})
}

// processPendingContainerAlert sends the "unhealthy" alert once its delay has
// expired, unless the alert record was deleted or already triggered elsewhere.
func (am *AlertManager) processPendingContainerAlert(alertID string, fetchLogs FetchContainerLogsFunc) {
	value, loaded := am.pendingContainerAlerts.LoadAndDelete(alertID)
	if !loaded {
		return
	}
	pending := value.(*pendingContainerAlert)

	refreshedAlertData, ok := am.alertsCache.Refresh(pending.alertData)
	if !ok || refreshedAlertData.Triggered {
		return
	}
	if err := am.sendContainerHealthAlert(true, pending.systemName, refreshedAlertData, pending.containers, fetchLogs); err != nil {
		am.hub.Logger().Error("Failed to send container health alert", "err", err)
	}
}

// sendContainerHealthAlert updates the alert's triggered state and sends the
// notification. When unhealthy is true, it embeds a log excerpt (prioritizing
// error/fatal lines) for up to containerAlertMaxLogged of the affected containers.
func (am *AlertManager) sendContainerHealthAlert(unhealthy bool, systemName string, alertData CachedAlertData, containers []*container.Stats, fetchLogs FetchContainerLogsFunc) error {
	if err := am.setAlertTriggered(alertData, unhealthy); err != nil {
		return err
	}

	link := am.hub.MakeLink("system", alertData.SystemID)
	linkText := "View " + systemName

	if !unhealthy {
		title := fmt.Sprintf("Containers on %s are healthy again ✅", systemName)
		return am.SendAlert(AlertMessageData{
			UserID:   alertData.UserID,
			SystemID: alertData.SystemID,
			Title:    title,
			Message:  strings.TrimSuffix(title, " ✅"),
			Link:     link,
			LinkText: linkText,
		})
	}

	names := make([]string, len(containers))
	for i, c := range containers {
		names[i] = c.Name
	}

	var title string
	if len(names) == 1 {
		title = fmt.Sprintf("Container %s on %s is unhealthy \U0001F534", names[0], systemName)
	} else {
		title = fmt.Sprintf("%d containers on %s are unhealthy \U0001F534", len(names), systemName)
	}

	var body strings.Builder
	fmt.Fprintf(&body, "Unhealthy: %s", strings.Join(names, ", "))
	body.WriteString(am.buildContainerLogsSection(containers, fetchLogs))

	message := body.String()
	if len(message) > containerAlertMessageMaxChars {
		message = message[:containerAlertMessageMaxChars] + "\n…(truncated)"
	}

	return am.SendAlert(AlertMessageData{
		UserID:   alertData.UserID,
		SystemID: alertData.SystemID,
		Title:    title,
		Message:  message,
		Link:     link,
		LinkText: linkText,
	})
}

// buildContainerLogsSection fetches and formats log excerpts for up to
// containerAlertMaxLogged unhealthy containers, to append to an alert message.
func (am *AlertManager) buildContainerLogsSection(containers []*container.Stats, fetchLogs FetchContainerLogsFunc) string {
	if fetchLogs == nil {
		return ""
	}

	var section strings.Builder
	logged := 0
	for _, c := range containers {
		if logged >= containerAlertMaxLogged {
			break
		}
		rawLogs, err := fetchLogs(c.Id)
		if err != nil {
			am.hub.Logger().Warn("Failed to fetch container logs for alert", "container", c.Name, "err", err)
			continue
		}
		excerpt := buildContainerLogExcerpt(rawLogs)
		if excerpt == "" {
			continue
		}
		fmt.Fprintf(&section, "\n\n%s logs:\n```\n%s\n```", c.Name, excerpt)
		logged++
	}

	if len(containers) > containerAlertMaxLogged {
		fmt.Fprintf(&section, "\n\n(+%d more unhealthy container(s), logs omitted)", len(containers)-containerAlertMaxLogged)
	}

	return section.String()
}

// buildContainerLogExcerpt filters raw container log output down to the lines
// most likely to explain why the container is unhealthy: lines containing
// "error" or "fatal" (case-insensitive) are preferred. If none match, the tail
// of the raw output is used instead so the notification still carries context.
func buildContainerLogExcerpt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")

	var matched []string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") {
			matched = append(matched, line)
		}
	}

	selected := matched
	if len(selected) == 0 {
		start := max(0, len(lines)-containerLogFallbackLines)
		selected = lines[start:]
	} else if len(selected) > containerLogMaxLines {
		selected = selected[len(selected)-containerLogMaxLines:]
	}

	excerpt := strings.TrimSpace(strings.Join(selected, "\n"))
	if len(excerpt) > containerLogExcerptMaxChars {
		excerpt = "…" + excerpt[len(excerpt)-containerLogExcerptMaxChars:]
	}
	return excerpt
}
