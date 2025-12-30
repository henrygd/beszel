// Package records handles creating longer records and deleting old records.
package records

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/kubernetes"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type RecordManager struct {
	app core.App
}

type LongerRecordData struct {
	shorterType        string
	longerType         string
	longerTimeDuration time.Duration
	minShorterRecords  int
}

type RecordIds []struct {
	Id string `db:"id"`
}

func NewRecordManager(app core.App) *RecordManager {
	return &RecordManager{app}
}

type StatsRecord struct {
	Stats []byte `db:"stats"`
}

// global variables for reusing allocations
var (
	statsRecord    StatsRecord
	containerStats []container.Stats
	podStats       []kubernetes.PodStats
	sumStats       system.Stats
	tempStats      system.Stats
	queryParams    = make(dbx.Params, 1)
	containerSums  = make(map[string]*container.Stats)
	podSums        = make(map[string]*kubernetes.PodStats)
)

// Create longer records by averaging shorter records
func (rm *RecordManager) CreateLongerRecords() {
	// start := time.Now()
	longerRecordData := []LongerRecordData{
		{
			shorterType: "1m",
			// change to 9 from 10 to allow edge case timing or short pauses
			minShorterRecords:  9,
			longerType:         "10m",
			longerTimeDuration: -10 * time.Minute,
		},
		{
			shorterType:        "10m",
			minShorterRecords:  2,
			longerType:         "20m",
			longerTimeDuration: -20 * time.Minute,
		},
		{
			shorterType:        "20m",
			minShorterRecords:  6,
			longerType:         "120m",
			longerTimeDuration: -120 * time.Minute,
		},
		{
			shorterType:        "120m",
			minShorterRecords:  4,
			longerType:         "480m",
			longerTimeDuration: -480 * time.Minute,
		},
	}
	// wrap the operations in a transaction
	rm.app.RunInTransaction(func(txApp core.App) error {
		var err error
		collections := [3]*core.Collection{}
		collections[0], err = txApp.FindCachedCollectionByNameOrId("system_stats")
		if err != nil {
			return err
		}
		collections[1], err = txApp.FindCachedCollectionByNameOrId("container_stats")
		if err != nil {
			return err
		}
		collections[2], err = txApp.FindCachedCollectionByNameOrId("pod_stats")
		if err != nil {
			return err
		}
		var systems RecordIds
		db := txApp.DB()

		db.NewQuery("SELECT id FROM systems WHERE status='up'").All(&systems)

		// loop through all active systems, time periods, and collections
		for _, system := range systems {
			// log.Println("processing system", system.GetString("name"))
			for i := range longerRecordData {
				recordData := longerRecordData[i]
				// log.Println("processing longer record type", recordData.longerType)
				// add one minute padding for longer records because they are created slightly later than the job start time
				longerRecordPeriod := time.Now().UTC().Add(recordData.longerTimeDuration + time.Minute)
				// shorter records are created independently of longer records, so we shouldn't need to add padding
				shorterRecordPeriod := time.Now().UTC().Add(recordData.longerTimeDuration)
				// loop through both collections
				for _, collection := range collections {
					// check creation time of last longer record if not 10m, since 10m is created every run
					if recordData.longerType != "10m" {
						count, err := txApp.CountRecords(
							collection.Id,
							dbx.NewExp(
								"system = {:system} AND type = {:type} AND created > {:created}",
								dbx.Params{"type": recordData.longerType, "system": system.Id, "created": longerRecordPeriod},
							),
						)
						// continue if longer record exists
						if err != nil || count > 0 {
							continue
						}
					}
					// get shorter records from the past x minutes
					var recordIds RecordIds

					err := txApp.DB().
						Select("id").
						From(collection.Name).
						AndWhere(dbx.NewExp(
							"system={:system} AND type={:type} AND created > {:created}",
							dbx.Params{
								"type":    recordData.shorterType,
								"system":  system.Id,
								"created": shorterRecordPeriod,
							},
						)).
						All(&recordIds)

					// continue if not enough shorter records
					if err != nil || len(recordIds) < recordData.minShorterRecords {
						continue
					}
					// average the shorter records and create longer record
					longerRecord := core.NewRecord(collection)
					longerRecord.Set("system", system.Id)
					longerRecord.Set("type", recordData.longerType)
					switch collection.Name {
					case "system_stats":
						longerRecord.Set("stats", rm.AverageSystemStats(db, recordIds))
					case "container_stats":
						longerRecord.Set("stats", rm.AverageContainerStats(db, recordIds))
					case "pod_stats":
						longerRecord.Set("stats", rm.AveragePodStats(db, recordIds))
					}
					if err := txApp.SaveNoValidate(longerRecord); err != nil {
						log.Println("failed to save longer record", "err", err)
					}
				}
			}
		}

		return nil
	})

	statsRecord.Stats = statsRecord.Stats[:0]

	// log.Println("finished creating longer records", "time (ms)", time.Since(start).Milliseconds())
}

