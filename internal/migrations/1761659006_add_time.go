package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	var collectionNames = []string{"system_stats", "container_stats"}

	m.Register(func(app core.App) error {
		return app.RunInTransaction(func(txApp core.App) error {
			updateCollection := func(collectionName string) error {
				fmt.Println("Updating collection:", collectionName)
				collection, err := txApp.FindCollectionByNameOrId(collectionName)
				if err != nil {
					return err
				}

				collection.Fields.Add(&core.DateField{
					Name:     "timestamp",
					Required: true,
				})

				err = txApp.Save(collection)
				if err != nil {
					return err
				}

				query := fmt.Sprintf("UPDATE %s SET timestamp = created", collectionName)
				fmt.Println("Running query:", query)
				_, err = txApp.DB().NewQuery(query).Execute()
				return err
			}

			for _, collectionName := range collectionNames {
				err := updateCollection(collectionName)
				if err != nil {
					return err
				}
			}

			return nil
		})
	}, func(app core.App) error {
		return app.RunInTransaction(func(txApp core.App) error {
			revertCollection := func(collectionName string) error {
				fmt.Println("Reverting collection:", collectionName)
				collection, err := txApp.FindCollectionByNameOrId(collectionName)
				if err != nil {
					return err
				}
				collection.Fields.RemoveByName("timestamp")
				return txApp.Save(collection)
			}

			for _, collectionName := range collectionNames {
				err := revertCollection(collectionName)
				if err != nil {
					return err
				}
			}

			return nil
		})
	})
}
