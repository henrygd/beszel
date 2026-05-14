package hub

import (
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

// setCollectionAuthSettings applies Beszel's collection auth settings.
func setCollectionAuthSettings(app core.App) error {
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
	usersCollection.PasswordAuth.Enabled = disablePasswordAuth != "true"
	usersCollection.PasswordAuth.IdentityFields = []string{"email"}
	// allow oauth user creation if USER_CREATION is set
	if userCreation, _ := utils.GetEnv("USER_CREATION"); userCreation == "true" {
		cr := "@request.context = 'oauth2'"
		usersCollection.CreateRule = &cr
	} else {
		usersCollection.CreateRule = nil
	}

	// enable mfaOtp mfa if MFA_OTP env var is set
	mfaOtp, _ := utils.GetEnv("MFA_OTP")
	usersCollection.OTP.Length = 6
	superusersCollection.OTP.Length = 6
	usersCollection.OTP.Enabled = mfaOtp == "true"
	usersCollection.MFA.Enabled = mfaOtp == "true"
	superusersCollection.OTP.Enabled = mfaOtp == "true" || mfaOtp == "superusers"
	superusersCollection.MFA.Enabled = mfaOtp == "true" || mfaOtp == "superusers"
	if err := app.Save(superusersCollection); err != nil {
		return err
	}
	if err := app.Save(usersCollection); err != nil {
		return err
	}

	// All authenticated users can view all systems and system-scoped data.
	// Only admin users can modify systems.
	authenticatedRule := "@request.auth.id != \"\""
	adminRule := authenticatedRule + " && @request.auth.role = \"admin\""

	if err := applyCollectionRules(app, []string{"systems"}, collectionRules{
		list:   &authenticatedRule,
		view:   &authenticatedRule,
		create: &adminRule,
		update: &adminRule,
		delete: &adminRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"containers", "container_stats", "system_stats", "systemd_services"}, collectionRules{
		list: &authenticatedRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"smart_devices"}, collectionRules{
		list:   &authenticatedRule,
		view:   &authenticatedRule,
		delete: &adminRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"fingerprints"}, collectionRules{
		list:   &authenticatedRule,
		view:   &authenticatedRule,
		create: &adminRule,
		update: &adminRule,
		delete: &adminRule,
	}); err != nil {
		return err
	}

	if err := applyCollectionRules(app, []string{"system_details"}, collectionRules{
		list: &authenticatedRule,
		view: &authenticatedRule,
	}); err != nil {
		return err
	}

	return nil
}

func applyCollectionRules(app core.App, collectionNames []string, rules collectionRules) error {
	for _, collectionName := range collectionNames {
		collection, err := app.FindCollectionByNameOrId(collectionName)
		if err != nil {
			return err
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