// Calculate the average stats of a list of system_stats records without reflect
func (rm *RecordManager) AverageSystemStats(db dbx.Builder, records RecordIds) *system.Stats {
	// Clear/reset global structs for reuse
	sumStats = system.Stats{}
	tempStats = system.Stats{}
	sum := &sumStats
	stats := &tempStats
	// necessary because uint8 is not big enough for the sum
	batterySum := 0
	// accumulate per-core usage across records
	var cpuCoresSums []uint64
	// accumulate cpu breakdown [user, system, iowait, steal, idle]
	var cpuBreakdownSums []float64

	count := float64(len(records))
	tempCount := float64(0)

	// Accumulate totals
	for _, record := range records {
		id := record.Id
		// clear global statsRecord for reuse
		statsRecord.Stats = statsRecord.Stats[:0]

		queryParams["id"] = id
		db.NewQuery("SELECT stats FROM system_stats WHERE id = {:id}").Bind(queryParams).One(&statsRecord)
		if err := json.Unmarshal(statsRecord.Stats, stats); err != nil {
			continue
		}

		sum.Cpu += stats.Cpu
		// accumulate cpu time breakdowns if present
		if stats.CpuBreakdown != nil {
			if len(cpuBreakdownSums) < len(stats.CpuBreakdown) {
				cpuBreakdownSums = append(cpuBreakdownSums, make([]float64, len(stats.CpuBreakdown)-len(cpuBreakdownSums))...)
			}
			for i, v := range stats.CpuBreakdown {
				cpuBreakdownSums[i] += v
			}
		}
		sum.Mem += stats.Mem
		sum.MemUsed += stats.MemUsed
		sum.MemPct += stats.MemPct
		sum.MemBuffCache += stats.MemBuffCache
		sum.MemZfsArc += stats.MemZfsArc
		sum.Swap += stats.Swap
		sum.SwapUsed += stats.SwapUsed
		sum.DiskTotal += stats.DiskTotal
		sum.DiskUsed += stats.DiskUsed
		sum.DiskPct += stats.DiskPct
		sum.DiskReadPs += stats.DiskReadPs
		sum.DiskWritePs += stats.DiskWritePs
		sum.NetworkSent += stats.NetworkSent
		sum.NetworkRecv += stats.NetworkRecv
		sum.LoadAvg[0] += stats.LoadAvg[0]
		sum.LoadAvg[1] += stats.LoadAvg[1]
		sum.LoadAvg[2] += stats.LoadAvg[2]
		sum.Bandwidth[0] += stats.Bandwidth[0]
		sum.Bandwidth[1] += stats.Bandwidth[1]
		sum.DiskIO[0] += stats.DiskIO[0]
		sum.DiskIO[1] += stats.DiskIO[1]
		batterySum += int(stats.Battery[0])
		sum.Battery[1] = stats.Battery[1]

		// accumulate per-core usage if present
		if stats.CpuCoresUsage != nil {
			if len(cpuCoresSums) < len(stats.CpuCoresUsage) {
				// extend slices to accommodate core count
				cpuCoresSums = append(cpuCoresSums, make([]uint64, len(stats.CpuCoresUsage)-len(cpuCoresSums))...)
			}
			for i, v := range stats.CpuCoresUsage {
				cpuCoresSums[i] += uint64(v)
			}
		}
		// Set peak values
		sum.MaxCpu = max(sum.MaxCpu, stats.MaxCpu, stats.Cpu)
		sum.MaxMem = max(sum.MaxMem, stats.MaxMem, stats.MemUsed)
		sum.MaxNetworkSent = max(sum.MaxNetworkSent, stats.MaxNetworkSent, stats.NetworkSent)
		sum.MaxNetworkRecv = max(sum.MaxNetworkRecv, stats.MaxNetworkRecv, stats.NetworkRecv)
		sum.MaxDiskReadPs = max(sum.MaxDiskReadPs, stats.MaxDiskReadPs, stats.DiskReadPs)
		sum.MaxDiskWritePs = max(sum.MaxDiskWritePs, stats.MaxDiskWritePs, stats.DiskWritePs)
		sum.MaxBandwidth[0] = max(sum.MaxBandwidth[0], stats.MaxBandwidth[0], stats.Bandwidth[0])
		sum.MaxBandwidth[1] = max(sum.MaxBandwidth[1], stats.MaxBandwidth[1], stats.Bandwidth[1])
		sum.MaxDiskIO[0] = max(sum.MaxDiskIO[0], stats.MaxDiskIO[0], stats.DiskIO[0])
		sum.MaxDiskIO[1] = max(sum.MaxDiskIO[1], stats.MaxDiskIO[1], stats.DiskIO[1])

		// Accumulate network interfaces
		if sum.NetworkInterfaces == nil {
			sum.NetworkInterfaces = make(map[string][4]uint64, len(stats.NetworkInterfaces))
		}
		for key, value := range stats.NetworkInterfaces {
			sum.NetworkInterfaces[key] = [4]uint64{
				sum.NetworkInterfaces[key][0] + value[0],
				sum.NetworkInterfaces[key][1] + value[1],
				max(sum.NetworkInterfaces[key][2], value[2]),
				max(sum.NetworkInterfaces[key][3], value[3]),
			}
		}

		// Accumulate temperatures
		if stats.Temperatures != nil {
			if sum.Temperatures == nil {
				sum.Temperatures = make(map[string]float64, len(stats.Temperatures))
			}
			tempCount++
			for key, value := range stats.Temperatures {
				sum.Temperatures[key] += value
			}
		}

		// Accumulate extra filesystem stats
		if stats.ExtraFs != nil {
			if sum.ExtraFs == nil {
				sum.ExtraFs = make(map[string]*system.FsStats, len(stats.ExtraFs))
			}
			for key, value := range stats.ExtraFs {
				if _, ok := sum.ExtraFs[key]; !ok {
					sum.ExtraFs[key] = &system.FsStats{}
				}
				fs := sum.ExtraFs[key]
				fs.DiskTotal += value.DiskTotal
				fs.DiskUsed += value.DiskUsed
				fs.DiskWritePs += value.DiskWritePs
				fs.DiskReadPs += value.DiskReadPs
				fs.MaxDiskReadPS = max(fs.MaxDiskReadPS, value.MaxDiskReadPS, value.DiskReadPs)
				fs.MaxDiskWritePS = max(fs.MaxDiskWritePS, value.MaxDiskWritePS, value.DiskWritePs)
				fs.DiskReadBytes += value.DiskReadBytes
				fs.DiskWriteBytes += value.DiskWriteBytes
				fs.MaxDiskReadBytes = max(fs.MaxDiskReadBytes, value.MaxDiskReadBytes, value.DiskReadBytes)
				fs.MaxDiskWriteBytes = max(fs.MaxDiskWriteBytes, value.MaxDiskWriteBytes, value.DiskWriteBytes)
			}
		}

		// Accumulate GPU data
		if stats.GPUData != nil {
			if sum.GPUData == nil {
				sum.GPUData = make(map[string]system.GPUData, len(stats.GPUData))
			}
			for id, value := range stats.GPUData {
				gpu, ok := sum.GPUData[id]
				if !ok {
					gpu = system.GPUData{Name: value.Name}
				}
				gpu.Temperature += value.Temperature
				gpu.MemoryUsed += value.MemoryUsed
				gpu.MemoryTotal += value.MemoryTotal
				gpu.Usage += value.Usage
				gpu.Power += value.Power
				gpu.Count += value.Count

				if value.Engines != nil {
					if gpu.Engines == nil {
						gpu.Engines = make(map[string]float64, len(value.Engines))
					}
					for engineKey, engineValue := range value.Engines {
						gpu.Engines[engineKey] += engineValue
					}
				}

				sum.GPUData[id] = gpu
			}
		}
	}

	// Compute averages in place
	if count > 0 {
		sum.Cpu = twoDecimals(sum.Cpu / count)
		sum.Mem = twoDecimals(sum.Mem / count)
		sum.MemUsed = twoDecimals(sum.MemUsed / count)
		sum.MemPct = twoDecimals(sum.MemPct / count)
		sum.MemBuffCache = twoDecimals(sum.MemBuffCache / count)
		sum.MemZfsArc = twoDecimals(sum.MemZfsArc / count)
		sum.Swap = twoDecimals(sum.Swap / count)
		sum.SwapUsed = twoDecimals(sum.SwapUsed / count)
		sum.DiskTotal = twoDecimals(sum.DiskTotal / count)
		sum.DiskUsed = twoDecimals(sum.DiskUsed / count)
		sum.DiskPct = twoDecimals(sum.DiskPct / count)
		sum.DiskReadPs = twoDecimals(sum.DiskReadPs / count)
		sum.DiskWritePs = twoDecimals(sum.DiskWritePs / count)
		sum.DiskIO[0] = sum.DiskIO[0] / uint64(count)
		sum.DiskIO[1] = sum.DiskIO[1] / uint64(count)
		sum.NetworkSent = twoDecimals(sum.NetworkSent / count)
		sum.NetworkRecv = twoDecimals(sum.NetworkRecv / count)
		sum.LoadAvg[0] = twoDecimals(sum.LoadAvg[0] / count)
		sum.LoadAvg[1] = twoDecimals(sum.LoadAvg[1] / count)
		sum.LoadAvg[2] = twoDecimals(sum.LoadAvg[2] / count)
		sum.Bandwidth[0] = sum.Bandwidth[0] / uint64(count)
		sum.Bandwidth[1] = sum.Bandwidth[1] / uint64(count)
		sum.Battery[0] = uint8(batterySum / int(count))

		// Average network interfaces
		if sum.NetworkInterfaces != nil {
			for key := range sum.NetworkInterfaces {
				sum.NetworkInterfaces[key] = [4]uint64{
					sum.NetworkInterfaces[key][0] / uint64(count),
					sum.NetworkInterfaces[key][1] / uint64(count),
					sum.NetworkInterfaces[key][2],
					sum.NetworkInterfaces[key][3],
				}
			}
		}

		// Average temperatures
		if sum.Temperatures != nil && tempCount > 0 {
			for key := range sum.Temperatures {
				sum.Temperatures[key] = twoDecimals(sum.Temperatures[key] / tempCount)
			}
		}

		// Average extra filesystem stats
		if sum.ExtraFs != nil {
			for key := range sum.ExtraFs {
				fs := sum.ExtraFs[key]
				fs.DiskTotal = twoDecimals(fs.DiskTotal / count)
				fs.DiskUsed = twoDecimals(fs.DiskUsed / count)
				fs.DiskWritePs = twoDecimals(fs.DiskWritePs / count)
				fs.DiskReadPs = twoDecimals(fs.DiskReadPs / count)
				fs.DiskReadBytes = fs.DiskReadBytes / uint64(count)
				fs.DiskWriteBytes = fs.DiskWriteBytes / uint64(count)
			}
		}

		// Average GPU data
		if sum.GPUData != nil {
			for id := range sum.GPUData {
				gpu := sum.GPUData[id]
				gpu.Temperature = twoDecimals(gpu.Temperature / count)
				gpu.MemoryUsed = twoDecimals(gpu.MemoryUsed / count)
				gpu.MemoryTotal = twoDecimals(gpu.MemoryTotal / count)
				gpu.Usage = twoDecimals(gpu.Usage / count)
				gpu.Power = twoDecimals(gpu.Power / count)
				gpu.Count = twoDecimals(gpu.Count / count)

				if gpu.Engines != nil {
					for engineKey := range gpu.Engines {
						gpu.Engines[engineKey] = twoDecimals(gpu.Engines[engineKey] / count)
					}
				}

				sum.GPUData[id] = gpu
			}
		}

		// Average per-core usage
		if len(cpuCoresSums) > 0 {
			avg := make(system.Uint8Slice, len(cpuCoresSums))
			for i := range cpuCoresSums {
				v := math.Round(float64(cpuCoresSums[i]) / count)
				avg[i] = uint8(v)
			}
			sum.CpuCoresUsage = avg
		}

		// Average CPU breakdown
		if len(cpuBreakdownSums) > 0 {
			avg := make([]float64, len(cpuBreakdownSums))
			for i := range cpuBreakdownSums {
				avg[i] = twoDecimals(cpuBreakdownSums[i] / count)
			}
			sum.CpuBreakdown = avg
		}
	}

	return sum
}

