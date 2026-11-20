package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		if err := patchDomainCollection(app); err != nil {
			return err
		}
		return nil
	}, func(app core.App) error {
		return nil
	})
}

func patchDomainCollection(app core.App) error {
	collection, err := app.FindCollectionByNameOrId("domains")
	if err != nil {
		return err
	}

	addTextField(collection, "registry_domain_id")
	addTextField(collection, "ssl_issuer_country")
	addTextField(collection, "ssl_subject")
	addDateField(collection, "ssl_valid_from")
	addTextField(collection, "ssl_fingerprint")
	addNumberField(collection, "ssl_key_size", true)
	addTextField(collection, "ssl_signature_algo")
	addTextField(collection, "host_region")
	addTextField(collection, "host_city")
	addTextField(collection, "host_org")
	addTextField(collection, "host_as")
	addNumberField(collection, "host_lat", false)
	addNumberField(collection, "host_lon", false)
	addTextField(collection, "registrant_name")
	addTextField(collection, "registrant_org")
	addTextField(collection, "registrant_street")
	addTextField(collection, "registrant_city")
	addTextField(collection, "registrant_state")
	addTextField(collection, "registrant_country")
	addTextField(collection, "registrant_postal")
	addTextField(collection, "abuse_email")
	addTextField(collection, "abuse_phone")
	addTextField(collection, "monitor_type")
	addNumberField(collection, "ssl_alert_days", true)
	addBoolField(collection, "notify_on_expiry")
	addBoolField(collection, "notify_on_ssl_expiry")
	addBoolField(collection, "notify_on_dns_change")
	addBoolField(collection, "notify_on_registrar_change")
	addBoolField(collection, "notify_on_value_change")
	addNumberField(collection, "value_change_threshold", false)
	addBoolField(collection, "quiet_hours_enabled")
	addTextField(collection, "quiet_hours_start")
	addTextField(collection, "quiet_hours_end")

	return app.Save(collection)
}

func addTextField(collection *core.Collection, name string) {
	if collection.Fields.GetByName(name) == nil {
		collection.Fields.Add(&core.TextField{Name: name})
	}
}

func addDateField(collection *core.Collection, name string) {
	if collection.Fields.GetByName(name) == nil {
		collection.Fields.Add(&core.DateField{Name: name})
	}
}

func addBoolField(collection *core.Collection, name string) {
	if collection.Fields.GetByName(name) == nil {
		collection.Fields.Add(&core.BoolField{Name: name})
	}
}

func addNumberField(collection *core.Collection, name string, onlyInt bool) {
	if collection.Fields.GetByName(name) == nil {
		collection.Fields.Add(&core.NumberField{Name: name, OnlyInt: onlyInt})
	}
}
