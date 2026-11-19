package migrations

import (
	"encoding/json"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Migration 1 was edited after some deployments had already applied it, so
// several collections it defines may be absent on upgraded data dirs.
// notifications/monitor_notifications/notification_events were never in a
// migration at all. This migration creates any missing monitoring
// collections and adds the per-monitor notification toggles the site dialog
// submits. Everything is idempotent.
func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
		{
				"id": "status_pages_collection_001",
				"name": "status_pages",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"updateRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"fields": [
						{
								"id": "id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "name_field",
								"name": "name",
								"type": "text",
								"required": true
						},
						{
								"id": "slug_field",
								"name": "slug",
								"type": "text",
								"required": true
						},
						{
								"id": "title_field",
								"name": "title",
								"type": "text",
								"required": false
						},
						{
								"id": "description_field",
								"name": "description",
								"type": "text",
								"required": false
						},
						{
								"id": "logo_field",
								"name": "logo",
								"type": "text",
								"required": false
						},
						{
								"id": "favicon_field",
								"name": "favicon",
								"type": "text",
								"required": false
						},
						{
								"id": "theme_field",
								"name": "theme",
								"type": "select",
								"required": false,
								"values": [
										"light",
										"dark",
										"auto"
								]
						},
						{
								"id": "custom_css_field",
								"name": "custom_css",
								"type": "text",
								"required": false
						},
						{
								"id": "public_field",
								"name": "public",
								"type": "bool",
								"required": false
						},
						{
								"id": "show_uptime_field",
								"name": "show_uptime",
								"type": "bool",
								"required": false
						},
						{
								"id": "user_field",
								"name": "user",
								"type": "relation",
								"required": true,
								"collectionId": "_pb_users_auth_",
								"maxSelect": 1
						},
						{
								"id": "created_field",
								"name": "created",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": false
						},
						{
								"id": "updated_field",
								"name": "updated",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": true
						}
				],
				"indexes": [
						"CREATE UNIQUE INDEX idx_status_page_slug ON status_pages (slug)",
						"CREATE INDEX idx_status_page_user ON status_pages (user)"
				]
		},
		{
				"id": "status_page_monitors_collection_001",
				"name": "status_page_monitors",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": null,
				"updateRule": null,
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id",
				"fields": [
						{
								"id": "id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "status_page_field",
								"name": "status_page",
								"type": "relation",
								"required": true,
								"collectionId": "status_pages_collection_001",
								"maxSelect": 1,
								"cascadeDelete": true
						},
						{
								"id": "monitor_field",
								"name": "monitor",
								"type": "relation",
								"required": true,
								"collectionId": "monitors_collection_001",
								"maxSelect": 1
						},
						{
								"id": "display_name_field",
								"name": "display_name",
								"type": "text",
								"required": false
						},
						{
								"id": "group_field",
								"name": "group",
								"type": "text",
								"required": false
						},
						{
								"id": "sort_order_field",
								"name": "sort_order",
								"type": "number",
								"required": false,
								"onlyInt": true
						},
						{
								"id": "user_field",
								"name": "user",
								"type": "relation",
								"required": true,
								"collectionId": "_pb_users_auth_",
								"maxSelect": 1
						},
						{
								"id": "created_field",
								"name": "created",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": false
						}
				],
				"indexes": [
						"CREATE INDEX idx_status_page_monitor_page ON status_page_monitors (status_page)",
						"CREATE INDEX idx_status_page_monitor_monitor ON status_page_monitors (monitor)"
				]
		},
		{
				"id": "maintenance_windows_collection_001",
				"name": "maintenance_windows",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"updateRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"fields": [
						{
								"id": "id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "name_field",
								"name": "name",
								"type": "text",
								"required": true
						},
						{
								"id": "description_field",
								"name": "description",
								"type": "text",
								"required": false
						},
						{
								"id": "monitor_field",
								"name": "monitor",
								"type": "relation",
								"required": false,
								"collectionId": "monitors_collection_001",
								"maxSelect": 1
						},
						{
								"id": "domain_field",
								"name": "domain",
								"type": "relation",
								"required": false,
								"collectionId": "domains_collection_001",
								"maxSelect": 1
						},
						{
								"id": "start_time_field",
								"name": "start_time",
								"type": "date",
								"required": true
						},
						{
								"id": "end_time_field",
								"name": "end_time",
								"type": "date",
								"required": true
						},
						{
								"id": "recurring_field",
								"name": "recurring",
								"type": "bool",
								"required": false
						},
						{
								"id": "recurrence_pattern_field",
								"name": "recurrence_pattern",
								"type": "text",
								"required": false
						},
						{
								"id": "status_field",
								"name": "status",
								"type": "select",
								"required": true,
								"values": [
										"scheduled",
										"in_progress",
										"completed",
										"cancelled"
								]
						},
						{
								"id": "suppress_alerts_field",
								"name": "suppress_alerts",
								"type": "bool",
								"required": false
						},
						{
								"id": "user_field",
								"name": "user",
								"type": "relation",
								"required": true,
								"collectionId": "_pb_users_auth_",
								"maxSelect": 1
						},
						{
								"id": "created_field",
								"name": "created",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": false
						},
						{
								"id": "updated_field",
								"name": "updated",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": true
						}
				],
				"indexes": [
						"CREATE INDEX idx_maintenance_user ON maintenance_windows (user)",
						"CREATE INDEX idx_maintenance_monitor ON maintenance_windows (monitor)",
						"CREATE INDEX idx_maintenance_domain ON maintenance_windows (domain)",
						"CREATE INDEX idx_maintenance_status ON maintenance_windows (status)",
						"CREATE INDEX idx_maintenance_time ON maintenance_windows (start_time, end_time)"
				]
		},
		{
				"id": "subdomains_collection_001",
				"name": "subdomains",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": null,
				"updateRule": null,
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id",
				"fields": [
						{
								"id": "id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "domain_field",
								"name": "domain",
								"type": "relation",
								"required": true,
								"collectionId": "domains_collection_001",
								"maxSelect": 1,
								"cascadeDelete": true
						},
						{
								"id": "subdomain_name_field",
								"name": "subdomain_name",
								"type": "text",
								"required": true
						},
						{
								"id": "status_field",
								"name": "status",
								"type": "select",
								"required": true,
								"values": [
										"active",
										"inactive",
										"unknown"
								]
						},
						{
								"id": "ip_addresses_field",
								"name": "ip_addresses",
								"type": "text",
								"required": false
						},
						{
								"id": "last_checked_field",
								"name": "last_checked",
								"type": "date",
								"required": false
						},
						{
								"id": "user_field",
								"name": "user",
								"type": "relation",
								"required": true,
								"collectionId": "_pb_users_auth_",
								"maxSelect": 1
						},
						{
								"id": "created_field",
								"name": "created",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": false
						}
				],
				"indexes": [
						"CREATE INDEX idx_subdomain_domain ON subdomains (domain)",
						"CREATE INDEX idx_subdomain_user ON subdomains (user)",
						"CREATE INDEX idx_subdomain_name ON subdomains (subdomain_name)"
				]
		},
		{
				"id": "badges_collection_001",
				"name": "badges",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": "@request.auth.id != '' && user = @request.auth.id",
				"updateRule": "@request.auth.id != '' && user = @request.auth.id",
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id",
				"fields": [
						{
								"id": "id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "name_field",
								"name": "name",
								"type": "text",
								"required": true
						},
						{
								"id": "type_field",
								"name": "type",
								"type": "select",
								"required": true,
								"values": [
										"status",
										"uptime",
										"response",
										"domain"
								]
						},
						{
								"id": "monitor_field",
								"name": "monitor",
								"type": "relation",
								"required": false,
								"collectionId": "monitors_collection_001",
								"maxSelect": 1
						},
						{
								"id": "domain_field",
								"name": "domain",
								"type": "relation",
								"required": false,
								"collectionId": "domains_collection_001",
								"maxSelect": 1
						},
						{
								"id": "system_field",
								"name": "system",
								"type": "relation",
								"required": false,
								"collectionId": "2hz5ncl8tizk5nx",
								"maxSelect": 1
						},
						{
								"id": "status_page_field",
								"name": "status_page",
								"type": "relation",
								"required": false,
								"collectionId": "status_pages_collection_001",
								"maxSelect": 1
						},
						{
								"id": "label_field",
								"name": "label",
								"type": "text",
								"required": false
						},
						{
								"id": "color_field",
								"name": "color",
								"type": "text",
								"required": false
						},
						{
								"id": "style_field",
								"name": "style",
								"type": "select",
								"required": false,
								"values": [
										"flat",
										"flat-square",
										"plastic",
										"for-the-badge"
								]
						},
						{
								"id": "user_field",
								"name": "user",
								"type": "relation",
								"required": true,
								"collectionId": "_pb_users_auth_",
								"maxSelect": 1
						},
						{
								"id": "created_field",
								"name": "created",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": false
						},
						{
								"id": "updated_field",
								"name": "updated",
								"type": "autodate",
								"onCreate": true,
								"onUpdate": true
						}
				],
				"indexes": [
						"CREATE INDEX idx_badge_user ON badges (user)",
						"CREATE INDEX idx_badge_monitor ON badges (monitor)",
						"CREATE INDEX idx_badge_domain ON badges (domain)",
						"CREATE INDEX idx_badge_system ON badges (system)"
				]
		},
		{
				"id": "notifications_collection_001",
				"name": "notifications",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"updateRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"fields": [
						{
								"id": "notif_id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "notif_name_field",
								"name": "name",
								"type": "text",
								"required": true
						},
						{
								"id": "notif_type_field",
								"name": "type",
								"type": "select",
								"required": true,
								"values": [
										"email",
										"webhook",
										"discord",
										"slack",
										"telegram",
										"gotify",
										"pushover",
										"push"
								],
								"maxSelect": 1
						},
						{
								"id": "notif_settings_field",
								"name": "settings",
								"type": "json"
						},
						{
								"id": "notif_is_default_field",
								"name": "is_default",
								"type": "bool"
						},
						{
								"id": "notif_active_field",
								"name": "active",
								"type": "bool"
						},
						{
								"id": "notif_user_field",
								"name": "user",
								"type": "relation",
								"required": true,
								"collectionId": "_pb_users_auth_",
								"cascadeDelete": true,
								"maxSelect": 1
						}
				],
				"indexes": [
						"CREATE INDEX idx_notification_user ON notifications (user)"
				]
		},
		{
				"id": "monitor_notifications_collection_001",
				"name": "monitor_notifications",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"updateRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"fields": [
						{
								"id": "mn_id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "mn_monitor_field",
								"name": "monitor",
								"type": "relation",
								"required": true,
								"collectionId": "monitors_collection_001",
								"cascadeDelete": true,
								"maxSelect": 1
						},
						{
								"id": "mn_notification_field",
								"name": "notification",
								"type": "relation",
								"required": true,
								"collectionId": "notifications_collection_001",
								"cascadeDelete": true,
								"maxSelect": 1
						},
						{
								"id": "mn_user_field",
								"name": "user",
								"type": "relation",
								"required": true,
								"collectionId": "_pb_users_auth_",
								"cascadeDelete": true,
								"maxSelect": 1
						}
				],
				"indexes": [
						"CREATE UNIQUE INDEX idx_monitor_notification_unique ON monitor_notifications (monitor, notification)",
						"CREATE INDEX idx_monitor_notification_user ON monitor_notifications (user)"
				]
		},
		{
				"id": "notification_events_collection_001",
				"name": "notification_events",
				"type": "base",
				"listRule": "@request.auth.id != '' && user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && user = @request.auth.id",
				"createRule": null,
				"updateRule": null,
				"deleteRule": "@request.auth.id != '' && user = @request.auth.id && @request.auth.role != 'readonly'",
				"fields": [
						{
								"id": "ne_id_field",
								"name": "id",
								"type": "text",
								"system": true,
								"required": true,
								"primaryKey": true,
								"autogeneratePattern": "[a-z0-9]{15}",
								"max": 15,
								"min": 15,
								"pattern": "^[a-z0-9]+$"
						},
						{
								"id": "ne_notification_field",
								"name": "notification",
								"type": "relation",
								"required": false,
								"collectionId": "notifications_collection_001",
								"cascadeDelete": true,
								"maxSelect": 1
						},
						{
								"id": "ne_monitor_field",
								"name": "monitor",
								"type": "relation",
								"required": false,
								"collectionId": "monitors_collection_001",
								"cascadeDelete": true,
								"maxSelect": 1
						},
						{
								"id": "ne_status_field",
								"name": "status",
								"type": "select",
								"required": false,
								"values": [
										"sent",
										"failed",
										"skipped"
								],
								"maxSelect": 1
						},
						{
								"id": "ne_message_field",
								"name": "message",
								"type": "text"
						},
						{
								"id": "ne_error_field",
								"name": "error",
								"type": "text"
						},
						{
								"id": "ne_user_field",
								"name": "user",
								"type": "relation",
								"required": false,
								"collectionId": "_pb_users_auth_",
								"cascadeDelete": true,
								"maxSelect": 1
						}
				],
				"indexes": [
						"CREATE INDEX idx_notification_events_monitor ON notification_events (monitor)"
				]
		}
]`

		var defs []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &defs); err != nil {
			return err
		}
		for _, def := range defs {
			name, _ := def["name"].(string)
			if _, err := app.FindCollectionByNameOrId(name); err == nil {
				continue // already exists
			}
			single, err := json.Marshal([]map[string]interface{}{def})
			if err != nil {
				return err
			}
			if err := app.ImportCollectionsByMarshaledJSON(single, false); err != nil {
				return err
			}
		}

		notifyFields := []core.Field{
			&core.BoolField{Id: "mon_notify_on_down", Name: "notify_on_down"},
			&core.BoolField{Id: "mon_notify_on_recover", Name: "notify_on_recover"},
			&core.BoolField{Id: "mon_notify_on_resp_time", Name: "notify_on_response_time"},
			&core.NumberField{Id: "mon_resp_time_thresh", Name: "response_time_threshold"},
			&core.BoolField{Id: "mon_notify_uptime_drop", Name: "notify_on_uptime_drop"},
			&core.NumberField{Id: "mon_uptime_thresh", Name: "uptime_threshold"},
			&core.BoolField{Id: "mon_notify_repeat_fail", Name: "notify_repeated_failures"},
			&core.NumberField{Id: "mon_consec_fail", Name: "consecutive_failures"},
			&core.BoolField{Id: "mon_quiet_hours_en", Name: "quiet_hours_enabled"},
			&core.TextField{Id: "mon_quiet_hours_start", Name: "quiet_hours_start"},
			&core.TextField{Id: "mon_quiet_hours_end", Name: "quiet_hours_end"},
			&core.TextField{Id: "mon_db_username", Name: "db_username"},
			&core.TextField{Id: "mon_db_password", Name: "db_password"},
			&core.TextField{Id: "mon_db_name", Name: "db_name"},
			&core.TextField{Id: "mon_mqtt_topic", Name: "mqtt_topic"},
			&core.TextField{Id: "mon_grpc_keyword", Name: "grpc_keyword"},
		}
		monitors, err := app.FindCollectionByNameOrId("monitors")
		if err != nil {
			return err
		}
		for _, f := range notifyFields {
			if monitors.Fields.GetByName(f.GetName()) == nil {
				monitors.Fields.Add(f)
			}
		}
		if err := app.Save(monitors); err != nil {
			return err
		}
		// New columns default to false; legacy monitors should notify
		// unless the user explicitly disables it.
		_, err = app.DB().NewQuery("UPDATE monitors SET notify_on_down = 1, notify_on_recover = 1").Execute()
		return err
	}, func(app core.App) error {
		return nil // down: no-op, collections/columns may be in use
	})
}
