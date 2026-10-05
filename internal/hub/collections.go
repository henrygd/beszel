package hub

import (
	"log/slog"
	"strings"

	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/pocketbase/pocketbase/core"
)

type collectionRules struct {
	list   *string
	view   *string
	create *string
	update *string
	delete *string
}

// ApplyCollectionAuthSettings applies Beszel's collection auth settings.
// Exported so the rules can be re-synced programmatically (tests, a future
// CLI command) without a full hub bootstrap.
func ApplyCollectionAuthSettings(app core.App) error {
	usersCollection, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	superusersCollection, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}

	// disable email auth if DISABLE_PASSWORD_AUTH env var is set
	disablePasswordAuth, _ := utils.GetEnv("DISABLE_PASSWORD_AUTH")
	// allow oauth user creation if USER_CREATION is set
	userCreation, _ := utils.GetEnv("USER_CREATION")
	// enable mfaOtp mfa if MFA_OTP env var is set
	mfaOtp, _ := utils.GetEnv("MFA_OTP")

	// Auth settings are beszel-managed just like the API rules: compare
	// first and warn before overwriting an out-of-band edit, and skip the
	// save entirely when nothing drifted.
	drifted := compareAuthSettings(usersCollection, superusersCollection, disablePasswordAuth, userCreation, mfaOtp)

	usersCollection.PasswordAuth.Enabled = disablePasswordAuth != "true"
	usersCollection.PasswordAuth.IdentityFields = []string{"email"}
	if userCreation == "true" {
		cr := "@request.context = 'oauth2'"
		usersCollection.CreateRule = &cr
	} else {
		usersCollection.CreateRule = nil
	}

	usersCollection.OTP.Length = 6
	superusersCollection.OTP.Length = 6
	usersCollection.OTP.Enabled = mfaOtp == "true"
	usersCollection.MFA.Enabled = mfaOtp == "true"
	superusersCollection.OTP.Enabled = mfaOtp == "true" || mfaOtp == "superusers"
	superusersCollection.MFA.Enabled = mfaOtp == "true" || mfaOtp == "superusers"

	// Note: an auth-settings save is skipped only for the auth settings -
	// the API rules further down must always run their own drift check.
	if len(drifted) > 0 {
		slog.Warn("collection auth settings drifted from beszel-managed values, overwriting",
			"collections", "users,superusers",
			"settings", strings.Join(drifted, ","))
		if err := app.Save(superusersCollection); err != nil {
			return err
		}
		if err := app.Save(usersCollection); err != nil {
			return err
		}
	}

	// When SHARE_ALL_SYSTEMS is enabled, any authenticated user can read
	// system-scoped data. Write rules continue to block readonly users.
	shareAllSystems, _ := utils.GetEnv("SHARE_ALL_SYSTEMS")

	authenticatedRule := "@request.auth.id != \"\""
	systemsMemberRule := authenticatedRule + " && users.id ?= @request.auth.id"
	systemMemberRule := authenticatedRule + " && system.users.id ?= @request.auth.id"

	systemsReadRule := systemsMemberRule
	systemScopedReadRule := systemMemberRule
	if shareAllSystems == "true" {
		systemsReadRule = authenticatedRule
		systemScopedReadRule = authenticatedRule
	}
	systemsWriteRule := systemsReadRule + " && @request.auth.role != \"readonly\""
	systemScopedWriteRule := systemScopedReadRule + " && @request.auth.role != \"readonly\""

	if err := applyCollectionRules(app, []string{"systems"}, collectionRules{
		list:   &systemsReadRule,
		view:   &systemsReadRule,
		create: &systemsWriteRule,
		update: &systemsWriteRule,
		delete: &systemsWriteRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"containers", "container_stats", "system_stats", "systemd_services", "network_monitor_stats"}, collectionRules{
		list: &systemScopedReadRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"smart_devices"}, collectionRules{
		list:   &systemScopedReadRule,
		view:   &systemScopedReadRule,
		delete: &systemScopedWriteRule,
	}); err != nil {
		return err
	}
	if err := applyCollectionRules(app, []string{"zfs_pools"}, collectionRules{
		list: &systemScopedReadRule,
		view: &systemScopedReadRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"fingerprints"}, collectionRules{
		list:   &systemScopedWriteRule,
		view:   &systemScopedWriteRule,
		create: &systemScopedWriteRule,
		update: &systemScopedWriteRule,
		delete: &systemScopedWriteRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"network_monitors"}, collectionRules{
		list:   &systemScopedReadRule,
		view:   &systemScopedReadRule,
		create: &systemScopedWriteRule,
		update: &systemScopedWriteRule,
		delete: &systemScopedWriteRule,
	}); err != nil {
		return err
	}

	// Alerts belong to their user and may only reference systems the user can access.
	// The user and system of an existing alert cannot be changed through the API.
	// Readonly users can still manage their own alerts, so these build on the read rule.
	alertsOwnerRule := authenticatedRule + " && user = @request.auth.id"
	alertsCreateRule := alertsOwnerRule
	alertsUpdateRule := alertsOwnerRule + " && @request.body.user:changed = false && @request.body.system:changed = false"
	if shareAllSystems != "true" {
		alertsCreateRule += " && system.users.id ?= @request.auth.id"
		alertsUpdateRule += " && system.users.id ?= @request.auth.id"
	}
	if err := applyCollectionRules(app, []string{"alerts"}, collectionRules{
		list:   &alertsOwnerRule,
		create: &alertsCreateRule,
		update: &alertsUpdateRule,
		delete: &alertsOwnerRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"system_details"}, collectionRules{
		list: &systemScopedReadRule,
		view: &systemScopedReadRule,
	}); err != nil {
		return err
	}

	return nil
}

