// Package records handles creating longer records and deleting old records.
package records

import (
	"beszel/internal/entities/container"
	"beszel/internal/entities/system"
	"log"
	"math"
	"time"

	"github.com/goccy/go-json"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

type RecordManager struct {
	app *pocketbase.PocketBase
}

type LongerRecordData struct {
	shorterType        string
	longerType         string
	longerTimeDuration time.Duration
	minShorterRecords  int
}

type RecordDeletionData struct {
	recordType string
	retention  time.Duration
}

type RecordStats []struct {
	Stats []byte `db:"stats"`
}

func NewRecordManager(app *pocketbase.PocketBase) *RecordManager {
	return &RecordManager{app}
}

// Create longer records by averaging shorter records
func (rm *RecordManager) CreateLongerRecords(collections []*core.Collection) {
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
		activeSystems, err := txApp.FindAllRecords("systems", dbx.NewExp("status = 'up'"))
		if err != nil {
			log.Println("failed to get active systems", "err", err.Error())
			return err
		}

		// loop through all active systems, time periods, and collections
		for _, system := range activeSystems {
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
						lastLongerRecord, err := txApp.FindFirstRecordByFilter(
							collection.Id,
							"type = {:type} && system = {:system} && created > {:created}",
							dbx.Params{"type": recordData.longerType, "system": system.Id, "created": longerRecordPeriod},
						)
						// continue if longer record exists
						if err == nil || lastLongerRecord != nil {
							// log.Println("longer record found. continuing")
							continue
						}
					}
					// get shorter records from the past x minutes
					var stats RecordStats

					err := txApp.DB().
						Select("stats").
						From(collection.Name).
						AndWhere(dbx.NewExp(
							"type={:type} AND system={:system} AND created > {:created}",
							dbx.Params{
								"type":    recordData.shorterType,
								"system":  system.Id,
								"created": shorterRecordPeriod,
							},
						)).
						All(&stats)

					// continue if not enough shorter records
					if err != nil || len(stats) < recordData.minShorterRecords {
						// log.Println("not enough shorter records. continue.", len(allShorterRecords), recordData.expectedShorterRecords)
						continue
					}
					// average the shorter records and create longer record
					longerRecord := core.NewRecord(collection)
					longerRecord.Set("system", system.Id)
					longerRecord.Set("type", recordData.longerType)
					switch collection.Name {
					case "system_stats":
						longerRecord.Set("stats", rm.AverageSystemStats(stats))
					case "container_stats":
						longerRecord.Set("stats", rm.AverageContainerStats(stats))
					}
					if err := txApp.SaveNoValidate(longerRecord); err != nil {
						log.Println("failed to save longer record", "err", err.Error())
					}
				}
			}
		}

		return nil
	})

	// log.Println("finished creating longer records", "time (ms)", time.Since(start).Milliseconds())
}

