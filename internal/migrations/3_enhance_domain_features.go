package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if err := enhanceDomainCollection(app); err != nil {
			return err
		}
		return nil
	}, func(app core.App) error {
		return nil
	})
}

func enhanceDomainCollection(app core.App) error {
	collection, err := app.FindCollectionByNameOrId("domains")
	if err != nil {
		return err
	}

	// Provider detection fields
	addTextField3(collection, "dns_provider")
	addTextField3(collection, "hosting_provider")
	addTextField3(collection, "email_provider")
	addTextField3(collection, "ca_provider")

	// JSON fields for complex data
	addJSONField3(collection, "headers")
	addJSONField3(collection, "certificates")
	addJSONField3(collection, "seo_meta")
	addJSONField3(collection, "domain_statuses")

	// WHOIS and registration fields
	addTextField3(collection, "whois_raw")
	addBoolField3(collection, "privacy_enabled")
	addBoolField3(collection, "transfer_lock")
	addTextField3(collection, "tld")

	// Enhanced geo
	addTextField3(collection, "host_country_code")

	return app.Save(collection)
}

func addTextField3(collection *core.Collection, name string) {
	if collection.Fields.GetByName(name) == nil {
		collection.Fields.Add(&core.TextField{Name: name})
	}
}

func addBoolField3(collection *core.Collection, name string) {
	if collection.Fields.GetByName(name) == nil {
		collection.Fields.Add(&core.BoolField{Name: name})
	}
}

func addJSONField3(collection *core.Collection, name string) {
	if collection.Fields.GetByName(name) == nil {
		collection.Fields.Add(&core.JSONField{Name: name})
	}
}
