//go:build testing

package hub_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authWithPasswordScenario builds an API scenario for the auth-with-password
// endpoint. Keeps the test tables below compact.
func authWithPasswordScenario(name, collection, identity, password string, status int, content []string, factory func(t testing.TB) *pbTests.TestApp) beszelTests.ApiScenario {
	return beszelTests.ApiScenario{
		Name:            name,
		Method:          http.MethodPost,
		URL:             fmt.Sprintf("/api/collections/%s/auth-with-password", collection),
		Body:            strings.NewReader(fmt.Sprintf(`{"identity":%q,"password":%q}`, identity, password)),
		ExpectedStatus:  status,
		ExpectedContent: content,
		TestAppFactory:  factory,
	}
}

// TestEmailIsNormalizedOnCreate verifies that new user/superuser records have
// their email lowercased on save. This prevents future case-variant duplicates
// and keeps the stored data canonical.
func TestEmailIsNormalizedOnCreate(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	// StartHub binds the event hooks; the returned error is about the TestApp
	// not being a full *pocketbase.PocketBase and is expected here
	_ = hub.StartHub()

	user, err := beszelTests.CreateUser(hub, "Mixed@Case.com", "password123")
	require.NoError(t, err)
	assert.Equal(t, "mixed@case.com", user.Email(), "user email should be stored lowercase")

	superuser, err := beszelTests.CreateSuperuser(hub, "Admin@Case.com", "password123")
	require.NoError(t, err)
	assert.Equal(t, "admin@case.com", superuser.Email(), "superuser email should be stored lowercase")

	// a second user with a different case should collide with the existing
	// normalized record via PocketBase's unique email index
	_, err = beszelTests.CreateUser(hub, "MIXED@case.com", "password123")
	require.Error(t, err, "creating a case-variant duplicate user should fail")
}

// TestCaseInsensitiveEmailLogin verifies that a user can authenticate
// regardless of the case used at login time, including for pre-existing
// records whose email was stored with mixed case before the normalize hook
// existed (the scenario reported in issue #1887).
func TestCaseInsensitiveEmailLogin(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	// create records BEFORE StartHub binds the normalize-on-create hook so
	// the emails are persisted with their original mixed case, simulating
	// accounts created on versions without this fix
	user, err := beszelTests.CreateUser(hub, "Legacy@Example.com", "password123")
	require.NoError(t, err)
	user.SetVerified(true)
	require.NoError(t, hub.Save(user))
	require.Equal(t, "Legacy@Example.com", user.Email(), "pre-existing email should retain original case")

	superuser, err := beszelTests.CreateSuperuser(hub, "Admin@Example.com", "password123")
	require.NoError(t, err)
	require.Equal(t, "Admin@Example.com", superuser.Email(), "pre-existing superuser email should retain original case")

	_ = hub.StartHub()

	factory := func(t testing.TB) *pbTests.TestApp { return hub.TestApp }

	okContent := []string{`"token":`, user.Id}
	superuserOk := []string{`"token":`, superuser.Id}
	failContent := []string{"Failed to authenticate"}

	scenarios := []beszelTests.ApiScenario{
		authWithPasswordScenario("user login with lowercase", "users", "legacy@example.com", "password123", 200, okContent, factory),
		authWithPasswordScenario("user login with uppercase", "users", "LEGACY@EXAMPLE.COM", "password123", 200, okContent, factory),
		authWithPasswordScenario("user login with original case", "users", "Legacy@Example.com", "password123", 200, okContent, factory),
		authWithPasswordScenario("user login with wrong password", "users", "legacy@example.com", "wrong", 400, failContent, factory),
		authWithPasswordScenario("user login with unknown email", "users", "nobody@example.com", "password123", 400, failContent, factory),
		authWithPasswordScenario("superuser login with lowercase", "_superusers", "admin@example.com", "password123", 200, superuserOk, factory),
		authWithPasswordScenario("superuser login with uppercase", "_superusers", "ADMIN@EXAMPLE.COM", "password123", 200, superuserOk, factory),
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