// Calculate the average stats of a list of container_stats records
func (rm *RecordManager) AverageContainerStats(db dbx.Builder, records RecordIds) []container.Stats {
	// Clear global map for reuse
	for k := range containerSums {
		delete(containerSums, k)
	}
	sums := containerSums
	count := float64(len(records))

	for i := range records {
		id := records[i].Id
		// clear global statsRecord and containerStats for reuse
		statsRecord.Stats = statsRecord.Stats[:0]
		containerStats = containerStats[:0]

		queryParams["id"] = id
		db.NewQuery("SELECT stats FROM container_stats WHERE id = {:id}").Bind(queryParams).One(&statsRecord)

		if err := json.Unmarshal(statsRecord.Stats, &containerStats); err != nil {
			return []container.Stats{}
		}
		for i := range containerStats {
			stat := containerStats[i]
			if _, ok := sums[stat.Name]; !ok {
				sums[stat.Name] = &container.Stats{Name: stat.Name}
			}
			sums[stat.Name].Cpu += stat.Cpu
			sums[stat.Name].Mem += stat.Mem
			sums[stat.Name].NetworkSent += stat.NetworkSent
			sums[stat.Name].NetworkRecv += stat.NetworkRecv
		}
	}

	result := make([]container.Stats, 0, len(sums))
	for _, value := range sums {
		result = append(result, container.Stats{
			Name:        value.Name,
			Cpu:         twoDecimals(value.Cpu / count),
			Mem:         twoDecimals(value.Mem / count),
			NetworkSent: twoDecimals(value.NetworkSent / count),
			NetworkRecv: twoDecimals(value.NetworkRecv / count),
		})
	}
	return result
}

