package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Add index to support batched retention delete: WHERE type=? AND created<?
		// Existing index is (system, type, created) which doesn't help queries without system filter
		for _, colName := range []string{"system_stats", "container_stats"} {
			col, err := app.FindCollectionByNameOrId(colName)
			if err != nil {
				continue
			}
			// avoid duplicate index if already exists
			has := false
			for _, idx := range col.Indexes {
				if idx == "CREATE INDEX `idx_type_created` ON `"+colName+"` (`type`, `created`)" {
					has = true
					break
				}
			}
			if has {
				continue
			}
			col.Indexes = append(col.Indexes, "CREATE INDEX `idx_type_created` ON `"+colName+"` (`type`, `created`)")
			if err := app.Save(col); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		for _, colName := range []string{"system_stats", "container_stats"} {
			col, err := app.FindCollectionByNameOrId(colName)
			if err != nil {
				continue
			}
			newIdx := make([]string, 0, len(col.Indexes))
			for _, idx := range col.Indexes {
				if idx != "CREATE INDEX `idx_type_created` ON `"+colName+"` (`type`, `created`)" {
					newIdx = append(newIdx, idx)
				}
			}
			col.Indexes = newIdx
			_ = app.Save(col)
		}
		return nil
	})
}
