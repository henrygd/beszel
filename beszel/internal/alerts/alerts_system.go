package alerts

import (
	"beszel/internal/entities/system"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cast"
)

func (am *AlertManager) HandleSystemAlerts(systemRecord *core.Record, data *system.CombinedData) error {
	alertRecords, err := am.hub.FindAllRecords("alerts",
		dbx.NewExp("system={:system} AND name!='Status'", dbx.Params{"system": systemRecord.Id}),
	)
	if err != nil || len(alertRecords) == 0 {
		return nil
	}

	var validAlerts []SystemAlertData
	now := systemRecord.GetDateTime("updated").Time().UTC()
	oldestTime := now

	for _, alertRecord := range alertRecords {
		name := alertRecord.GetString("name")
		var val float64
		unit := "%"

		switch name {
		case "CPU":
			val = data.Info.Cpu
		case "Memory":
			val = data.Info.MemPct
		case "Bandwidth":
			val = data.Info.Bandwidth * 8 // Convert MB/s to Mbps
			unit = " Mbps"
		case "BandwidthUp":
			val = data.Stats.NetworkSent * 8 // Convert MB/s to Mbps for upload
			unit = " Mbps"
		case "BandwidthDown":
			val = data.Stats.NetworkRecv * 8 // Convert MB/s to Mbps for download  
			unit = " Mbps"
		case "Disk":
			// Check if this is a filesystem-specific alert
			filesystem := alertRecord.GetString("filesystem")
			if filesystem != "" {
				// This is a filesystem-specific alert
				if filesystem == "root" {
					val = data.Info.DiskPct
				} else {
					// Find the matching extra filesystem
					found := false
					for key, fs := range data.Stats.ExtraFs {
						if key == filesystem {
							val = fs.DiskUsed / fs.DiskTotal * 100
							found = true
							break
						}
					}
					if !found {
						continue // Filesystem not found, skip this alert
					}
				}
			} else {
				// Legacy disk alert - use the old behavior for backward compatibility
				maxUsedPct := data.Info.DiskPct
				for _, fs := range data.Stats.ExtraFs {
					usedPct := fs.DiskUsed / fs.DiskTotal * 100
					if usedPct > maxUsedPct {
						maxUsedPct = usedPct
					}
				}
				val = maxUsedPct
			}
		case "Temperature":
			if data.Info.DashboardTemp < 1 {
				continue
			}
			val = data.Info.DashboardTemp
			unit = "°C"
		case "LoadAvg1":
			val = data.Info.LoadAvg[0]
			unit = ""
		case "LoadAvg5":
			val = data.Info.LoadAvg[1]
			unit = ""
		case "LoadAvg15":
			val = data.Info.LoadAvg[2]
			unit = ""
		case "Swap":
			if data.Info.SwapPct == 0 && data.Stats.Swap == 0 {
				continue // Skip if no swap is configured
			}
			val = data.Info.SwapPct
			unit = "%"
		}

		triggered := alertRecord.GetBool("triggered")
		threshold := alertRecord.GetFloat("value")

		// CONTINUE
		// IF alert is not triggered and curValue is less than threshold
		// OR alert is triggered and curValue is greater than threshold
		if (!triggered && val <= threshold) || (triggered && val > threshold) {
			continue
		}

		min := max(1, cast.ToUint8(alertRecord.Get("min")))

		alert := SystemAlertData{
			systemRecord: systemRecord,
			alertRecord:  alertRecord,
			name:         name,
			unit:         unit,
			val:          val,
			threshold:    threshold,
			triggered:    triggered,
			min:          min,
		}

		// send alert immediately if min is 1 - no need to sum up values.
		if min == 1 {
			alert.triggered = val > threshold
			go am.sendSystemAlert(alert)
			continue
		}

		alert.time = now.Add(-time.Duration(min) * time.Minute)
		if alert.time.Before(oldestTime) {
			oldestTime = alert.time
		}

		validAlerts = append(validAlerts, alert)
	}

	systemStats := []struct {
		Stats   []byte         `db:"stats"`
		Created types.DateTime `db:"created"`
	}{}

	err = am.hub.DB().
		Select("stats", "created").
		From("system_stats").
		Where(dbx.NewExp(
			"system={:system} AND type='1m' AND created > {:created}",
			dbx.Params{
				"system": systemRecord.Id,
				// subtract some time to give us a bit of buffer
				"created": oldestTime.Add(-time.Second * 90),
			},
		)).
		OrderBy("created").
		All(&systemStats)
	if err != nil || len(systemStats) == 0 {
		return err
	}

	// get oldest record creation time from first record in the slice
	oldestRecordTime := systemStats[0].Created.Time()
	// log.Println("oldestRecordTime", oldestRecordTime.String())

	// Filter validAlerts to keep only those with time newer than oldestRecord
	filteredAlerts := make([]SystemAlertData, 0, len(validAlerts))
	for _, alert := range validAlerts {
		if alert.time.After(oldestRecordTime) {
			filteredAlerts = append(filteredAlerts, alert)
		}
	}
	validAlerts = filteredAlerts

	if len(validAlerts) == 0 {
		// log.Println("no valid alerts found")
		return nil
	}

	var stats SystemAlertStats

	// we can skip the latest systemStats record since it's the current value
	for i := range systemStats {
		stat := systemStats[i]
		// subtract 10 seconds to give a small time buffer
		systemStatsCreation := stat.Created.Time().Add(-time.Second * 10)
		if err := json.Unmarshal(stat.Stats, &stats); err != nil {
			return err
		}
		// log.Println("stats", stats)
		for j := range validAlerts {
			alert := &validAlerts[j]
			// reset alert val on first iteration
			if i == 0 {
				alert.val = 0
			}
			// continue if system_stats is older than alert time range
			if systemStatsCreation.Before(alert.time) {
				continue
			}
			// add to alert value
			switch alert.name {
			case "CPU":
				alert.val += stats.Cpu
			case "Memory":
				alert.val += stats.Mem
			case "Bandwidth":
				alert.val += (stats.NetSent + stats.NetRecv) * 8 // Convert MB/s to Mbps
			case "BandwidthUp":
				alert.val += stats.NetSent * 8 // Convert MB/s to Mbps for upload
			case "BandwidthDown":
				alert.val += stats.NetRecv * 8 // Convert MB/s to Mbps for download
			case "Disk":
				// Check if this is a filesystem-specific alert
				filesystem := alert.alertRecord.GetString("filesystem")
				if filesystem != "" {
					// Filesystem-specific alert
					if filesystem == "root" {
						alert.val += stats.Disk
					} else {
						// Find the matching extra filesystem
						for key, fs := range data.Stats.ExtraFs {
							if key == filesystem {
								alert.val += fs.DiskUsed / fs.DiskTotal * 100
								break
							}
						}
					}
				} else {
					// Legacy disk alert - use old behavior for backward compatibility
					if alert.mapSums == nil {
						alert.mapSums = make(map[string]float32, len(data.Stats.ExtraFs)+1)
					}
					// add root disk
					if _, ok := alert.mapSums["root"]; !ok {
						alert.mapSums["root"] = 0.0
					}
					alert.mapSums["root"] += float32(stats.Disk)
					// add extra disks
					for key, fs := range data.Stats.ExtraFs {
						if _, ok := alert.mapSums[key]; !ok {
							alert.mapSums[key] = 0.0
						}
						alert.mapSums[key] += float32(fs.DiskUsed / fs.DiskTotal * 100)
					}
				}
			case "Temperature":
				if alert.mapSums == nil {
					alert.mapSums = make(map[string]float32, len(stats.Temperatures))
				}
				for key, temp := range stats.Temperatures {
					if _, ok := alert.mapSums[key]; !ok {
						alert.mapSums[key] = float32(0)
					}
					alert.mapSums[key] += temp
				}
			case "LoadAvg1":
				alert.val += stats.LoadAvg[0]
			case "LoadAvg5":
				alert.val += stats.LoadAvg[1]
			case "LoadAvg15":
				alert.val += stats.LoadAvg[2]
			case "Swap":
				alert.val += stats.SwapPct
			default:
				continue
			}
			alert.count++
		}
	}
	// sum up vals for each alert
	for _, alert := range validAlerts {
		switch alert.name {
		case "Disk":
			// Check if this is a filesystem-specific alert
			filesystem := alert.alertRecord.GetString("filesystem")
			if filesystem != "" {
				// Filesystem-specific alert - handle normally like other alerts
				alert.val = alert.val / float64(alert.count)
				if (!alert.triggered && alert.val > alert.threshold) || (alert.triggered && alert.val <= alert.threshold) {
					alert.triggered = alert.val > alert.threshold
					alert.descriptor = fmt.Sprintf("Usage of %s", filesystem)
					go am.sendSystemAlert(alert)
				}
			} else {
				// Legacy disk alert - send separate alerts for each filesystem that exceeds threshold
				var alertedFilesystems []string
				for key, value := range alert.mapSums {
					avgPct := float64(value) / float64(alert.count)
					if avgPct > alert.threshold {
						alertedFilesystems = append(alertedFilesystems, key)
					}
				}

				// Send individual alerts for each filesystem above threshold
				for _, fsName := range alertedFilesystems {
					avgPct := float64(alert.mapSums[fsName]) / float64(alert.count)
					diskAlert := alert // Copy alert data
					diskAlert.descriptor = fmt.Sprintf("Usage of %s", fsName)
					diskAlert.val = avgPct
					diskAlert.triggered = avgPct > alert.threshold
					go am.sendSystemAlert(diskAlert)
				}
			}
			continue // Skip normal alert processing for disk alerts
		case "Temperature":
			maxTemp := float32(0)
			for key, value := range alert.mapSums {
				sumTemp := float32(value) / float32(alert.count)
				if sumTemp > maxTemp {
					maxTemp = sumTemp
					alert.descriptor = fmt.Sprintf("Highest sensor %s", key)
				}
			}
			alert.val = float64(maxTemp)
		default:
			alert.val = alert.val / float64(alert.count)
		}
		minCount := float32(alert.min) / 1.2
		// pass through alert if count is greater than or equal to minCount
		if float32(alert.count) >= minCount {
			if !alert.triggered && alert.val > alert.threshold {
				alert.triggered = true
				go am.sendSystemAlert(alert)
			} else if alert.triggered && alert.val <= alert.threshold {
				alert.triggered = false
				go am.sendSystemAlert(alert)
			}
		}
	}
	return nil
}

