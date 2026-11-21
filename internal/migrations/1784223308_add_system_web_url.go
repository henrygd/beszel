package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}

		// web_url lets a user override the auto-derived http://<host> link
		// shown on the system's name in the dashboard - useful for servers
		// whose own web page uses a different scheme/port than the agent
		// connection, or for Unix-socket-connected agents, which have no
		// derivable network address at all.
		collection.Fields.Add(&core.TextField{
			Id:   "sysweburl01",
			Name: "web_url",
			Max:  500,
		})

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}

		collection.Fields.RemoveById("sysweburl01")

		return app.Save(collection)
	})
}
