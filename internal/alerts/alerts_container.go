package alerts

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cast"
)

// ContainerAlertData holds the data for a container alert
type ContainerAlertData struct {
	systemRecord    *core.Record
	containerRecord *core.Record
	alertRecord     *core.Record
	name            string
	unit            string
	val             float64
	threshold       float64
	triggered       bool
	time            time.Time
	count           uint8
	min             uint8
	descriptor      string // override descriptor in notification body
}

// ContainerAlertStats represents the stats structure for container alerts
type ContainerAlertStats struct {
	Cpu         float64 `json:"c"`
	Mem         float64 `json:"m"`
	NetworkSent float64 `json:"ns"`
	NetworkRecv float64 `json:"nr"`
	Status      string  `json:"s"`
	Health      string  `json:"h"`
}

// HandleContainerAlerts processes container alerts for a system
func (am *AlertManager) HandleContainerAlerts(systemRecord *core.Record, data *system.CombinedData) error {
	containers := data.Containers
	if len(containers) == 0 {
		return nil
	}

	// Get all container alerts for this system
	alertRecords, err := am.hub.FindAllRecords("container_alerts",
		dbx.NewExp("system={:system}", dbx.Params{"system": systemRecord.Id}),
	)
	if err != nil || len(alertRecords) == 0 {
		return nil
	}

	// Group alerts by container
	containerAlerts := make(map[string][]*core.Record)
	for _, alertRecord := range alertRecords {
		containerId := alertRecord.GetString("container")
		containerAlerts[containerId] = append(containerAlerts[containerId], alertRecord)
	}

	// Process alerts for each container
	for containerId, alerts := range containerAlerts {
		// Find the container in the current stats
		var containerStats *container.Stats
		for _, c := range containers {
			if c.Id == containerId {
				containerStats = c
				break
			}
		}

		// If container not found in current stats, it might be stopped
		// We'll still process Status and Health alerts
		if containerStats == nil {
			for _, alertRecord := range alerts {
				name := alertRecord.GetString("name")
				if name == "Status" || name == "Health" {
					// Container is missing, so it's likely stopped
					go am.sendContainerStatusAlert(alertRecord, systemRecord, containerId, "stopped")
				}
			}
			continue
		}

		// Process each alert for this container
		var validAlerts []ContainerAlertData
		now := time.Now().UTC()
		oldestTime := now

		for _, alertRecord := range alerts {
			name := alertRecord.GetString("name")
			var val float64
			unit := "%"

			switch name {
			case "CPU":
				val = containerStats.Cpu
			case "Memory":
				val = containerStats.Mem
			case "Network":
				val = containerStats.NetworkSent + containerStats.NetworkRecv
				unit = " MB/s"
			case "Status":
				// Status alert is handled differently - immediate trigger
				triggered := alertRecord.GetBool("triggered")
				isRunning := containerStats.Status == "running"
				if (!triggered && !isRunning) || (triggered && isRunning) {
					go am.sendContainerStatusAlert(alertRecord, systemRecord, containerId, containerStats.Status)
				}
				continue
			case "Health":
				// Health alert - trigger on unhealthy status
				triggered := alertRecord.GetBool("triggered")
				isHealthy := containerStats.Health == container.DockerHealthNone || containerStats.Health == container.DockerHealthHealthy
				if (!triggered && !isHealthy) || (triggered && isHealthy) {
					go am.sendContainerHealthAlert(alertRecord, systemRecord, containerId, containerStats.Health)
				}
				continue
			default:
				continue
			}

			triggered := alertRecord.GetBool("triggered")
			threshold := alertRecord.GetFloat("value")
			min := max(1, cast.ToUint8(alertRecord.Get("min")))

			alert := ContainerAlertData{
				systemRecord: systemRecord,
				alertRecord:  alertRecord,
				name:         name,
				unit:         unit,
				val:          val,
				threshold:    threshold,
				triggered:    triggered,
				min:          min,
			}

			// Send alert immediately if min is 1
			if min == 1 {
				alert.triggered = val > threshold
				go am.sendContainerAlert(alert, containerId, containerStats.Name)
				continue
			}

			alert.time = now.Add(-time.Duration(min) * time.Minute)
			if alert.time.Before(oldestTime) {
				oldestTime = alert.time
			}

			validAlerts = append(validAlerts, alert)
		}

		if len(validAlerts) == 0 {
			continue
		}

		// Fetch historical container stats
		containerStatsRecords := []struct {
			Stats   []byte         `db:"stats"`
			Created types.DateTime `db:"created"`
		}{}

		err = am.hub.DB().
			Select("stats", "created").
			From("container_stats").
			Where(dbx.NewExp(
				"system={:system} AND type='1m' AND created > {:created}",
				dbx.Params{
					"system":  systemRecord.Id,
					"created": oldestTime.Add(-time.Second * 90),
				},
			)).
			OrderBy("created").
			All(&containerStatsRecords)

		if err != nil || len(containerStatsRecords) == 0 {
			continue
		}

		oldestRecordTime := containerStatsRecords[0].Created.Time()

		// Filter valid alerts
		filteredAlerts := make([]ContainerAlertData, 0, len(validAlerts))
		for _, alert := range validAlerts {
			if alert.time.After(oldestRecordTime) {
				filteredAlerts = append(filteredAlerts, alert)
			}
		}
		validAlerts = filteredAlerts

		if len(validAlerts) == 0 {
			continue
		}

		// Process historical stats
		for i := range containerStatsRecords {
			stat := containerStatsRecords[i]
			systemStatsCreation := stat.Created.Time().Add(-time.Second * 10)

			// Parse container stats array
			var allContainerStats []*container.Stats
			if err := json.Unmarshal(stat.Stats, &allContainerStats); err != nil {
				continue
			}

			// Find this container in the stats
			var thisContainerStats *container.Stats
			for _, cs := range allContainerStats {
				if cs.Id == containerId {
					thisContainerStats = cs
					break
				}
			}

			if thisContainerStats == nil {
				continue
			}

			for j := range validAlerts {
				alert := &validAlerts[j]
				if i == 0 {
					alert.val = 0
				}
				if systemStatsCreation.Before(alert.time) {
					continue
				}

				switch alert.name {
				case "CPU":
					alert.val += thisContainerStats.Cpu
				case "Memory":
					alert.val += thisContainerStats.Mem
				case "Network":
					alert.val += thisContainerStats.NetworkSent + thisContainerStats.NetworkRecv
				default:
					continue
				}
				alert.count++
			}
		}

		// Calculate averages and send alerts
		for _, alert := range validAlerts {
			alert.val = alert.val / float64(alert.count)
			minCount := float32(alert.min) / 1.2

			if float32(alert.count) >= minCount {
				if !alert.triggered && alert.val > alert.threshold {
					alert.triggered = true
					go am.sendContainerAlert(alert, containerId, containerStats.Name)
				} else if alert.triggered && alert.val <= alert.threshold {
					alert.triggered = false
					go am.sendContainerAlert(alert, containerId, containerStats.Name)
				}
			}
		}
	}

	return nil
}