func (am *AlertManager) sendSystemAlert(alert SystemAlertData) {
	systemName := alert.systemRecord.GetString("name")

	// change Disk to Disk usage
	if alert.name == "Disk" {
		alert.name += " usage"
	}
	// format LoadAvg5 and LoadAvg15
	if after, ok := strings.CutPrefix(alert.name, "LoadAvg"); ok {
		alert.name = after + "m Load"
	}
	// format Bandwidth alerts
	if alert.name == "BandwidthUp" {
		alert.name = "Upload bandwidth"
	} else if alert.name == "BandwidthDown" {
		alert.name = "Download bandwidth"
	}

	// make title alert name lowercase if not CPU
	titleAlertName := alert.name
	if titleAlertName != "CPU" {
		titleAlertName = strings.ToLower(titleAlertName)
	}

	var subject string
	if alert.triggered {
		subject = fmt.Sprintf("%s %s above threshold", systemName, titleAlertName)
	} else {
		subject = fmt.Sprintf("%s %s below threshold", systemName, titleAlertName)
	}
	minutesLabel := "minute"
	if alert.min > 1 {
		minutesLabel += "s"
	}
	if alert.descriptor == "" {
		alert.descriptor = alert.name
	}
	body := fmt.Sprintf("%s averaged %.2f%s for the previous %v %s.", alert.descriptor, alert.val, alert.unit, alert.min, minutesLabel)

	alert.alertRecord.Set("triggered", alert.triggered)
	
	// Initialize repeat tracking when alert is first triggered
	if alert.triggered {
		alert.alertRecord.Set("repeat_count", 0)
		alert.alertRecord.Set("last_sent", types.NowDateTime())
	} else {
		// Reset repeat tracking when alert is resolved
		alert.alertRecord.Set("repeat_count", 0)
		alert.alertRecord.Set("last_sent", nil)
	}
	
	if err := am.hub.Save(alert.alertRecord); err != nil {
		return
	}
	am.SendAlert(AlertMessageData{
		UserID:   alert.alertRecord.GetString("user"),
		Title:    subject,
		Message:  body,
		Link:     am.hub.MakeLink("system", systemName),
		LinkText: "View " + systemName,
	})
}
