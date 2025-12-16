package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// update collections
		jsonData := `[
	{
		"id": "elngm8x1l60zi2v",
		"listRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"viewRule": "",
		"createRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"updateRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"deleteRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"name": "alerts",
		"type": "base",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{15}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 15,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "_pb_users_auth_",
				"hidden": false,
				"id": "hn5ly3vi",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "user",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "g5sl3jdg",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"hidden": false,
				"id": "zj3ingrv",
				"maxSelect": 1,
				"name": "name",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "select",
				"values": [
					"Status",
					"CPU",
					"Memory",
					"Disk",
					"Temperature",
					"Bandwidth",
					"GPU",
					"LoadAvg1",
					"LoadAvg5",
					"LoadAvg15",
					"Battery"
				]
			},
			{
				"hidden": false,
				"id": "o2ablxvn",
				"max": null,
				"min": null,
				"name": "value",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "fstdehcq",
				"max": 60,
				"min": null,
				"name": "min",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "6hgdf6hs",
				"name": "triggered",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "bool"
			},
			{
				"hidden": false,
				"id": "autodate2990389176",
				"name": "created",
				"onCreate": true,
				"onUpdate": false,
				"presentable": false,
				"system": false,
				"type": "autodate"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"indexes": [
			"CREATE UNIQUE INDEX ` + "`" + `idx_MnhEt21L5r` + "`" + ` ON ` + "`" + `alerts` + "`" + ` (\n  ` + "`" + `user` + "`" + `,\n  ` + "`" + `system` + "`" + `,\n  ` + "`" + `name` + "`" + `\n)"
		],
		"system": false
	},
	{
		"id": "pbc_1697146157",
		"listRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"viewRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"createRule": null,
		"updateRule": null,
		"deleteRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"name": "alerts_history",
		"type": "base",
		"fields": [
			{
					"autogeneratePattern": "[a-z0-9]{15}",
					"hidden": false,
					"id": "text3208210256",
					"max": 15,
					"min": 15,
					"name": "id",
					"pattern": "^[a-z0-9]+$",
					"presentable": false,
					"primaryKey": true,
					"required": true,
					"system": true,
					"type": "text"
				},
				{
					"cascadeDelete": true,
					"collectionId": "_pb_users_auth_",
					"hidden": false,
					"id": "relation2375276105",
					"maxSelect": 1,
					"minSelect": 0,
					"name": "user",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "relation"
				},
				{
					"cascadeDelete": true,
					"collectionId": "2hz5ncl8tizk5nx",
					"hidden": false,
					"id": "relation3377271179",
					"maxSelect": 1,
					"minSelect": 0,
					"name": "system",
					"presentable": false,
					"required": true,
					"system": false,
					"type": "relation"
				},
				{
					"autogeneratePattern": "",
					"hidden": false,
					"id": "text2466471794",
					"max": 0,
					"min": 0,
					"name": "alert_id",
					"pattern": "",
					"presentable": false,
					"primaryKey": false,
					"required": false,
					"system": false,
					"type": "text"
				},
				{
					"autogeneratePattern": "",
					"hidden": false,
					"id": "text1579384326",
					"max": 0,
					"min": 0,
					"name": "name",
					"pattern": "",
					"presentable": false,
					"primaryKey": false,
					"required": true,
					"system": false,
					"type": "text"
				},
				{
					"hidden": false,
					"id": "number494360628",
					"max": null,
					"min": null,
					"name": "value",
					"onlyInt": false,
					"presentable": false,
					"required": false,
					"system": false,
					"type": "number"
				},
				{
					"hidden": false,
					"id": "autodate2990389176",
					"name": "created",
					"onCreate": true,
					"onUpdate": false,
					"presentable": false,
					"system": false,
					"type": "autodate"
				},
				{
					"hidden": false,
					"id": "date2276568630",
					"max": "",
					"min": "",
					"name": "resolved",
					"presentable": false,
					"required": false,
					"system": false,
					"type": "date"
				}
		],
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_YdGnup5aqB` + "`" + ` ON ` + "`" + `alerts_history` + "`" + ` (` + "`" + `user` + "`" + `)",
			"CREATE INDEX ` + "`" + `idx_taLet9VdME` + "`" + ` ON ` + "`" + `alerts_history` + "`" + ` (` + "`" + `created` + "`" + `)"
		],
		"system": false
	},
	{
		"id": "juohu4jipgc13v7",
		"listRule": "@request.auth.id != \"\"",
		"viewRule": null,
		"createRule": null,
		"updateRule": null,
		"deleteRule": null,
		"name": "container_stats",
		"type": "base",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{15}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 15,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "hutcu6ps",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"hidden": false,
				"id": "r39hhnil",
				"maxSize": 2000000,
				"name": "stats",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "vo7iuj96",
				"maxSelect": 1,
				"name": "type",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "select",
				"values": [
					"1m",
					"10m",
					"20m",
					"120m",
					"480m"
				]
			},
			{
				"hidden": false,
				"id": "autodate2990389176",
				"name": "created",
				"onCreate": true,
				"onUpdate": false,
				"presentable": false,
				"system": false,
				"type": "autodate"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_d87OiXGZD8` + "`" + ` ON ` + "`" + `container_stats` + "`" + ` (\n  ` + "`" + `system` + "`" + `,\n  ` + "`" + `type` + "`" + `,\n  ` + "`" + `created` + "`" + `\n)"
		],
		"system": false
	},
	{
		"id": "pbc_3663931638",
		"listRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id",
		"viewRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id",
		"createRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
		"updateRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
		"deleteRule": null,
		"name": "fingerprints",
		"type": "base",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{9}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 9,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "relation3377271179",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"autogeneratePattern": "[a-zA-Z9-9]{20}",
				"hidden": false,
				"id": "text1597481275",
				"max": 255,
				"min": 9,
				"name": "token",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": true,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text4228609354",
				"max": 255,
				"min": 9,
				"name": "fingerprint",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_p9qZlu26po` + "`" + ` ON ` + "`" + `fingerprints` + "`" + ` (` + "`" + `token` + "`" + `)",
			"CREATE UNIQUE INDEX ` + "`" + `idx_ngboulGMYw` + "`" + ` ON ` + "`" + `fingerprints` + "`" + ` (` + "`" + `system` + "`" + `)"
		],
		"system": false
	},
	{
		"id": "ej9oowivz8b2mht",
		"listRule": "@request.auth.id != \"\"",
		"viewRule": null,
		"createRule": null,
		"updateRule": null,
		"deleteRule": null,
		"name": "system_stats",
		"type": "base",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{15}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 15,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "h9sg148r",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"hidden": false,
				"id": "azftn0be",
				"maxSize": 2000000,
				"name": "stats",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "m1ekhli3",
				"maxSelect": 1,
				"name": "type",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "select",
				"values": [
					"1m",
					"10m",
					"20m",
					"120m",
					"480m"
				]
			},
			{
				"hidden": false,
				"id": "autodate2990389176",
				"name": "created",
				"onCreate": true,
				"onUpdate": false,
				"presentable": false,
				"system": false,
				"type": "autodate"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_GxIee0j` + "`" + ` ON ` + "`" + `system_stats` + "`" + ` (\n  ` + "`" + `system` + "`" + `,\n  ` + "`" + `type` + "`" + `,\n  ` + "`" + `created` + "`" + `\n)"
		],
		"system": false
	},
	{
		"id": "4afacsdnlu8q8r2",
		"listRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"viewRule": null,
		"createRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"updateRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"deleteRule": null,
		"name": "user_settings",
		"type": "base",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{15}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 15,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "_pb_users_auth_",
				"hidden": false,
				"id": "d5vztyxa",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "user",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"hidden": false,
				"id": "xcx4qgqq",
				"maxSize": 2000000,
				"name": "settings",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "autodate2990389176",
				"name": "created",
				"onCreate": true,
				"onUpdate": false,
				"presentable": false,
				"system": false,
				"type": "autodate"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"indexes": [
			"CREATE UNIQUE INDEX ` + "`" + `idx_30Lwgf2` + "`" + ` ON ` + "`" + `user_settings` + "`" + ` (` + "`" + `user` + "`" + `)"
		],
		"system": false
	},
	{
		"id": "2hz5ncl8tizk5nx",
		"listRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id",
		"viewRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id",
		"createRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
		"updateRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
		"deleteRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
		"name": "systems",
		"type": "base",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{15}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 15,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "7xloxkwk",
				"max": 0,
				"min": 0,
				"name": "name",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": true,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "waj7seaf",
				"maxSelect": 1,
				"name": "status",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "select",
				"values": [
					"up",
					"down",
					"paused",
					"pending"
				]
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "ve781smf",
				"max": 0,
				"min": 0,
				"name": "host",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": true,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "pij0k2jk",
				"max": 0,
				"min": 0,
				"name": "port",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "qoq64ntl",
				"maxSize": 2000000,
				"name": "info",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "json"
			},
			{
				"cascadeDelete": true,
				"collectionId": "_pb_users_auth_",
				"hidden": false,
				"id": "jcarjnjj",
				"maxSelect": 2147483647,
				"minSelect": 0,
				"name": "users",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"hidden": false,
				"id": "autodate2990389176",
				"name": "created",
				"onCreate": true,
				"onUpdate": false,
				"presentable": false,
				"system": false,
				"type": "autodate"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_systems_status` + "`" + ` ON ` + "`" + `systems` + "`" + ` (` + "`" + `status` + "`" + `)"
		],
		"system": false
	},
	{
		"id": "_pb_users_auth_",
		"listRule": "id = @request.auth.id",
		"viewRule": "id = @request.auth.id",
		"createRule": null,
		"updateRule": null,
		"deleteRule": null,
		"name": "users",
		"type": "auth",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{15}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 15,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cost": 10,
				"hidden": true,
				"id": "password901924565",
				"max": 0,
				"min": 8,
				"name": "password",
				"pattern": "",
				"presentable": false,
				"required": true,
				"system": true,
				"type": "password"
			},
			{
				"autogeneratePattern": "[a-zA-Z0-9_]{50}",
				"hidden": true,
				"id": "text2504183744",
				"max": 60,
				"min": 30,
				"name": "tokenKey",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"exceptDomains": null,
				"hidden": false,
				"id": "email3885137012",
				"name": "email",
				"onlyDomains": null,
				"presentable": false,
				"required": true,
				"system": true,
				"type": "email"
			},
			{
				"hidden": false,
				"id": "bool1547992806",
				"name": "emailVisibility",
				"presentable": false,
				"required": false,
				"system": true,
				"type": "bool"
			},
			{
				"hidden": false,
				"id": "bool256245529",
				"name": "verified",
				"presentable": false,
				"required": false,
				"system": true,
				"type": "bool"
			},
			{
				"autogeneratePattern": "users[0-9]{6}",
				"hidden": false,
				"id": "text4166911607",
				"max": 150,
				"min": 3,
				"name": "username",
				"pattern": "^[\\w][\\w\\.\\-]*$",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "qkbp58ae",
				"maxSelect": 1,
				"name": "role",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "select",
				"values": [
					"user",
					"admin",
					"readonly"
				]
			},
			{
				"hidden": false,
				"id": "autodate2990389176",
				"name": "created",
				"onCreate": true,
				"onUpdate": false,
				"presentable": false,
				"system": false,
				"type": "autodate"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"indexes": [
			"CREATE UNIQUE INDEX ` + "`" + `__pb_users_auth__username_idx` + "`" + ` ON ` + "`" + `users` + "`" + ` (username COLLATE NOCASE)",
			"CREATE UNIQUE INDEX ` + "`" + `__pb_users_auth__email_idx` + "`" + ` ON ` + "`" + `users` + "`" + ` (` + "`" + `email` + "`" + `) WHERE ` + "`" + `email` + "`" + ` != ''",
			"CREATE UNIQUE INDEX ` + "`" + `__pb_users_auth__tokenKey_idx` + "`" + ` ON ` + "`" + `users` + "`" + ` (` + "`" + `tokenKey` + "`" + `)"
		],
		"system": false,
		"authRule": "verified=true",
		"manageRule": null
	},
	{
		"id": "pbc_1864144027",
		"listRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id",
		"viewRule": null,
		"createRule": null,
		"updateRule": null,
		"deleteRule": null,
		"name": "containers",
		"type": "base",
		"fields": [
				{
						"autogeneratePattern": "[a-f0-9]{6}",
						"hidden": false,
						"id": "text3208210256",
						"max": 12,
						"min": 6,
						"name": "id",
						"pattern": "^[a-f0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
				},
				{
						"cascadeDelete": false,
						"collectionId": "2hz5ncl8tizk5nx",
						"hidden": false,
						"id": "relation3377271179",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "system",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
				},
				{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text1579384326",
						"max": 0,
						"min": 0,
						"name": "name",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
				},
				{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text2063623452",
						"max": 0,
						"min": 0,
						"name": "status",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
				},
				{
						"hidden": false,
						"id": "number3470402323",
						"max": null,
						"min": null,
						"name": "health",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
				},
				{
						"hidden": false,
						"id": "number3128971310",
						"max": 100,
						"min": 0,
						"name": "cpu",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
				},
				{
						"hidden": false,
						"id": "number3933025333",
						"max": null,
						"min": 0,
						"name": "memory",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
				},
				{
					"hidden": false,
					"id": "number4075427327",
					"max": null,
					"min": null,
					"name": "net",
					"onlyInt": false,
					"presentable": false,
					"required": false,
					"system": false,
					"type": "number"
				},
				{
					"hidden": false,
					"id": "number3332085495",
					"max": null,
					"min": null,
					"name": "updated",
					"onlyInt": true,
					"presentable": false,
					"required": true,
					"system": false,
					"type": "number"
				},
				{
					"autogeneratePattern": "",
					"hidden": false,
					"id": "text3309110367",
					"max": 0,
					"min": 0,
					"name": "image",
					"pattern": "",
					"presentable": false,
					"primaryKey": false,
					"required": false,
					"system": false,
					"type": "text"
				}
		],
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_JxWirjdhyO` + "`" + ` ON ` + "`" + `containers` + "`" + ` (` + "`" + `updated` + "`" + `)",
			"CREATE INDEX ` + "`" + `idx_r3Ja0rs102` + "`" + ` ON ` + "`" + `containers` + "`" + ` (` + "`" + `system` + "`" + `)"
		],
		"system": false
	},
	{
		"createRule": null,
		"deleteRule": null,
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{10}",
				"hidden": false,
				"id": "text3208210256",
				"max": 10,
				"min": 6,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text1579384326",
				"max": 0,
				"min": 0,
				"name": "name",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "relation3377271179",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "relation"
			},
			{
				"hidden": false,
				"id": "number2063623452",
				"max": null,
				"min": null,
				"name": "state",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number1476559580",
				"max": null,
				"min": null,
				"name": "sub",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number3128971310",
				"max": null,
				"min": null,
				"name": "cpu",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number1052053287",
				"max": null,
				"min": null,
				"name": "cpuPeak",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number3933025333",
				"max": null,
				"min": null,
				"name": "memory",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number1828797201",
				"max": null,
				"min": null,
				"name": "memPeak",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number3332085495",
				"max": null,
				"min": null,
				"name": "updated",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			}
		],
		"id": "pbc_3494996990",
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_4Z7LuLNdQb` + "`" + ` ON ` + "`" + `systemd_services` + "`" + ` (` + "`" + `system` + "`" + `)",
			"CREATE INDEX ` + "`" + `idx_pBp1fF837e` + "`" + ` ON ` + "`" + `systemd_services` + "`" + ` (` + "`" + `updated` + "`" + `)"
		],
		"listRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id",
		"name": "systemd_services",
		"system": false,
		"type": "base",
		"updateRule": null,
		"viewRule": null
	},
	{
		"createRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"deleteRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{10}",
				"hidden": false,
				"id": "text3208210256",
				"max": 10,
				"min": 10,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "_pb_users_auth_",
				"hidden": false,
				"id": "relation2375276105",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "user",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "relation"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "relation3377271179",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "relation"
			},
			{
				"hidden": false,
				"id": "select2844932856",
				"maxSelect": 1,
				"name": "type",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "select",
				"values": [
					"one-time",
					"daily"
				]
			},
			{
				"hidden": false,
				"id": "date2675529103",
				"max": "",
				"min": "",
				"name": "start",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "date"
			},
			{
				"hidden": false,
				"id": "date16528305",
				"max": "",
				"min": "",
				"name": "end",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "date"
			}
		],
		"id": "pbc_451525641",
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_q0iKnRP9v8` + "`" + ` ON ` + "`" + `quiet_hours` + "`" + ` (\n  ` + "`" + `user` + "`" + `,\n  ` + "`" + `system` + "`" + `\n)",
			"CREATE INDEX ` + "`" + `idx_6T7ljT7FJd` + "`" + ` ON ` + "`" + `quiet_hours` + "`" + ` (\n  ` + "`" + `type` + "`" + `,\n  ` + "`" + `end` + "`" + `\n)"
		],
		"listRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"name": "quiet_hours",
		"system": false,
		"type": "base",
		"updateRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
		"viewRule": "@request.auth.id != \"\" && user.id = @request.auth.id"
	},
	{
		"createRule": null,
		"deleteRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{10}",
				"hidden": false,
				"id": "text3208210256",
				"max": 10,
				"min": 10,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "relation3377271179",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text1579384326",
				"max": 0,
				"min": 0,
				"name": "name",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text3616895705",
				"max": 0,
				"min": 0,
				"name": "model",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text2744374011",
				"max": 0,
				"min": 0,
				"name": "state",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "number3051925876",
				"max": null,
				"min": null,
				"name": "capacity",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number190023114",
				"max": null,
				"min": null,
				"name": "temp",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text3589068740",
				"max": 0,
				"min": 0,
				"name": "firmware",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text3547646428",
				"max": 0,
				"min": 0,
				"name": "serial",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text2363381545",
				"max": 0,
				"min": 0,
				"name": "type",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "number1234567890",
				"max": null,
				"min": null,
				"name": "hours",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number0987654321",
				"max": null,
				"min": null,
				"name": "cycles",
				"onlyInt": true,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "json832282224",
				"maxSize": 0,
				"name": "attributes",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "json"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"id": "pbc_2571630677",
		"indexes": [
			"CREATE INDEX ` + "`" + `idx_DZ9yhvgl44` + "`" + ` ON ` + "`" + `smart_devices` + "`" + ` (` + "`" + `system` + "`" + `)"
		],
		"listRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id",
		"name": "smart_devices",
		"system": false,
		"type": "base",
		"updateRule": null,
		"viewRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id"
	},
	{
		"createRule": "",
		"deleteRule": "",
		"fields": [
			{
				"autogeneratePattern": "[a-z0-9]{15}",
				"hidden": false,
				"id": "text3208210256",
				"max": 15,
				"min": 15,
				"name": "id",
				"pattern": "^[a-z0-9]+$",
				"presentable": false,
				"primaryKey": true,
				"required": true,
				"system": true,
				"type": "text"
			},
			{
				"cascadeDelete": true,
				"collectionId": "2hz5ncl8tizk5nx",
				"hidden": false,
				"id": "relation3377271179",
				"maxSelect": 1,
				"minSelect": 0,
				"name": "system",
				"presentable": false,
				"required": true,
				"system": false,
				"type": "relation"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text3847340049",
				"max": 0,
				"min": 0,
				"name": "hostname",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "number1789936913",
				"max": null,
				"min": null,
				"name": "os",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text2818598173",
				"max": 0,
				"min": 0,
				"name": "os_name",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text1574083243",
				"max": 0,
				"min": 0,
				"name": "kernel",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text3128971310",
				"max": 0,
				"min": 0,
				"name": "cpu",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"autogeneratePattern": "",
				"hidden": false,
				"id": "text4161937994",
				"max": 0,
				"min": 0,
				"name": "arch",
				"pattern": "",
				"presentable": false,
				"primaryKey": false,
				"required": false,
				"system": false,
				"type": "text"
			},
			{
				"hidden": false,
				"id": "number4245036687",
				"max": null,
				"min": null,
				"name": "cores",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number1871592925",
				"max": null,
				"min": null,
				"name": "threads",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "number3933025333",
				"max": null,
				"min": null,
				"name": "memory",
				"onlyInt": false,
				"presentable": false,
				"required": false,
				"system": false,
				"type": "number"
			},
			{
				"hidden": false,
				"id": "bool2200265312",
				"name": "podman",
				"presentable": false,
				"required": false,
				"system": false,
				"type": "bool"
			},
			{
				"hidden": false,
				"id": "autodate3332085495",
				"name": "updated",
				"onCreate": true,
				"onUpdate": true,
				"presentable": false,
				"system": false,
				"type": "autodate"
			}
		],
		"id": "pbc_3116237454",
		"indexes": [],
		"listRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id",
		"name": "system_details",
		"system": false,
		"type": "base",
		"updateRule": "",
		"viewRule": "@request.auth.id != \"\" && system.users.id ?= @request.auth.id"
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
