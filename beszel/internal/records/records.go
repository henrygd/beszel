// Package records handles creating longer records and deleting old records.
package records

import (
	"beszel/internal/entities/container"
	"beszel/internal/entities/system"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

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
var statsRecord StatsRecord
var containerStats []container.Stats
var sumStats system.Stats
var tempStats system.Stats
var queryParams = make(dbx.Params, 1)
var containerSums = make(map[string]*container.Stats)

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
		collections := [2]*core.Collection{}
		collections[0], err = txApp.FindCachedCollectionByNameOrId("system_stats")
		if err != nil {
			return err
		}
		collections[1], err = txApp.FindCachedCollectionByNameOrId("container_stats")
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
		sum.LoadAvg1 += stats.LoadAvg1
		sum.LoadAvg5 += stats.LoadAvg5
		sum.LoadAvg15 += stats.LoadAvg15
		// Set peak values
		sum.MaxCpu = max(sum.MaxCpu, stats.MaxCpu, stats.Cpu)
		sum.MaxNetworkSent = max(sum.MaxNetworkSent, stats.MaxNetworkSent, stats.NetworkSent)
		sum.MaxNetworkRecv = max(sum.MaxNetworkRecv, stats.MaxNetworkRecv, stats.NetworkRecv)
		sum.MaxDiskReadPs = max(sum.MaxDiskReadPs, stats.MaxDiskReadPs, stats.DiskReadPs)
		sum.MaxDiskWritePs = max(sum.MaxDiskWritePs, stats.MaxDiskWritePs, stats.DiskWritePs)

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
		sum.NetworkSent = twoDecimals(sum.NetworkSent / count)
		sum.NetworkRecv = twoDecimals(sum.NetworkRecv / count)
		sum.LoadAvg1 = twoDecimals(sum.LoadAvg1 / count)
		sum.LoadAvg5 = twoDecimals(sum.LoadAvg5 / count)
		sum.LoadAvg15 = twoDecimals(sum.LoadAvg15 / count)
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
				sum.GPUData[id] = gpu
			}
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

// Delete old records
func (rm *RecordManager) DeleteOldRecords() {
	rm.app.RunInTransaction(func(txApp core.App) error {
		err := deleteOldSystemStats(txApp)
		if err != nil {
			return err
		}
		err = deleteOldAlertsHistory(txApp, 200, 250)
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
	collections := [2]string{"system_stats", "container_stats"}

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

/* Round float to two decimals */
func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
