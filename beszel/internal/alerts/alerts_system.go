package alerts

import (
	"beszel/internal/entities/system"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cast"
)

func (am *AlertManager) HandleSystemAlerts(systemRecord *core.Record, data *system.CombinedData) error {
	alertRecords, err := am.app.FindAllRecords("alerts",
		dbx.NewExp("system={:system}", dbx.Params{"system": systemRecord.Id}),
	)
	if err != nil || len(alertRecords) == 0 {
		// log.Println("no alerts found for system")
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
			val = data.Info.Bandwidth
			unit = " MB/s"
		case "Disk":
			maxUsedPct := data.Info.DiskPct
			for _, fs := range data.Stats.ExtraFs {
				usedPct := fs.DiskUsed / fs.DiskTotal * 100
				if usedPct > maxUsedPct {
					maxUsedPct = usedPct
				}
			}
			val = maxUsedPct
		case "Temperature":
			if data.Stats.Temperatures == nil {
				continue
			}
			for _, temp := range data.Stats.Temperatures {
				if temp > val {
					val = temp
				}
			}
			unit = "°C"
		}

		triggered := alertRecord.GetBool("triggered")
		threshold := alertRecord.GetFloat("value")

		// CONTINUE
		// IF alert is not triggered and curValue is less than threshold
		// OR alert is triggered and curValue is greater than threshold
		if (!triggered && val <= threshold) || (triggered && val > threshold) {
			// log.Printf("Skipping alert %s: val %f | threshold %f | triggered %v\n", name, val, threshold, triggered)
			continue
		}

		min := max(1, cast.ToUint8(alertRecord.Get("min")))
		// add time to alert time to make sure it's slighty after record creation
		time := now.Add(-time.Duration(min) * time.Minute)
		if time.Before(oldestTime) {
			oldestTime = time
		}

		validAlerts = append(validAlerts, SystemAlertData{
			systemRecord: systemRecord,
			alertRecord:  alertRecord,
			name:         name,
			unit:         unit,
			val:          val,
			threshold:    threshold,
			triggered:    triggered,
			time:         time,
			min:          min,
		})
	}

	systemStats := []struct {
		Stats   []byte         `db:"stats"`
		Created types.DateTime `db:"created"`
	}{}

	err = am.app.DB().
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

	// delete from validAlerts if time is older than oldestRecord
	for i := range validAlerts {
		if validAlerts[i].time.Before(oldestRecordTime) {
			// log.Println("deleting alert - time is older than oldestRecord", validAlerts[i].name, oldestRecordTime, validAlerts[i].time)
			validAlerts = slices.Delete(validAlerts, i, i+1)
		}
	}

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
				alert.val += stats.NetSent + stats.NetRecv
			case "Disk":
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
			maxPct := float32(0)
			for key, value := range alert.mapSums {
				sumPct := float32(value)
				if sumPct > maxPct {
					maxPct = sumPct
					alert.descriptor = fmt.Sprintf("Usage of %s", key)
				}
			}
			alert.val = float64(maxPct / float32(alert.count))
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
		// log.Println("alert", alert.name, "val", alert.val, "threshold", alert.threshold, "triggered", alert.triggered)
		// log.Printf("%s: val %f | count %d | min-count %f | threshold %f\n", alert.name, alert.val, alert.count, minCount, alert.threshold)
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
	// log.Printf("Sending alert %s: val %f | count %d | threshold %f\n", alert.name, alert.val, alert.count, alert.threshold)
	systemName := alert.systemRecord.GetString("name")

	// change Disk to Disk usage
	if alert.name == "Disk" {
		alert.name += " usage"
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
	if err := am.app.Save(alert.alertRecord); err != nil {
		// app.Logger().Error("failed to save alert record", "err", err.Error())
		return
	}
	// expand the user relation and send the alert
	if errs := am.app.ExpandRecord(alert.alertRecord, []string{"user"}, nil); len(errs) > 0 {
		// app.Logger().Error("failed to expand user relation", "errs", errs)
		return
	}
	if user := alert.alertRecord.ExpandedOne("user"); user != nil {
		am.SendAlert(AlertMessageData{
			UserID:   user.Id,
			Title:    subject,
			Message:  body,
			Link:     am.app.Settings().Meta.AppURL + "/system/" + url.PathEscape(systemName),
			LinkText: "View " + systemName,
		})
	}
}