// Calculate the average stats of a list of pod_stats records
func (rm *RecordManager) AveragePodStats(db dbx.Builder, records RecordIds) []kubernetes.PodStats {
	// Clear global map for reuse
	for k := range podSums {
		delete(podSums, k)
	}
	sums := podSums
	count := float64(len(records))

	for i := range records {
		id := records[i].Id
		// clear global statsRecord and podStats for reuse
		statsRecord.Stats = statsRecord.Stats[:0]
		podStats = podStats[:0]

		queryParams["id"] = id
		db.NewQuery("SELECT stats FROM pod_stats WHERE id = {:id}").Bind(queryParams).One(&statsRecord)

		if err := json.Unmarshal(statsRecord.Stats, &podStats); err != nil {
			return []kubernetes.PodStats{}
		}
		for i := range podStats {
			stat := podStats[i]
			// Use namespace/name as unique key
			podKey := stat.Namespace + "/" + stat.Name
			if _, ok := sums[podKey]; !ok {
				sums[podKey] = &kubernetes.PodStats{
					Name:           stat.Name,
					Namespace:      stat.Namespace,
					Node:           stat.Node,
					Status:         stat.Status,
					RestartCount:   stat.RestartCount,
					ContainerCount: stat.ContainerCount,
				}
			}
			sums[podKey].Cpu += stat.Cpu
			sums[podKey].Mem += stat.Mem
			sums[podKey].MemLimit += stat.MemLimit
			sums[podKey].NetworkSent += stat.NetworkSent
			sums[podKey].NetworkRecv += stat.NetworkRecv
		}
	}

	result := make([]kubernetes.PodStats, 0, len(sums))
	for _, value := range sums {
		result = append(result, kubernetes.PodStats{
			Name:           value.Name,
			Namespace:      value.Namespace,
			Node:           value.Node,
			Cpu:            twoDecimals(value.Cpu / count),
			Mem:            twoDecimals(value.Mem / count),
			MemLimit:       twoDecimals(value.MemLimit / count),
			NetworkSent:    twoDecimals(value.NetworkSent / count),
			NetworkRecv:    twoDecimals(value.NetworkRecv / count),
			Status:         value.Status,
			RestartCount:   value.RestartCount,
			ContainerCount: value.ContainerCount,
		})
	}
	return result
}

