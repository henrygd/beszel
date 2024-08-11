package records

import (
	"beszel/internal/entities/container"
	"beszel/internal/entities/system"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

type RecordManager struct {
	app *pocketbase.PocketBase
}

func NewRecordManager(app *pocketbase.PocketBase) *RecordManager {
	return &RecordManager{app: app}
}

func (rm *RecordManager) CreateLongerRecords(collectionName string, shorterRecord *models.Record) {
	shorterRecordType := shorterRecord.GetString("type")
	systemId := shorterRecord.GetString("system")
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
	lastLongerRecord, err := rm.app.Dao().FindFirstRecordByFilter(
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
	allShorterRecords, err := rm.app.Dao().FindRecordsByFilter(
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
	// average the shorter records and create longer record
	var stats interface{}
	switch collectionName {
	case "system_stats":
		stats = rm.AverageSystemStats(allShorterRecords)
	case "container_stats":
		stats = rm.AverageContainerStats(allShorterRecords)
	}
	collection, _ := rm.app.Dao().FindCollectionByNameOrId(collectionName)
	longerRecord := models.NewRecord(collection)
	longerRecord.Set("system", systemId)
	longerRecord.Set("stats", stats)
	longerRecord.Set("type", longerRecordType)
	if err := rm.app.Dao().SaveRecord(longerRecord); err != nil {
		fmt.Println("failed to save longer record", "err", err.Error())
	}

}

// calculate the average stats of a list of system_stats records
func (rm *RecordManager) AverageSystemStats(records []*models.Record) system.SystemStats {
	count := float64(len(records))
	sum := reflect.New(reflect.TypeOf(system.SystemStats{})).Elem()

	for _, record := range records {
		var stats system.SystemStats
		record.UnmarshalJSONField("stats", &stats)
		statValue := reflect.ValueOf(stats)
		for i := 0; i < statValue.NumField(); i++ {
			field := sum.Field(i)
			field.SetFloat(field.Float() + statValue.Field(i).Float())
		}
	}

	average := reflect.New(reflect.TypeOf(system.SystemStats{})).Elem()
	for i := 0; i < sum.NumField(); i++ {
		average.Field(i).SetFloat(twoDecimals(sum.Field(i).Float() / count))
	}

	return average.Interface().(system.SystemStats)
}

// calculate the average stats of a list of container_stats records
func (rm *RecordManager) AverageContainerStats(records []*models.Record) (stats []container.ContainerStats) {
	sums := make(map[string]*container.ContainerStats)
	count := float64(len(records))
	for _, record := range records {
		var stats []container.ContainerStats
		record.UnmarshalJSONField("stats", &stats)
		for _, stat := range stats {
			if _, ok := sums[stat.Name]; !ok {
				sums[stat.Name] = &container.ContainerStats{Name: stat.Name, Cpu: 0, Mem: 0}
			}
			sums[stat.Name].Cpu += stat.Cpu
			sums[stat.Name].Mem += stat.Mem
			sums[stat.Name].NetworkSent += stat.NetworkSent
			sums[stat.Name].NetworkRecv += stat.NetworkRecv
		}
	}
	for _, value := range sums {
		stats = append(stats, container.ContainerStats{
			Name:        value.Name,
			Cpu:         twoDecimals(value.Cpu / count),
			Mem:         twoDecimals(value.Mem / count),
			NetworkSent: twoDecimals(value.NetworkSent / count),
			NetworkRecv: twoDecimals(value.NetworkRecv / count),
		})
	}
	return stats
}

/* Round float to two decimals */
func twoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

/* Delete records of specified collections and type that are older than timeLimit */
func (rm *RecordManager) DeleteOldRecords(collections []string, recordType string, timeLimit time.Duration) {
	timeLimitStamp := time.Now().UTC().Add(-timeLimit).Format("2006-01-02 15:04:05")

	// db query
	expType := dbx.NewExp("type = {:type}", dbx.Params{"type": recordType})
	expCreated := dbx.NewExp("created < {:created}", dbx.Params{"created": timeLimitStamp})

	var records []*models.Record
	for _, collection := range collections {
		if collectionRecords, err := rm.app.Dao().FindRecordsByExpr(collection, expType, expCreated); err == nil {
			records = append(records, collectionRecords...)
		}
	}

	rm.app.Dao().RunInTransaction(func(txDao *daos.Dao) error {
		for _, record := range records {
			err := txDao.DeleteRecord(record)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
