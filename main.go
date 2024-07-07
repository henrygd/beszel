package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/cron"

	_ "monitor-site/migrations"
)

func main() {
	app := pocketbase.New()

	// loosely check if it was executed using "go run"
	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())
	// enable auto creation of migration files when making collection changes in the Admin UI
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		// (the isGoRun check is to enable it only during development)
		Automigrate: isGoRun,
	})

	// set up cron job to delete records older than 30 days
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		scheduler := cron.New()
		scheduler.MustAdd("delete old records", "* 2 * * *", func() {
			// log.Println("Deleting old records...")
			// Get the current time
			now := time.Now().UTC()
			// Subtract one month
			oneMonthAgo := now.AddDate(0, 0, -30)
			// Format the time as a string
			timeString := oneMonthAgo.Format("2006-01-02 15:04:05")
			// collections to be cleaned
			collections := []string{"systems", "system_stats", "container_stats"}

			for _, collection := range collections {
				records, err := app.Dao().FindRecordsByFilter(
					collection,
					fmt.Sprintf("updated <= \"%s\"", timeString), // filter
					"", // sort
					-1, // limit
					0,  // offset
				)
				if err != nil {
					log.Println(err)
					return
				}
				// delete records
				for _, record := range records {
					if err := app.Dao().DeleteRecord(record); err != nil {
						log.Fatal(err)
					}
				}
			}
		})
		scheduler.Start()
		return nil
	})

	// serves static files from the provided public dir (if exists)
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		e.Router.GET("/*", apis.StaticDirectoryHandler(os.DirFS("./pb_public"), false))
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
