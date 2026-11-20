package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
			{
				"id": "monitors_collection_001",
				"name": "monitors",
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
						"id": "type_field",
						"name": "type",
						"type": "select",
						"required": true,
						"values": ["http", "https", "tcp", "ping", "dns", "keyword", "json-query", "docker"]
					},
					{
						"id": "url_field",
						"name": "url",
						"type": "text",
						"required": false
					},
					{
						"id": "hostname_field",
						"name": "hostname",
						"type": "text",
						"required": false
					},
					{
						"id": "port_field",
						"name": "port",
						"type": "number",
						"required": false,
						"onlyInt": true
					},
					{
						"id": "method_field",
						"name": "method",
						"type": "select",
						"required": false,
						"values": ["GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH"]
					},
					{
						"id": "headers_field",
						"name": "headers",
						"type": "text",
						"required": false
					},
					{
						"id": "body_field",
						"name": "body",
						"type": "text",
						"required": false
					},
					{
						"id": "interval_field",
						"name": "interval",
						"type": "number",
						"required": true,
						"onlyInt": true,
						"min": 20,
						"max": 86400
					},
					{
						"id": "timeout_field",
						"name": "timeout",
						"type": "number",
						"required": true,
						"onlyInt": true,
						"min": 1,
						"max": 300
					},
					{
						"id": "retries_field",
						"name": "retries",
						"type": "number",
						"required": true,
						"onlyInt": true,
						"min": 0,
						"max": 10
					},
					{
						"id": "retry_interval_field",
						"name": "retry_interval",
						"type": "number",
						"required": false,
						"onlyInt": true
					},
					{
						"id": "max_redirects_field",
						"name": "max_redirects",
						"type": "number",
						"required": false,
						"onlyInt": true
					},
					{
						"id": "keyword_field",
						"name": "keyword",
						"type": "text",
						"required": false
					},
					{
						"id": "json_query_field",
						"name": "json_query",
						"type": "text",
						"required": false
					},
					{
						"id": "expected_value_field",
						"name": "expected_value",
						"type": "text",
						"required": false
					},
					{
						"id": "invert_keyword_field",
						"name": "invert_keyword",
						"type": "bool",
						"required": false
					},
					{
						"id": "dns_resolve_server_field",
						"name": "dns_resolve_server",
						"type": "text",
						"required": false
					},
					{
						"id": "dns_resolver_mode_field",
						"name": "dns_resolver_mode",
						"type": "select",
						"required": false,
						"values": ["A", "AAAA", "CNAME", "MX", "NS", "SOA", "SRV", "TXT", "PTR"]
					},
					{
						"id": "status_field",
						"name": "status",
						"type": "select",
						"required": false,
						"values": ["up", "down", "pending", "paused", "maintenance"]
					},
					{
						"id": "active_field",
						"name": "active",
						"type": "bool",
						"required": true
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
						"id": "description_field",
						"name": "description",
						"type": "text",
						"required": false
					},
					{
						"id": "cert_expiry_notification_field",
						"name": "cert_expiry_notification",
						"type": "bool",
						"required": false
					},
					{
						"id": "cert_expiry_days_field",
						"name": "cert_expiry_days",
						"type": "number",
						"required": false
					},
					{
						"id": "ignore_tls_error_field",
						"name": "ignore_tls_error",
						"type": "bool",
						"required": false
					},
					{
						"id": "last_check_field",
						"name": "last_check",
						"type": "date",
						"required": false
					},
					{
						"id": "uptime_stats_field",
						"name": "uptime_stats",
						"type": "json",
						"required": false
					},
					{
						"id": "tags_field",
						"name": "tags",
						"type": "json",
						"required": false
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
					"CREATE INDEX idx_monitor_user ON monitors (user)",
					"CREATE INDEX idx_monitor_status ON monitors (status)",
					"CREATE INDEX idx_monitor_active ON monitors (active)"
				]
			},
			{
				"id": "heartbeats_collection_001",
				"name": "monitor_heartbeats",
				"type": "base",
				"listRule": "@request.auth.id != '' && monitor.user = @request.auth.id",
				"viewRule": "@request.auth.id != '' && monitor.user = @request.auth.id",
				"createRule": null,
				"updateRule": null,
				"deleteRule": "@request.auth.id != '' && monitor.user = @request.auth.id",
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
						"id": "monitor_field",
						"name": "monitor",
						"type": "relation",
						"required": true,
						"collectionId": "monitors_collection_001",
						"maxSelect": 1,
						"cascadeDelete": true
					},
					{
						"id": "status_field",
						"name": "status",
						"type": "select",
						"required": true,
						"values": ["up", "down", "pending"]
					},
					{
						"id": "ping_field",
						"name": "ping",
						"type": "number",
						"required": false
					},
					{
						"id": "msg_field",
						"name": "msg",
						"type": "text",
						"required": false
					},
					{
						"id": "cert_expiry_field",
						"name": "cert_expiry",
						"type": "number",
						"required": false
					},
					{
						"id": "cert_valid_field",
						"name": "cert_valid",
						"type": "bool",
						"required": false
					},
					{
						"id": "time_field",
						"name": "time",
						"type": "date",
						"required": true
					}
				],
				"indexes": [
				"CREATE INDEX idx_heartbeat_monitor ON monitor_heartbeats (monitor)",
				"CREATE INDEX idx_heartbeat_time ON monitor_heartbeats (time)"
			]
		},
		{
			"id": "domains_collection_001",
			"name": "domains",
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
					"id": "domain_name_field",
					"name": "domain_name",
					"type": "text",
					"required": true
				},
				{
					"id": "status_field",
					"name": "status",
					"type": "select",
					"required": false,
					"values": ["active", "expiring", "expired", "unknown", "paused"]
				},
				{
					"id": "active_field",
					"name": "active",
					"type": "bool",
					"required": true
				},
				{
					"id": "expiry_date_field",
					"name": "expiry_date",
					"type": "date",
					"required": false
				},
				{
					"id": "creation_date_field",
					"name": "creation_date",
					"type": "date",
					"required": false
				},
				{
					"id": "updated_date_field",
					"name": "updated_date",
					"type": "date",
					"required": false
				},
				{
					"id": "registrar_name_field",
					"name": "registrar_name",
					"type": "text",
					"required": false
				},
				{
					"id": "registrar_id_field",
					"name": "registrar_id",
					"type": "text",
					"required": false
				},
				{
					"id": "registrar_url_field",
					"name": "registrar_url",
					"type": "text",
					"required": false
				},
				{
					"id": "dnssec_field",
					"name": "dnssec",
					"type": "text",
					"required": false
				},
				{
					"id": "name_servers_field",
					"name": "name_servers",
					"type": "json",
					"required": false
				},
				{
					"id": "mx_records_field",
					"name": "mx_records",
					"type": "json",
					"required": false
				},
				{
					"id": "txt_records_field",
					"name": "txt_records",
					"type": "json",
					"required": false
				},
				{
					"id": "ipv4_addresses_field",
					"name": "ipv4_addresses",
					"type": "json",
					"required": false
				},
				{
					"id": "ipv6_addresses_field",
					"name": "ipv6_addresses",
					"type": "json",
					"required": false
				},
				{
					"id": "ssl_issuer_field",
					"name": "ssl_issuer",
					"type": "text",
					"required": false
				},
				{
					"id": "ssl_valid_to_field",
					"name": "ssl_valid_to",
					"type": "date",
					"required": false
				},
				{
					"id": "host_country_field",
					"name": "host_country",
					"type": "text",
					"required": false
				},
				{
					"id": "host_isp_field",
					"name": "host_isp",
					"type": "text",
					"required": false
				},
				{
					"id": "purchase_price_field",
					"name": "purchase_price",
					"type": "number",
					"required": false
				},
				{
					"id": "current_value_field",
					"name": "current_value",
					"type": "number",
					"required": false
				},
				{
					"id": "renewal_cost_field",
					"name": "renewal_cost",
					"type": "number",
					"required": false
				},
				{
					"id": "auto_renew_field",
					"name": "auto_renew",
					"type": "bool",
					"required": false
				},
				{
					"id": "alert_days_before_field",
					"name": "alert_days_before",
					"type": "number",
					"required": false
				},
				{
					"id": "ssl_alert_enabled_field",
					"name": "ssl_alert_enabled",
					"type": "bool",
					"required": false
				},
				{
					"id": "tags_field",
					"name": "tags",
					"type": "json",
					"required": false
				},
				{
					"id": "notes_field",
					"name": "notes",
					"type": "text",
					"required": false
				},
				{
					"id": "favicon_url_field",
					"name": "favicon_url",
					"type": "text",
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
					"id": "last_checked_field",
					"name": "last_checked",
					"type": "date",
					"required": false
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
				"CREATE INDEX idx_domain_user ON domains (user)",
				"CREATE INDEX idx_domain_status ON domains (status)",
				"CREATE INDEX idx_domain_expiry ON domains (expiry_date)"
			]
		},
		{
			"id": "domain_history_collection_001",
			"name": "domain_history",
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
					"id": "change_type_field",
					"name": "change_type",
					"type": "select",
					"required": true,
					"values": ["expiry", "ssl", "dns", "registrar", "ip", "host", "status"]
				},
				{
					"id": "field_name_field",
					"name": "field_name",
					"type": "text",
					"required": true
				},
				{
					"id": "old_value_field",
					"name": "old_value",
					"type": "text",
					"required": false
				},
				{
					"id": "new_value_field",
					"name": "new_value",
					"type": "text",
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
					"id": "created_at_field",
					"name": "created_at",
					"type": "date",
					"required": true
				}
			],
			"indexes": [
				"CREATE INDEX idx_history_domain ON domain_history (domain)",
				"CREATE INDEX idx_history_created ON domain_history (created_at)"
			]
		},
		{
			"id": "incidents_collection_001",
			"name": "incidents",
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
					"id": "title_field",
					"name": "title",
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
					"id": "type_field",
					"name": "type",
					"type": "select",
					"required": true,
					"values": ["monitor_down", "monitor_up", "domain_expiring", "domain_expired", "ssl_expiring", "system_offline", "system_online"]
				},
				{
					"id": "severity_field",
					"name": "severity",
					"type": "select",
					"required": true,
					"values": ["critical", "high", "medium", "low"]
				},
				{
					"id": "status_field",
					"name": "status",
					"type": "select",
					"required": true,
					"values": ["open", "acknowledged", "resolved", "closed"]
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
					"id": "assigned_to_field",
					"name": "assigned_to",
					"type": "relation",
					"required": false,
					"collectionId": "_pb_users_auth_",
					"maxSelect": 1
				},
				{
					"id": "started_at_field",
					"name": "started_at",
					"type": "date",
					"required": true
				},
				{
					"id": "acknowledged_at_field",
					"name": "acknowledged_at",
					"type": "date",
					"required": false
				},
				{
					"id": "resolved_at_field",
					"name": "resolved_at",
					"type": "date",
					"required": false
				},
				{
					"id": "closed_at_field",
					"name": "closed_at",
					"type": "date",
					"required": false
				},
				{
					"id": "resolution_field",
					"name": "resolution",
					"type": "text",
					"required": false
				},
				{
					"id": "root_cause_field",
					"name": "root_cause",
					"type": "text",
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
				"CREATE INDEX idx_incident_user ON incidents (user)",
				"CREATE INDEX idx_incident_status ON incidents (status)",
				"CREATE INDEX idx_incident_severity ON incidents (severity)",
				"CREATE INDEX idx_incident_started ON incidents (started_at)"
			]
		},
		{
			"id": "incident_updates_collection_001",
			"name": "incident_updates",
			"type": "base",
			"listRule": "@request.auth.id != '' && incident.user = @request.auth.id",
			"viewRule": "@request.auth.id != '' && incident.user = @request.auth.id",
			"createRule": null,
			"updateRule": null,
			"deleteRule": "@request.auth.id != '' && incident.user = @request.auth.id",
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
					"id": "incident_field",
					"name": "incident",
					"type": "relation",
					"required": true,
					"collectionId": "incidents_collection_001",
					"maxSelect": 1,
					"cascadeDelete": true
				},
				{
					"id": "message_field",
					"name": "message",
					"type": "text",
					"required": true
				},
				{
					"id": "update_type_field",
					"name": "update_type",
					"type": "select",
					"required": true,
					"values": ["note", "status_change", "assignment"]
				},
				{
					"id": "old_status_field",
					"name": "old_status",
					"type": "text",
					"required": false
				},
				{
					"id": "new_status_field",
					"name": "new_status",
					"type": "text",
					"required": false
				},
				{
					"id": "created_by_field",
					"name": "created_by",
					"type": "relation",
					"required": true,
					"collectionId": "_pb_users_auth_",
					"maxSelect": 1
				},
				{
					"id": "created_at_field",
					"name": "created_at",
					"type": "date",
					"required": true
				}
			],
			"indexes": [
				"CREATE INDEX idx_update_incident ON incident_updates (incident)",
				"CREATE INDEX idx_update_created ON incident_updates (created_at)"
			]
		},
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
					"values": ["light", "dark", "auto"]
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
					"values": ["scheduled", "in_progress", "completed", "cancelled"]
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
					"values": ["active", "inactive", "unknown"]
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
					"values": ["status", "uptime", "response", "domain"]
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
					"values": ["flat", "flat-square", "plastic", "for-the-badge"]
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
		}
	]`

		err := app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
		if err != nil {
			return err
		}
		return nil
	}, func(app core.App) error {
		return nil
	})
}