// Delete old records
func (rm *RecordManager) DeleteOldRecords() {
	rm.app.RunInTransaction(func(txApp core.App) error {
		err := deleteOldSystemStats(txApp)
		if err != nil {
			return err
		}
		err = deleteOldContainerRecords(txApp)
		if err != nil {
			return err
		}
		err = deleteOldSystemdServiceRecords(txApp)
		if err != nil {
			return err
		}
		err = deleteOldAlertsHistory(txApp, 200, 250)
		if err != nil {
			return err
		}
		err = deleteOldQuietHours(txApp)
		if err != nil {
			return err
		}
		return nil
	})
}

// Delete old alerts history records
func deleteOldAlertsHistory(app core.App, countToKeep, countBeforeDeletion int) error {
	db := app.DB()
	var users []struct {
		Id string `db:"user"`
	}
	err := db.NewQuery("SELECT user, COUNT(*) as count FROM alerts_history GROUP BY user HAVING count > {:countBeforeDeletion}").Bind(dbx.Params{"countBeforeDeletion": countBeforeDeletion}).All(&users)
	if err != nil {
		return err
	}
	for _, user := range users {
		_, err = db.NewQuery("DELETE FROM alerts_history WHERE user = {:user} AND id NOT IN (SELECT id FROM alerts_history WHERE user = {:user} ORDER BY created DESC LIMIT {:countToKeep})").Bind(dbx.Params{"user": user.Id, "countToKeep": countToKeep}).Execute()
		if err != nil {
			return err
		}
	}
	return nil
}

