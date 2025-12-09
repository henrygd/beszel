package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Create tags collection
		tagsCollection := core.NewBaseCollection("tags")
		tagsCollection.Id = "tags001"
		tagsCollection.ListRule = strPtr("@request.auth.id != \"\"")
		tagsCollection.ViewRule = strPtr("@request.auth.id != \"\"")
		tagsCollection.CreateRule = strPtr("@request.auth.id != \"\" && @request.auth.role != \"readonly\"")
		tagsCollection.UpdateRule = strPtr("@request.auth.id != \"\" && @request.auth.role != \"readonly\"")
		tagsCollection.DeleteRule = strPtr("@request.auth.id != \"\" && @request.auth.role != \"readonly\"")

		// Add fields to tags collection
		tagsCollection.Fields.Add(
			&core.TextField{
				Id:       "name",
				Name:     "name",
				Required: true,
				Max:      50,
			},
		)
		tagsCollection.Fields.Add(
			&core.TextField{
				Id:      "color",
				Name:    "color",
				Required: false,
				Max:     7, // hex color #RRGGBB
			},
		)

		// Create unique index on name
		tagsCollection.Indexes = []string{
			"CREATE UNIQUE INDEX `idx_tags_name` ON `tags` (`name`)",
		}

		if err := app.Save(tagsCollection); err != nil {
			return err
		}

		// Update systems collection to add tags relation field
		systemsCollection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}

		systemsCollection.Fields.Add(
			&core.RelationField{
				Id:           "tags",
				Name:         "tags",
				Required:     false,
				CollectionId: "tags001",
				MaxSelect:    100, // max 100 tags per system
				MinSelect:    0,
			},
		)

		return app.Save(systemsCollection)
	}, func(app core.App) error {
		// Rollback: remove tags field from systems and delete tags collection
		systemsCollection, err := app.FindCollectionByNameOrId("systems")
		if err == nil {
			systemsCollection.Fields.RemoveById("tags")
			app.Save(systemsCollection)
		}

		tagsCollection, err := app.FindCollectionByNameOrId("tags")
		if err == nil {
			app.Delete(tagsCollection)
		}

		return nil
	})
}

// Helper function to create string pointer
func strPtr(s string) *string {
	return &s
}
