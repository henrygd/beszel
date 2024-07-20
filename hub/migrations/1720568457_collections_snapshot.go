package migrations

import (
	"encoding/json"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/models"
)

func init() {
	m.Register(func(db dbx.Builder) error {
		jsonData := `[
			{
				"id": "2hz5ncl8tizk5nx",
				"created": "2024-07-07 16:08:20.979Z",
				"updated": "2024-07-17 15:27:00.429Z",
				"name": "systems",
				"type": "base",
				"system": false,
				"schema": [
					{
						"system": false,
						"id": "7xloxkwk",
						"name": "name",
						"type": "text",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"min": null,
							"max": null,
							"pattern": ""
						}
					},
					{
						"system": false,
						"id": "waj7seaf",
						"name": "status",
						"type": "select",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSelect": 1,
							"values": [
								"up",
								"down",
								"paused",
								"pending"
							]
						}
					},
					{
						"system": false,
						"id": "ve781smf",
						"name": "host",
						"type": "text",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"min": null,
							"max": null,
							"pattern": ""
						}
					},
					{
						"system": false,
						"id": "pij0k2jk",
						"name": "port",
						"type": "text",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"min": null,
							"max": null,
							"pattern": ""
						}
					},
					{
						"system": false,
						"id": "qoq64ntl",
						"name": "info",
						"type": "json",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSize": 2000000
						}
					},
					{
						"system": false,
						"id": "jcarjnjj",
						"name": "users",
						"type": "relation",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"collectionId": "_pb_users_auth_",
							"cascadeDelete": false,
							"minSelect": null,
							"maxSelect": null,
							"displayFields": null
						}
					}
				],
				"indexes": [],
				"listRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id",
				"viewRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id",
				"createRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
				"updateRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
				"deleteRule": "@request.auth.id != \"\" && users.id ?= @request.auth.id && @request.auth.role != \"readonly\"",
				"options": {}
			},
			{
				"id": "ej9oowivz8b2mht",
				"created": "2024-07-07 16:09:09.179Z",
				"updated": "2024-07-18 15:56:45.302Z",
				"name": "system_stats",
				"type": "base",
				"system": false,
				"schema": [
					{
						"system": false,
						"id": "h9sg148r",
						"name": "system",
						"type": "relation",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"collectionId": "2hz5ncl8tizk5nx",
							"cascadeDelete": true,
							"minSelect": null,
							"maxSelect": 1,
							"displayFields": null
						}
					},
					{
						"system": false,
						"id": "azftn0be",
						"name": "stats",
						"type": "json",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSize": 2000000
						}
					},
					{
						"system": false,
						"id": "m1ekhli3",
						"name": "type",
						"type": "select",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSelect": 1,
							"values": [
								"1m",
								"10m",
								"20m",
								"120m",
								"480m"
							]
						}
					}
				],
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_GxIee0j` + "`" + ` ON ` + "`" + `system_stats` + "`" + ` (` + "`" + `system` + "`" + `)"
				],
				"listRule": "@request.auth.id != \"\"",
				"viewRule": null,
				"createRule": null,
				"updateRule": null,
				"deleteRule": null,
				"options": {}
			},
			{
				"id": "juohu4jipgc13v7",
				"created": "2024-07-07 16:09:57.976Z",
				"updated": "2024-07-18 15:57:50.933Z",
				"name": "container_stats",
				"type": "base",
				"system": false,
				"schema": [
					{
						"system": false,
						"id": "hutcu6ps",
						"name": "system",
						"type": "relation",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"collectionId": "2hz5ncl8tizk5nx",
							"cascadeDelete": true,
							"minSelect": null,
							"maxSelect": 1,
							"displayFields": null
						}
					},
					{
						"system": false,
						"id": "r39hhnil",
						"name": "stats",
						"type": "json",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSize": 2000000
						}
					},
					{
						"system": false,
						"id": "vo7iuj96",
						"name": "type",
						"type": "select",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSelect": 1,
							"values": [
								"1m",
								"10m",
								"20m",
								"120m",
								"480m"
							]
						}
					}
				],
				"indexes": [],
				"listRule": "@request.auth.id != \"\"",
				"viewRule": null,
				"createRule": null,
				"updateRule": null,
				"deleteRule": null,
				"options": {}
			},
			{
				"id": "_pb_users_auth_",
				"created": "2024-07-14 16:25:18.226Z",
				"updated": "2024-07-20 00:55:02.071Z",
				"name": "users",
				"type": "auth",
				"system": false,
				"schema": [
					{
						"system": false,
						"id": "qkbp58ae",
						"name": "role",
						"type": "select",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSelect": 1,
							"values": [
								"user",
								"admin",
								"readonly"
							]
						}
					},
					{
						"system": false,
						"id": "users_avatar",
						"name": "avatar",
						"type": "file",
						"required": false,
						"presentable": false,
						"unique": false,
						"options": {
							"mimeTypes": [
								"image/jpeg",
								"image/png",
								"image/svg+xml",
								"image/gif",
								"image/webp"
							],
							"thumbs": null,
							"maxSelect": 1,
							"maxSize": 5242880,
							"protected": false
						}
					}
				],
				"indexes": [],
				"listRule": "id = @request.auth.id",
				"viewRule": "id = @request.auth.id",
				"createRule": null,
				"updateRule": null,
				"deleteRule": null,
				"options": {
					"allowEmailAuth": true,
					"allowOAuth2Auth": true,
					"allowUsernameAuth": true,
					"exceptEmailDomains": null,
					"manageRule": null,
					"minPasswordLength": 8,
					"onlyEmailDomains": null,
					"onlyVerified": true,
					"requireEmail": false
				}
			},
			{
				"id": "elngm8x1l60zi2v",
				"created": "2024-07-15 01:16:04.044Z",
				"updated": "2024-07-15 22:44:12.297Z",
				"name": "alerts",
				"type": "base",
				"system": false,
				"schema": [
					{
						"system": false,
						"id": "hn5ly3vi",
						"name": "user",
						"type": "relation",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"collectionId": "_pb_users_auth_",
							"cascadeDelete": true,
							"minSelect": null,
							"maxSelect": 1,
							"displayFields": null
						}
					},
					{
						"system": false,
						"id": "g5sl3jdg",
						"name": "system",
						"type": "relation",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"collectionId": "2hz5ncl8tizk5nx",
							"cascadeDelete": true,
							"minSelect": null,
							"maxSelect": 1,
							"displayFields": null
						}
					},
					{
						"system": false,
						"id": "zj3ingrv",
						"name": "name",
						"type": "select",
						"required": true,
						"presentable": false,
						"unique": false,
						"options": {
							"maxSelect": 1,
							"values": [
								"status"
							]
						}
					}
				],
				"indexes": [],
				"listRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
				"viewRule": "",
				"createRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
				"updateRule": null,
				"deleteRule": "@request.auth.id != \"\" && user.id = @request.auth.id",
				"options": {}
			}
		]`

		collections := []*models.Collection{}
		if err := json.Unmarshal([]byte(jsonData), &collections); err != nil {
			return err
		}

		return daos.New(db).ImportCollections(collections, true, nil)
	}, func(db dbx.Builder) error {
		return nil
	})
}