// Deletes system_stats records older than what is displayed in the UI
func deleteOldSystemStats(app core.App) error {
	// Collections to process
	collections := [3]string{"system_stats", "container_stats", "pod_stats"}

	// Record types and their retention periods
	type RecordDeletionData struct {
		recordType string
		retention  time.Duration
	}
	recordData := []RecordDeletionData{
		{recordType: "1m", retention: time.Hour},             // 1 hour
		{recordType: "10m", retention: 12 * time.Hour},       // 12 hours
		{recordType: "20m", retention: 24 * time.Hour},       // 1 day
		{recordType: "120m", retention: 7 * 24 * time.Hour},  // 7 days
		{recordType: "480m", retention: 30 * 24 * time.Hour}, // 30 days
	}

	now := time.Now().UTC()

	for _, collection := range collections {
		// Build the WHERE clause
		var conditionParts []string
		var params dbx.Params = make(map[string]any)
		for i := range recordData {
			rd := recordData[i]
			// Create parameterized condition for this record type
			dateParam := fmt.Sprintf("date%d", i)
			conditionParts = append(conditionParts, fmt.Sprintf("(type = '%s' AND created < {:%s})", rd.recordType, dateParam))
			params[dateParam] = now.Add(-rd.retention)
		}
		// Combine conditions with OR
		conditionStr := strings.Join(conditionParts, " OR ")
		// Construct and execute the full raw query
		rawQuery := fmt.Sprintf("DELETE FROM %s WHERE %s", collection, conditionStr)
		if _, err := app.DB().NewQuery(rawQuery).Bind(params).Execute(); err != nil {
			return fmt.Errorf("failed to delete from %s: %v", collection, err)
		}
	}
	return nil
}

// Deletes systemd service records that haven't been updated in the last 20 minutes
func deleteOldSystemdServiceRecords(app core.App) error {
	now := time.Now().UTC()
	twentyMinutesAgo := now.Add(-20 * time.Minute)

	// Delete systemd service records where updated < twentyMinutesAgo
	_, err := app.DB().NewQuery("DELETE FROM systemd_services WHERE updated < {:updated}").Bind(dbx.Params{"updated": twentyMinutesAgo.UnixMilli()}).Execute()
	if err != nil {
		return fmt.Errorf("failed to delete old systemd service records: %v", err)
	}

	return nil
}

// Deletes container records that haven't been updated in the last 10 minutes
func deleteOldContainerRecords(app core.App) error {
	now := time.Now().UTC()
	tenMinutesAgo := now.Add(-10 * time.Minute)

	// Delete container records where updated < tenMinutesAgo
	_, err := app.DB().NewQuery("DELETE FROM containers WHERE updated < {:updated}").Bind(dbx.Params{"updated": tenMinutesAgo.UnixMilli()}).Execute()
	if err != nil {
		return fmt.Errorf("failed to delete old container records: %v", err)
	}

	// Also delete pod records that haven't been updated in the last 10 minutes
	_, err = app.DB().NewQuery("DELETE FROM pods WHERE updated < {:updated}").Bind(dbx.Params{"updated": tenMinutesAgo.UnixMilli()}).Execute()
	if err != nil {
		return fmt.Errorf("failed to delete old pod records: %v", err)
	}

	return nil
}

// Deletes old quiet hours records where end date has passed
func deleteOldQuietHours(app core.App) error {
	now := time.Now().UTC()
	_, err := app.DB().NewQuery("DELETE FROM quiet_hours WHERE type = 'one-time' AND end < {:now}").Bind(dbx.Params{"now": now}).Execute()
	if err != nil {
		return err
	}

	return nil
}

/* Round float to two decimals */
func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
