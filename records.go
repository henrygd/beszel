package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"reflect"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/types"
)

func createLongerRecords(collectionName string, shorterRecord *models.Record) {
	shorterRecordType := shorterRecord.Get("type").(string)
	systemId := shorterRecord.Get("system").(string)
	// fmt.Println("create longer records", "recordType", shorterRecordType, "systemId", systemId)
	var longerRecordType string
	var timeAgo time.Duration
	var expectedShorterRecords int
	switch shorterRecordType {
	case "1m":
		longerRecordType = "10m"
		timeAgo = -10 * time.Minute
		expectedShorterRecords = 10
	case "10m":
		longerRecordType = "20m"
		timeAgo = -20 * time.Minute
		expectedShorterRecords = 2
	case "20m":
		longerRecordType = "120m"
		timeAgo = -120 * time.Minute
		expectedShorterRecords = 6
	default:
		longerRecordType = "480m"
		timeAgo = -480 * time.Minute
		expectedShorterRecords = 4
	}

	longerRecordPeriod := time.Now().UTC().Add(timeAgo + 10*time.Second).Format("2006-01-02 15:04:05")
	// check creation time of last 10m record
	lastLongerRecord, err := app.Dao().FindFirstRecordByFilter(
		collectionName,
		"type = {:type} && system = {:system} && created > {:created}",
		dbx.Params{"type": longerRecordType, "system": systemId, "created": longerRecordPeriod},
	)
	// return if longer record exists
	if err == nil || lastLongerRecord != nil {
		// log.Println("longer record found. returning")
		return
	}
	// get shorter records from the past x minutes
	// shorterRecordPeriod := time.Now().UTC().Add(timeAgo + time.Second).Format("2006-01-02 15:04:05")
	allShorterRecords, err := app.Dao().FindRecordsByFilter(
		collectionName,
		"type = {:type} && system = {:system} && created > {:created}",
		"-created",
		-1,
		0,
		dbx.Params{"type": shorterRecordType, "system": systemId, "created": longerRecordPeriod},
	)
	// return if not enough shorter records
	if err != nil || len(allShorterRecords) < expectedShorterRecords {
		// log.Println("not enough shorter records. returning")
		return
	}
	// average the shorter records and create 10m record
	averagedStats := averageSystemStats(allShorterRecords)
	collection, _ := app.Dao().FindCollectionByNameOrId(collectionName)
	tenMinRecord := models.NewRecord(collection)
	tenMinRecord.Set("system", systemId)
	tenMinRecord.Set("stats", averagedStats)
	tenMinRecord.Set("type", longerRecordType)
	if err := app.Dao().SaveRecord(tenMinRecord); err != nil {
		fmt.Println("failed to save longer record", "err", err.Error())
	}

}

// calculate the average of a list of SystemStats using reflection
func averageSystemStats(records []*models.Record) SystemStats {
	count := float64(len(records))
	sum := reflect.New(reflect.TypeOf(SystemStats{})).Elem()

	for _, record := range records {
		var stats SystemStats
		json.Unmarshal([]byte(record.Get("stats").(types.JsonRaw)), &stats)
		statValue := reflect.ValueOf(stats)
		for i := 0; i < statValue.NumField(); i++ {
			field := sum.Field(i)
			field.SetFloat(field.Float() + statValue.Field(i).Float())
		}
	}

	average := reflect.New(reflect.TypeOf(SystemStats{})).Elem()
	for i := 0; i < sum.NumField(); i++ {
		average.Field(i).SetFloat(twoDecimals(sum.Field(i).Float() / count))
	}

	return average.Interface().(SystemStats)
}

// func averageSystemStats(records []*models.Record) SystemStats {
// 	var sum SystemStats
// 	count := float64(len(records))

// 	for _, record := range records {
// 		var stats SystemStats
// 		json.Unmarshal([]byte(record.Get("stats").(types.JsonRaw)), &stats)
// 		sum.Cpu += stats.Cpu
// 		sum.Mem += stats.Mem
// 		sum.MemUsed += stats.MemUsed
// 		sum.MemPct += stats.MemPct
// 		sum.MemBuffCache += stats.MemBuffCache
// 		sum.Disk += stats.Disk
// 		sum.DiskUsed += stats.DiskUsed
// 		sum.DiskPct += stats.DiskPct
// 		sum.DiskRead += stats.DiskRead
// 		sum.DiskWrite += stats.DiskWrite
// 		sum.NetworkSent += stats.NetworkSent
// 		sum.NetworkRecv += stats.NetworkRecv
// 	}

// 	return SystemStats{
// 		Cpu:          twoDecimals(sum.Cpu / count),
// 		Mem:          twoDecimals(sum.Mem / count),
// 		MemUsed:      twoDecimals(sum.MemUsed / count),
// 		MemPct:       twoDecimals(sum.MemPct / count),
// 		MemBuffCache: twoDecimals(sum.MemBuffCache / count),
// 		Disk:         twoDecimals(sum.Disk / count),
// 		DiskUsed:     twoDecimals(sum.DiskUsed / count),
// 		DiskPct:      twoDecimals(sum.DiskPct / count),
// 		DiskRead:     twoDecimals(sum.DiskRead / count),
// 		DiskWrite:    twoDecimals(sum.DiskWrite / count),
// 		NetworkSent:  twoDecimals(sum.NetworkSent / count),
// 		NetworkRecv:  twoDecimals(sum.NetworkRecv / count),
// 	}
// }

/* Round float to two decimals */
func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

/* Delete records of specified collection and type that are older than timeLimit */
func deleteOldRecords(collection string, recordType string, timeLimit time.Duration) {
	log.Println("Deleting old", recordType, "records...")
	timeLimitStamp := time.Now().UTC().Add(timeLimit).Format("2006-01-02 15:04:05")
	records, _ := app.Dao().FindRecordsByExpr(collection,
		dbx.NewExp("type = {:type}", dbx.Params{"type": recordType}),
		dbx.NewExp("created > {:created}", dbx.Params{"created": timeLimitStamp}),
	)
	for _, record := range records {
		if err := app.Dao().DeleteRecord(record); err != nil {
			log.Fatal(err)
		}
	}
}
