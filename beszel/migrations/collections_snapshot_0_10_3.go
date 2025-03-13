package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
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
					"Bandwidth"
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
		"indexes": [],
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
	}
]`

		return app.ImportCollectionsByMarshaledJSON([]byte(jsonData), false)
	}, func(app core.App) error {
		return nil
	})
}