// compareAuthSettings reports which beszel-managed auth settings on the
// users/superusers collections differ from the target state derived from the
// environment. An empty result means the collections are already in sync and
// can be left untouched.
func compareAuthSettings(usersCollection, superusersCollection *core.Collection, disablePasswordAuth, userCreation, mfaOtp string) (drifted []string) {
	expectCreateRule := (*string)(nil)
	if userCreation == "true" {
		cr := "@request.context = 'oauth2'"
		expectCreateRule = &cr
	}
	expectOTPEnabled := mfaOtp == "true"
	expectSuperOTPEnabled := mfaOtp == "true" || mfaOtp == "superusers"

	check := func(name string, current, target any) {
		switch t := target.(type) {
		case bool:
			if current.(bool) != t {
				drifted = append(drifted, name)
			}
		case int:
			if current.(int) != t {
				drifted = append(drifted, name)
			}
		case []string:
			currentSlice := current.([]string)
			if len(currentSlice) != len(t) {
				drifted = append(drifted, name)
				return
			}
			for i := range t {
				if currentSlice[i] != t[i] {
					drifted = append(drifted, name)
					return
				}
			}
		case *string:
			if !ruleEqual(current.(*string), t) {
				drifted = append(drifted, name)
			}
		}
	}

	check("users.PasswordAuth.Enabled", usersCollection.PasswordAuth.Enabled, disablePasswordAuth != "true")
	check("users.PasswordAuth.IdentityFields", usersCollection.PasswordAuth.IdentityFields, []string{"email"})
	check("users.CreateRule", usersCollection.CreateRule, expectCreateRule)
	check("users.OTP.Length", usersCollection.OTP.Length, 6)
	check("users.OTP.Enabled", usersCollection.OTP.Enabled, expectOTPEnabled)
	check("users.MFA.Enabled", usersCollection.MFA.Enabled, expectOTPEnabled)
	check("superusers.OTP.Length", superusersCollection.OTP.Length, 6)
	check("superusers.OTP.Enabled", superusersCollection.OTP.Enabled, expectSuperOTPEnabled)
	check("superusers.MFA.Enabled", superusersCollection.MFA.Enabled, expectSuperOTPEnabled)
	return drifted
}

// ruleEqual compares rule pointers. nil and "" are intentionally NOT
// equivalent: in PocketBase a nil rule means superusers-only while an empty
// rule means public, so treating them as equal would silently preserve a
// security-relevant drift.
func ruleEqual(a, b *string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}

// compareCollectionRules reports whether the collection's rules differ from
// the beszel-managed targets, and which slots were customized (a non-empty
// current value that differs) so overwriting them is warned about. The
// migrated snapshot state (empty rules) is applied silently; only genuine
// out-of-band edits produce a warning.
func compareCollectionRules(collection *core.Collection, rules collectionRules) (changed bool, drifted []string) {
	slots := []struct {
		name    string
		current *string
		target  *string
	}{
		{"listRule", collection.ListRule, rules.list},
		{"viewRule", collection.ViewRule, rules.view},
		{"createRule", collection.CreateRule, rules.create},
		{"updateRule", collection.UpdateRule, rules.update},
		{"deleteRule", collection.DeleteRule, rules.delete},
	}
	for _, slot := range slots {
		if ruleEqual(slot.current, slot.target) {
			continue
		}
		changed = true
		if slot.current != nil {
			drifted = append(drifted, slot.name)
		}
	}
	return changed, drifted
}

// applyCollectionRules sets the API rules of the named collections to their
// beszel-managed values. Unchanged collections are not rewritten; a
// collection whose rules were modified outside beszel is overwritten with a
// warning, so rule loss is visible in the logs instead of silent.
func applyCollectionRules(app core.App, collectionNames []string, rules collectionRules) error {
	for _, collectionName := range collectionNames {
		collection, err := app.FindCollectionByNameOrId(collectionName)
		if err != nil {
			return err
		}
		changed, drifted := compareCollectionRules(collection, rules)
		if !changed {
			continue
		}
		if len(drifted) > 0 {
			slog.Warn("collection rules drifted from beszel-managed values, overwriting",
				"collection", collectionName,
				"rules", strings.Join(drifted, ","))
		}
		collection.ListRule = rules.list
		collection.ViewRule = rules.view
		collection.CreateRule = rules.create
		collection.UpdateRule = rules.update
		collection.DeleteRule = rules.delete
		if err := app.Save(collection); err != nil {
			return err
		}
	}
	return nil
}