// sendContainerAlert sends a container alert notification
func (am *AlertManager) sendContainerAlert(alert ContainerAlertData, containerId, containerName string) {
	systemName := alert.systemRecord.GetString("name")

	var subject string
	if alert.triggered {
		subject = fmt.Sprintf("%s container %s %s above threshold", systemName, containerName, strings.ToLower(alert.name))
	} else {
		subject = fmt.Sprintf("%s container %s %s below threshold", systemName, containerName, strings.ToLower(alert.name))
	}

	minutesLabel := "minute"
	if alert.min > 1 {
		minutesLabel += "s"
	}

	descriptor := alert.name
	if alert.descriptor != "" {
		descriptor = alert.descriptor
	}

	body := fmt.Sprintf("%s averaged %.2f%s for the previous %v %s.", descriptor, alert.val, alert.unit, alert.min, minutesLabel)

	alert.alertRecord.Set("triggered", alert.triggered)
	if err := am.hub.Save(alert.alertRecord); err != nil {
		return
	}

	am.SendAlert(AlertMessageData{
		UserID:   alert.alertRecord.GetString("user"),
		SystemID: alert.systemRecord.Id,
		Title:    subject,
		Message:  body,
		Link:     am.hub.MakeLink("system", alert.systemRecord.Id, "containers", containerId),
		LinkText: "View " + containerName,
	})
}

// sendContainerStatusAlert sends a container status change alert
func (am *AlertManager) sendContainerStatusAlert(alertRecord, systemRecord *core.Record, containerId, status string) {
	triggered := alertRecord.GetBool("triggered")
	isRunning := status == "running"

	// Only send if state changed
	if (!triggered && isRunning) || (triggered && !isRunning) {
		return
	}

	systemName := systemRecord.GetString("name")

	// Try to get container name from containers collection
	containerName := containerId
	if containerRecord, err := am.hub.FindFirstRecordByFilter("containers", "id={:id}", dbx.Params{"id": containerId}); err == nil {
		containerName = containerRecord.GetString("name")
	}

	var subject string
	if isRunning {
		subject = fmt.Sprintf("%s container %s is now running", systemName, containerName)
	} else {
		subject = fmt.Sprintf("%s container %s has stopped", systemName, containerName)
	}

	body := fmt.Sprintf("Container status changed to: %s", status)

	alertRecord.Set("triggered", !isRunning)
	if err := am.hub.Save(alertRecord); err != nil {
		return
	}

	am.SendAlert(AlertMessageData{
		UserID:   alertRecord.GetString("user"),
		SystemID: systemRecord.Id,
		Title:    subject,
		Message:  body,
		Link:     am.hub.MakeLink("system", systemRecord.Id, "containers", containerId),
		LinkText: "View " + containerName,
	})
}

// sendContainerHealthAlert sends a container health status change alert
func (am *AlertManager) sendContainerHealthAlert(alertRecord, systemRecord *core.Record, containerId string, health container.DockerHealth) {
	triggered := alertRecord.GetBool("triggered")
	isHealthy := health == container.DockerHealthNone || health == container.DockerHealthHealthy

	// Only send if state changed
	if (!triggered && isHealthy) || (triggered && !isHealthy) {
		return
	}

	systemName := systemRecord.GetString("name")

	// Try to get container name from containers collection
	containerName := containerId
	if containerRecord, err := am.hub.FindFirstRecordByFilter("containers", "id={:id}", dbx.Params{"id": containerId}); err == nil {
		containerName = containerRecord.GetString("name")
	}

	var subject string
	if isHealthy {
		subject = fmt.Sprintf("%s container %s is now healthy", systemName, containerName)
	} else {
		subject = fmt.Sprintf("%s container %s is unhealthy", systemName, containerName)
	}

	// Convert DockerHealth to string
	healthStatus := "unknown"
	for str, val := range container.DockerHealthStrings {
		if val == health {
			healthStatus = str
			break
		}
	}
	body := fmt.Sprintf("Container health status changed to: %s", healthStatus)

	alertRecord.Set("triggered", !isHealthy)
	if err := am.hub.Save(alertRecord); err != nil {
		return
	}

	am.SendAlert(AlertMessageData{
		UserID:   alertRecord.GetString("user"),
		SystemID: systemRecord.Id,
		Title:    subject,
		Message:  body,
		Link:     am.hub.MakeLink("system", systemRecord.Id, "containers", containerId),
		LinkText: "View " + containerName,
	})
}