// Calculate the average stats of a list of system_stats records without reflect
func (rm *RecordManager) AverageSystemStats(records RecordStats) system.Stats {
	sum := system.Stats{}
	count := float64(len(records))
	// use different counter for temps in case some records don't have them
	tempCount := float64(0)

	var stats system.Stats
	for i := range records {
		json.Unmarshal(records[i].Stats, &stats)
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
		// set peak values
		sum.MaxCpu = max(sum.MaxCpu, stats.MaxCpu, stats.Cpu)
		sum.MaxNetworkSent = max(sum.MaxNetworkSent, stats.MaxNetworkSent, stats.NetworkSent)
		sum.MaxNetworkRecv = max(sum.MaxNetworkRecv, stats.MaxNetworkRecv, stats.NetworkRecv)
		sum.MaxDiskReadPs = max(sum.MaxDiskReadPs, stats.MaxDiskReadPs, stats.DiskReadPs)
		sum.MaxDiskWritePs = max(sum.MaxDiskWritePs, stats.MaxDiskWritePs, stats.DiskWritePs)
		// add temps to sum
		if stats.Temperatures != nil {
			if sum.Temperatures == nil {
				sum.Temperatures = make(map[string]float64, len(stats.Temperatures))
			}
			tempCount++
			for key, value := range stats.Temperatures {
				if _, ok := sum.Temperatures[key]; !ok {
					sum.Temperatures[key] = 0
				}
				sum.Temperatures[key] += value
			}
		}
		// add extra fs to sum
		if stats.ExtraFs != nil {
			if sum.ExtraFs == nil {
				sum.ExtraFs = make(map[string]*system.FsStats, len(stats.ExtraFs))
			}
			for key, value := range stats.ExtraFs {
				if _, ok := sum.ExtraFs[key]; !ok {
					sum.ExtraFs[key] = &system.FsStats{}
				}
				sum.ExtraFs[key].DiskTotal += value.DiskTotal
				sum.ExtraFs[key].DiskUsed += value.DiskUsed
				sum.ExtraFs[key].DiskWritePs += value.DiskWritePs
				sum.ExtraFs[key].DiskReadPs += value.DiskReadPs
				// peak values
				sum.ExtraFs[key].MaxDiskReadPS = max(sum.ExtraFs[key].MaxDiskReadPS, value.MaxDiskReadPS, value.DiskReadPs)
				sum.ExtraFs[key].MaxDiskWritePS = max(sum.ExtraFs[key].MaxDiskWritePS, value.MaxDiskWritePS, value.DiskWritePs)
			}
		}
		// add GPU data
		if stats.GPUData != nil {
			if sum.GPUData == nil {
				sum.GPUData = make(map[string]system.GPUData, len(stats.GPUData))
			}
			for id, value := range stats.GPUData {
				if _, ok := sum.GPUData[id]; !ok {
					sum.GPUData[id] = system.GPUData{Name: value.Name}
				}
				gpu := sum.GPUData[id]
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

	stats = system.Stats{
		Cpu:            twoDecimals(sum.Cpu / count),
		Mem:            twoDecimals(sum.Mem / count),
		MemUsed:        twoDecimals(sum.MemUsed / count),
		MemPct:         twoDecimals(sum.MemPct / count),
		MemBuffCache:   twoDecimals(sum.MemBuffCache / count),
		MemZfsArc:      twoDecimals(sum.MemZfsArc / count),
		Swap:           twoDecimals(sum.Swap / count),
		SwapUsed:       twoDecimals(sum.SwapUsed / count),
		DiskTotal:      twoDecimals(sum.DiskTotal / count),
		DiskUsed:       twoDecimals(sum.DiskUsed / count),
		DiskPct:        twoDecimals(sum.DiskPct / count),
		DiskReadPs:     twoDecimals(sum.DiskReadPs / count),
		DiskWritePs:    twoDecimals(sum.DiskWritePs / count),
		NetworkSent:    twoDecimals(sum.NetworkSent / count),
		NetworkRecv:    twoDecimals(sum.NetworkRecv / count),
		MaxCpu:         sum.MaxCpu,
		MaxDiskReadPs:  sum.MaxDiskReadPs,
		MaxDiskWritePs: sum.MaxDiskWritePs,
		MaxNetworkSent: sum.MaxNetworkSent,
		MaxNetworkRecv: sum.MaxNetworkRecv,
	}

	if sum.Temperatures != nil {
		stats.Temperatures = make(map[string]float64, len(sum.Temperatures))
		for key, value := range sum.Temperatures {
			stats.Temperatures[key] = twoDecimals(value / tempCount)
		}
	}

	if sum.ExtraFs != nil {
		stats.ExtraFs = make(map[string]*system.FsStats, len(sum.ExtraFs))
		for key, value := range sum.ExtraFs {
			stats.ExtraFs[key] = &system.FsStats{
				DiskTotal:      twoDecimals(value.DiskTotal / count),
				DiskUsed:       twoDecimals(value.DiskUsed / count),
				DiskWritePs:    twoDecimals(value.DiskWritePs / count),
				DiskReadPs:     twoDecimals(value.DiskReadPs / count),
				MaxDiskReadPS:  value.MaxDiskReadPS,
				MaxDiskWritePS: value.MaxDiskWritePS,
			}
		}
	}

	if sum.GPUData != nil {
		stats.GPUData = make(map[string]system.GPUData, len(sum.GPUData))
		for id, value := range sum.GPUData {
			stats.GPUData[id] = system.GPUData{
				Name:        value.Name,
				Temperature: twoDecimals(value.Temperature / count),
				MemoryUsed:  twoDecimals(value.MemoryUsed / count),
				MemoryTotal: twoDecimals(value.MemoryTotal / count),
				Usage:       twoDecimals(value.Usage / count),
				Power:       twoDecimals(value.Power / count),
				Count:       twoDecimals(value.Count / count),
			}
		}
	}

	return stats
}

// Calculate the average stats of a list of container_stats records
func (rm *RecordManager) AverageContainerStats(records RecordStats) []container.Stats {
	sums := make(map[string]*container.Stats)
	count := float64(len(records))

	var containerStats []container.Stats
	for i := range records {
		// Reset the slice length to 0, but keep the capacity
		containerStats = containerStats[:0]
		if err := json.Unmarshal(records[i].Stats, &containerStats); err != nil {
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

// Deletes records older than what is displayed in the UI
func (rm *RecordManager) DeleteOldRecords() {
	collections := []string{"system_stats", "container_stats"}
	recordData := []RecordDeletionData{
		{
			recordType: "1m",
			retention:  time.Hour,
		},
		{
			recordType: "10m",
			retention:  12 * time.Hour,
		},
		{
			recordType: "20m",
			retention:  24 * time.Hour,
		},
		{
			recordType: "120m",
			retention:  7 * 24 * time.Hour,
		},
		{
			recordType: "480m",
			retention:  30 * 24 * time.Hour,
		},
	}
	db := rm.app.NonconcurrentDB()
	for _, recordData := range recordData {
		for _, collectionSlug := range collections {
			formattedDate := time.Now().UTC().Add(-recordData.retention).Format(types.DefaultDateLayout)
			expr := dbx.NewExp("[[created]] < {:date} AND [[type]] = {:type}", dbx.Params{"date": formattedDate, "type": recordData.recordType})
			_, err := db.Delete(collectionSlug, expr).Execute()
			if err != nil {
				rm.app.Logger().Error("Failed to delete records", "err", err.Error())
			}
		}
	}
}

/* Round float to two decimals */
func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
