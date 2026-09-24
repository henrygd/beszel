// Package records handles creating longer records and deleting old records.
package records

import (
	"encoding/json"
	"math"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/system"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
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

// Create longer records by averaging shorter records
func (rm *RecordManager) CreateLongerRecords() {
	now := time.Now().UTC()
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
	// Pocketbase cron does not handle errors, log them here.
	err := rm.app.RunInTransaction(func(txApp core.App) error {
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
		nutStatsColl, err := txApp.FindCachedCollectionByNameOrId("nut_stats")
		if err != nil {
			return err
		}
		monitorStatsColl, err := txApp.FindCachedCollectionByNameOrId("network_monitor_stats")
		if err != nil {
			return err
		}
		var systems RecordIds
		db := txApp.DB()

		if err := db.NewQuery("SELECT id FROM systems WHERE status='up'").All(&systems); err != nil {
			return err
		}

		// loop through all active systems, time periods, and collections
		for _, system := range systems {
			// log.Println("processing system", system.GetString("name"))
			for i := range longerRecordData {
				recordData := longerRecordData[i]
				// log.Println("processing longer record type", recordData.longerType)
				// add one minute padding for longer records because they are created slightly later than the job start time
				longerRecordPeriod := now.Add(recordData.longerTimeDuration + time.Minute)
				// shorter records are created independently of longer records, so we shouldn't need to add padding
				shorterRecordPeriod := now.Add(recordData.longerTimeDuration)
				for _, collection := range collections {
					// check creation time of last longer record if not 10m, since 10m is created every run
					if recordData.longerType != "10m" {
						count, err := txApp.CountRecords(collection.Id, dbx.NewExp(
							"system = {:system} AND type = {:type} AND created > {:created}",
							dbx.Params{
								"type":    recordData.longerType,
								"system":  system.Id,
								"created": longerRecordPeriod.Format(types.DefaultDateLayout),
							},
						))
						if err != nil {
							return err
						}
						// continue if longer record exists
						if count > 0 {
							continue
						}
					}
					// get shorter records from the past x minutes
					var recordIds RecordIds

					params := dbx.Params{
						"type":    recordData.shorterType,
						"system":  system.Id,
						"created": shorterRecordPeriod.Format(types.DefaultDateLayout),
					}

					err := db.
						Select("id").
						From(collection.Name).
						Where(dbx.NewExp(
							"system={:system} AND type={:type} AND created > {:created}",
							params,
						)).
						OrderBy("created").
						All(&recordIds)
					if err != nil {
						return err
					}

					// continue if not enough shorter records
					if len(recordIds) < recordData.minShorterRecords {
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
						txApp.Logger().Error("failed to save longer record", "err", err)
					}
				}
			}
			if err := rm.createLongerNutRecords(txApp, nutStatsColl, system.Id, longerRecordData, now); err != nil {
				return err
			}
		}

		// network_monitor_stats is aggregated per monitor (not per system)
		var monitors []struct {
			Id     string `db:"id"`
			System string `db:"system"`
		}
		// Disabled monitors still have history that must advance through retention tiers.
		if err := db.NewQuery("SELECT id, system FROM network_monitors").All(&monitors); err != nil {
			return err
		}

		for _, monitorRec := range monitors {
			for i := range longerRecordData {
				recordData := longerRecordData[i]
				longerRecordPeriod := now.Add(recordData.longerTimeDuration + time.Minute)
				shorterRecordPeriod := now.Add(recordData.longerTimeDuration)

				if recordData.longerType != "10m" {
					count, err := txApp.CountRecords(monitorStatsColl.Id, dbx.NewExp(
						"monitor={:monitor} AND type={:type} AND created>{:created}",
						dbx.Params{
							"monitor": monitorRec.Id,
							"type":    recordData.longerType,
							"created": longerRecordPeriod.UnixMilli(),
						},
					))
					if err != nil {
						return err
					}
					if count > 0 {
						continue
					}
				}

				stats, count, err := rm.AverageMonitorStats(db, monitorRec.Id, recordData.shorterType, shorterRecordPeriod.UnixMilli())
				if err != nil {
					txApp.Logger().Error("failed to average monitor stats", "monitor", monitorRec.Id, "err", err)
					continue
				}
				// Monitor intervals can exceed the aggregation window, so average
				// any available records at every level and skip only empty windows.
				if count == 0 {
					continue
				}

				longerRecord := core.NewRecord(monitorStatsColl)
				longerRecord.Set("system", monitorRec.System)
				longerRecord.Set("monitor", monitorRec.Id)
				longerRecord.Set("type", recordData.longerType)
				longerRecord.Set("created", now.UnixMilli())
				longerRecord.Set("res_min", stats.ResMin)
				longerRecord.Set("res_max", stats.ResMax)
				longerRecord.Set("total_count", stats.TotalCount)
				longerRecord.Set("success_count", stats.SuccessCount)
				longerRecord.Set("res_sum", stats.ResponseSum)
				if err := txApp.SaveNoValidate(longerRecord); err != nil {
					txApp.Logger().Error("failed to save monitor longer record", "err", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		rm.app.Logger().Error("failed to create longer records", "err", err)
	}
}

func (rm *RecordManager) createLongerNutRecords(app core.App, collection *core.Collection, systemID string, periods []LongerRecordData, now time.Time) error {
	var devices []struct {
		Id string `db:"device"`
	}
	if err := app.DB().NewQuery("SELECT DISTINCT device FROM nut_stats WHERE system = {:system}").Bind(dbx.Params{"system": systemID}).All(&devices); err != nil {
		return err
	}
	for _, device := range devices {
		for _, period := range periods {
			longerPeriod := now.Add(period.longerTimeDuration + time.Minute)
			if period.longerType != "10m" {
				count, err := app.CountRecords(collection.Id, dbx.NewExp("system={:system} AND device={:device} AND type={:type} AND created>{:created}", dbx.Params{"system": systemID, "device": device.Id, "type": period.longerType, "created": longerPeriod.Format(types.DefaultDateLayout)}))
				if err != nil {
					return err
				}
				if count > 0 {
					continue
				}
			}
			rows, err := app.FindRecordsByFilter(collection, "system={:system} && device={:device} && type={:type} && created>{:created}", "", 0, 0, dbx.Params{"system": systemID, "device": device.Id, "type": period.shorterType, "created": now.Add(period.longerTimeDuration).Format(types.DefaultDateLayout)})
			if err != nil {
				return err
			}
			if len(rows) < period.minShorterRecords {
				continue
			}
			record := core.NewRecord(collection)
			record.Set("system", systemID)
			record.Set("device", device.Id)
			record.Set("type", period.longerType)
			for _, field := range []string{"battery_charge", "battery_runtime", "battery_voltage", "input_voltage", "output_voltage", "load", "output_current", "output_power"} {
				total := 0.0
				for _, row := range rows {
					total += row.GetFloat(field)
				}
				record.Set(field, total/float64(len(rows)))
			}
			if err := app.SaveNoValidate(record); err != nil {
				return err
			}
		}
	}
	return nil
}

func getCreatedTimeField(collectionName string, period time.Time) any {
	// network_monitor_stats stores created as unix timestamp in ms, not as a date string
	if collectionName == "network_monitor_stats" {
		return period.UnixMilli()
	}
	return period.Format(types.DefaultDateLayout)
}

// Calculate the average stats of a list of system_stats records without reflect
func (rm *RecordManager) AverageSystemStats(db dbx.Builder, records RecordIds) *system.Stats {
	stats := make([]system.Stats, 0, len(records))
	var row StatsRecord
	params := make(dbx.Params, 1)
	for _, rec := range records {
		row.Stats = row.Stats[:0]
		params["id"] = rec.Id
		db.NewQuery("SELECT stats FROM system_stats WHERE id = {:id}").Bind(params).One(&row)
		var s system.Stats
		if err := json.Unmarshal(row.Stats, &s); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	result := AverageSystemStatsSlice(stats)
	return &result
}

// AverageSystemStatsSlice computes the average of a slice of system stats.
func AverageSystemStatsSlice(records []system.Stats) system.Stats {
	var sum system.Stats
	count := float64(len(records))
	if count == 0 {
		return sum
	}

	// necessary because uint8 is not big enough for the sum
	batterySum := 0
	batteryCount := 0
	batterySums := make(map[string]uint64)
	batteryCounts := make(map[string]uint64)
	// accumulate per-core usage across records
	var cpuCoresSums []uint64
	// accumulate cpu breakdown [user, system, iowait, steal, idle]
	var cpuBreakdownSums []float64
	tempCount := float64(0)
	var fanSums map[string]uint64
	fanCount := uint64(0)
	zfsPoolCounts := make(map[string]uint64)
	zfsCapacityCounts := make(map[string]uint64)

	// Accumulate totals
	for i := range records {
		stats := &records[i]

		sum.Cpu += stats.Cpu
		// accumulate cpu time breakdowns if present
		if stats.CpuBreakdown != nil {
			if len(cpuBreakdownSums) < len(stats.CpuBreakdown) {
				cpuBreakdownSums = append(cpuBreakdownSums, make([]float64, len(stats.CpuBreakdown)-len(cpuBreakdownSums))...)
			}
			for j, v := range stats.CpuBreakdown {
				cpuBreakdownSums[j] += v
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
		for i := range stats.DiskIoStats {
			sum.DiskIoStats[i] += stats.DiskIoStats[i]
		}
		if hasBattery(stats.Battery, stats.Batteries) {
			batterySum += int(stats.Battery[0])
			batteryCount++
			sum.Battery[1] = stats.Battery[1]
		}
		for name, percent := range stats.Batteries {
			batterySums[name] += uint64(percent)
			batteryCounts[name]++
		}

		// accumulate per-core usage if present
		if stats.CpuCoresUsage != nil {
			if len(cpuCoresSums) < len(stats.CpuCoresUsage) {
				// extend slices to accommodate core count
				cpuCoresSums = append(cpuCoresSums, make([]uint64, len(stats.CpuCoresUsage)-len(cpuCoresSums))...)
			}
			for j, v := range stats.CpuCoresUsage {
				cpuCoresSums[j] += uint64(v)
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
		sum.DiskIOTotal[0] = max(sum.DiskIOTotal[0], stats.DiskIOTotal[0])
		sum.DiskIOTotal[1] = max(sum.DiskIOTotal[1], stats.DiskIOTotal[1])
		for i := range stats.DiskIoStats {
			sum.MaxDiskIoStats[i] = max(sum.MaxDiskIoStats[i], stats.MaxDiskIoStats[i], stats.DiskIoStats[i])
		}

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

		// Accumulate fan speeds
		if stats.Fans != nil {
			if fanSums == nil {
				fanSums = make(map[string]uint64, len(stats.Fans))
			}
			fanCount++
			for key, value := range stats.Fans {
				fanSums[key] += uint64(value)
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
				fs.TotalRead = max(fs.TotalRead, value.TotalRead)
				fs.TotalWrite = max(fs.TotalWrite, value.TotalWrite)
				for i := range value.DiskIoStats {
					fs.DiskIoStats[i] += value.DiskIoStats[i]
					fs.MaxDiskIoStats[i] = max(fs.MaxDiskIoStats[i], value.MaxDiskIoStats[i], value.DiskIoStats[i])
				}
			}
		}

		// Accumulate ZFS pool stats. Counts are tracked per entry so a pool
		// missing from some samples is not averaged as zero.
		if stats.ZfsPools != nil {
			if sum.ZfsPools == nil {
				sum.ZfsPools = make(map[string]*system.ZfsPool, len(stats.ZfsPools))
			}
			for name, value := range stats.ZfsPools {
				if value == nil {
					continue
				}
				pool := sum.ZfsPools[name]
				if pool == nil {
					pool = &system.ZfsPool{HideUsage: value.HideUsage, HideIO: value.HideIO}
					sum.ZfsPools[name] = pool
				}
				// Never average physical and usable capacity into the same value.
				if pool.Raw != value.Raw {
					pool.Total, pool.Used = 0, 0
					zfsCapacityCounts[name] = 0
				}
				pool.HideUsage = pool.HideUsage && value.HideUsage
				pool.HideIO = pool.HideIO && value.HideIO
				pool.DisplayName = value.DisplayName
				pool.Raw = value.Raw
				zfsCapacityCounts[name]++
				pool.Total += value.Total
				pool.Used += value.Used
				pool.ReadBytes += value.ReadBytes
				pool.WriteBytes += value.WriteBytes
				if value.Health != "" {
					pool.Health = value.Health
				}
				zfsPoolCounts[name]++
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

	// Compute averages
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
	for i := range sum.DiskIoStats {
		sum.DiskIoStats[i] = twoDecimals(sum.DiskIoStats[i] / count)
	}
	sum.NetworkSent = twoDecimals(sum.NetworkSent / count)
	sum.NetworkRecv = twoDecimals(sum.NetworkRecv / count)
	sum.LoadAvg[0] = twoDecimals(sum.LoadAvg[0] / count)
	sum.LoadAvg[1] = twoDecimals(sum.LoadAvg[1] / count)
	sum.LoadAvg[2] = twoDecimals(sum.LoadAvg[2] / count)
	sum.Bandwidth[0] = sum.Bandwidth[0] / uint64(count)
	sum.Bandwidth[1] = sum.Bandwidth[1] / uint64(count)
	if batteryCount > 0 {
		sum.Battery[0] = uint8(batterySum / batteryCount)
	}
	if len(batterySums) > 0 {
		sum.Batteries = make(map[string]uint8, len(batterySums))
		for name, total := range batterySums {
			sum.Batteries[name] = uint8(total / batteryCounts[name])
		}
	}

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

	// Average fan speeds
	if fanSums != nil && fanCount > 0 {
		sum.Fans = make(map[string]uint16, len(fanSums))
		for key, value := range fanSums {
			sum.Fans[key] = uint16(value / fanCount)
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
			for i := range fs.DiskIoStats {
				fs.DiskIoStats[i] = twoDecimals(fs.DiskIoStats[i] / count)
			}
		}
	}

	// Average ZFS pool stats.
	for name, pool := range sum.ZfsPools {
		entryCount := zfsPoolCounts[name]
		pool.Total = twoDecimals(pool.Total / float64(zfsCapacityCounts[name]))
		pool.Used = twoDecimals(pool.Used / float64(zfsCapacityCounts[name]))
		pool.ReadBytes /= entryCount
		pool.WriteBytes /= entryCount
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

	return sum
}

func hasBattery(legacy [2]uint8, batteries map[string]uint8) bool {
	return legacy != [2]uint8{} || len(batteries) > 0
}

// Calculate the average stats of a list of container_stats records
func (rm *RecordManager) AverageContainerStats(db dbx.Builder, records RecordIds) []container.Stats {
	allStats := make([][]container.Stats, 0, len(records))
	var row StatsRecord
	params := make(dbx.Params, 1)
	for _, rec := range records {
		row.Stats = row.Stats[:0]
		params["id"] = rec.Id
		db.NewQuery("SELECT stats FROM container_stats WHERE id = {:id}").Bind(params).One(&row)
		var cs []container.Stats
		if err := json.Unmarshal(row.Stats, &cs); err != nil {
			return []container.Stats{}
		}
		allStats = append(allStats, cs)
	}
	return AverageContainerStatsSlice(allStats)
}

// AverageContainerStatsSlice computes the average of container stats across multiple time periods.
func AverageContainerStatsSlice(records [][]container.Stats) []container.Stats {
	if len(records) == 0 {
		return []container.Stats{}
	}
	sums := make(map[string]*container.Stats)
	count := float64(len(records))

	for _, containerStats := range records {
		for i := range containerStats {
			stat := &containerStats[i]
			if _, ok := sums[stat.Name]; !ok {
				sums[stat.Name] = &container.Stats{Name: stat.Name}
			}
			sums[stat.Name].Cpu += stat.Cpu
			sums[stat.Name].Mem += stat.Mem
			sentBytes := stat.Bandwidth[0]
			recvBytes := stat.Bandwidth[1]
			if sentBytes == 0 && recvBytes == 0 && (stat.NetworkSent != 0 || stat.NetworkRecv != 0) {
				sentBytes = uint64(stat.NetworkSent * 1024 * 1024)
				recvBytes = uint64(stat.NetworkRecv * 1024 * 1024)
			}
			sums[stat.Name].Bandwidth[0] += sentBytes
			sums[stat.Name].Bandwidth[1] += recvBytes
		}
	}

	result := make([]container.Stats, 0, len(sums))
	for _, value := range sums {
		result = append(result, container.Stats{
			Name:      value.Name,
			Cpu:       twoDecimals(value.Cpu / count),
			Mem:       twoDecimals(value.Mem / count),
			Bandwidth: [2]uint64{uint64(float64(value.Bandwidth[0]) / count), uint64(float64(value.Bandwidth[1]) / count)},
		})
	}
	return result
}

// AverageMonitorStats merges probe counts and response sums, preserving their
// weights through every retention tier. Failed probes do not contribute latency.
func (rm *RecordManager) AverageMonitorStats(db dbx.Builder, monitorID, recordType string, createdAfter int64) (monitor.Stats, int, error) {
	var result struct {
		monitor.Stats
		Count int `db:"count"`
	}
	err := db.Select(
		"COUNT(*) AS count",
		"COALESCE(SUM(total_count), 0) AS total_count",
		"COALESCE(SUM(success_count), 0) AS success_count",
		"COALESCE(SUM(res_sum), 0) AS res_sum",
		"COALESCE(MIN(CASE WHEN success_count > 0 THEN res_min END), 0) AS res_min",
		"COALESCE(MAX(CASE WHEN success_count > 0 THEN res_max END), 0) AS res_max",
	).From("network_monitor_stats").Where(dbx.NewExp(
		"monitor={:monitor} AND type={:type} AND created>{:created}",
		dbx.Params{"monitor": monitorID, "type": recordType, "created": createdAfter},
	)).One(&result)
	if err != nil {
		return monitor.Stats{}, 0, err
	}
	if result.SuccessCount > 0 {
		result.ResAvg = twoDecimals(float64(result.ResponseSum) / float64(result.SuccessCount))
	}
	if result.TotalCount > 0 {
		result.Loss = twoDecimals(float64(result.TotalCount-result.SuccessCount) * 100 / float64(result.TotalCount))
	}
	return result.Stats, result.Count, nil
}

/* Round float to two decimals */
func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
