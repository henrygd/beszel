package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		systems, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		if _, err := app.FindCollectionByNameOrId("pod_stats"); err == nil {
			return nil
		}

		podStats := core.NewBaseCollection("pod_stats", "k8s_pod_stats01")
		podStats.Fields.Add(
			&core.RelationField{
				Id:            "pod_system01",
				Name:          "system",
				Required:      true,
				MaxSelect:     1,
				CollectionId:  systems.Id,
				CascadeDelete: true,
			},
			&core.JSONField{
				Id:       "pod_stats01",
				Name:     "stats",
				Required: true,
				MaxSize:  2_000_000,
			},
			&core.SelectField{
				Id:        "pod_type01",
				Name:      "type",
				Required:  true,
				MaxSelect: 1,
				Values:    []string{"1m", "10m", "20m", "120m", "480m"},
			},
			// PocketBase 0.23+ base collections do not add created/updated unless
			// these fields exist. The rollup queries and this index both use created.
			&core.AutodateField{
				Id:       "pod_stats_created",
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Id:       "pod_stats_updated",
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		podStats.AddIndex("idx_pod_stats01", false, "`system`, `type`, `created`", "")
		if err := app.Save(podStats); err != nil {
			return err
		}

		minZero := 0.0
		maxCPU := 100.0
		pods := core.NewBaseCollection("pods", "k8s_pods_collection")
		pods.Fields.Add(
			&core.RelationField{
				Id:           "relation_pod_system",
				Name:         "system",
				MaxSelect:    1,
				CollectionId: systems.Id,
			},
			&core.TextField{Id: "text_pod_name", Name: "name"},
			&core.TextField{Id: "text_pod_namespace", Name: "namespace"},
			&core.TextField{Id: "text_pod_status", Name: "status"},
			&core.NumberField{Id: "number_pod_cpu", Name: "cpu", Min: &minZero, Max: &maxCPU},
			&core.NumberField{Id: "number_pod_memory", Name: "memory", Min: &minZero},
			&core.NumberField{Id: "number_pod_net", Name: "net"},
			&core.NumberField{Id: "number_pod_restarts", Name: "restarts", Min: &minZero, OnlyInt: true},
			&core.NumberField{Id: "number_pod_updated", Name: "updated", OnlyInt: true, Required: true},
		)
		pods.AddIndex("idx_pod_updated", false, "updated", "")
		pods.AddIndex("idx_pod_system", false, "system", "")
		return app.Save(pods)
	}, func(app core.App) error {
		pods, err := app.FindCollectionByNameOrId("pods")
		if err == nil {
			if err := app.Delete(pods); err != nil {
				return err
			}
		}
		podStats, err := app.FindCollectionByNameOrId("pod_stats")
		if err == nil {
			return app.Delete(podStats)
		}
		return nil
	})
}
