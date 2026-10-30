package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// tagColors must match tagColorClasses in internal/site/src/lib/tag-utils.ts
var tagColors = []string{
	"red", "orange", "amber", "yellow", "lime", "green", "emerald", "teal", "cyan",
	"sky", "blue", "indigo", "violet", "purple", "fuchsia", "pink", "rose",
}

func init() {
	m.Register(func(app core.App) error {
		tagsCollection := core.NewBaseCollection("tags")
		tagsCollection.Id = "tags001"
		tagsCollection.ListRule = types.Pointer("@request.auth.id != \"\"")
		tagsCollection.ViewRule = types.Pointer("@request.auth.id != \"\"")
		writeRule := types.Pointer("@request.auth.id != \"\" && @request.auth.role != \"readonly\"")
		tagsCollection.CreateRule = writeRule
		tagsCollection.UpdateRule = writeRule
		tagsCollection.DeleteRule = writeRule

		tagsCollection.Fields.Add(
			&core.TextField{Id: "name", Name: "name", Required: true, Max: 50},
			&core.SelectField{Id: "color", Name: "color", MaxSelect: 1, Values: tagColors},
		)
		// names are trimmed by the hub (see hub/tags.go) before this index is checked
		tagsCollection.Indexes = []string{
			"CREATE UNIQUE INDEX `idx_tags_name` ON `tags` (name COLLATE NOCASE)",
		}
		if err := app.Save(tagsCollection); err != nil {
			return err
		}

		systemsCollection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		// not required, so PocketBase removes a deleted tag's id from systems automatically
		systemsCollection.Fields.Add(&core.RelationField{
			Id:           "tags",
			Name:         "tags",
			CollectionId: tagsCollection.Id,
			MaxSelect:    100,
		})
		return app.Save(systemsCollection)
	}, func(app core.App) error {
		systemsCollection, err := app.FindCollectionByNameOrId("systems")
		if err == nil {
			systemsCollection.Fields.RemoveByName("tags")
			if err := app.Save(systemsCollection); err != nil {
				return err
			}
		}

		tagsCollection, err := app.FindCollectionByNameOrId("tags")
		if err == nil {
			return app.Delete(tagsCollection)
		}
		return nil
	})
}
