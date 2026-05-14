//go:build testing

package hub_test

import (
	"fmt"
	"net/http"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectionRulesDefault(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	const isAuthenticated = `@request.auth.id != ""`
	const isAdmin = `@request.auth.id != "" && @request.auth.role = "admin"`
	const isUserMatchesUser = `@request.auth.id != "" && user = @request.auth.id`

	// users collection
	usersCollection, err := hub.FindCollectionByNameOrId("users")
	assert.NoError(t, err, "Failed to find users collection")
	assert.True(t, usersCollection.PasswordAuth.Enabled)
	assert.Equal(t, usersCollection.PasswordAuth.IdentityFields, []string{"email"})
	assert.Nil(t, usersCollection.CreateRule)
	assert.False(t, usersCollection.MFA.Enabled)

	// superusers collection
	superusersCollection, err := hub.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	assert.NoError(t, err, "Failed to find superusers collection")
	assert.True(t, superusersCollection.PasswordAuth.Enabled)
	assert.Equal(t, superusersCollection.PasswordAuth.IdentityFields, []string{"email"})
	assert.Nil(t, superusersCollection.CreateRule)
	assert.False(t, superusersCollection.MFA.Enabled)

	// alerts collection
	alertsCollection, err := hub.FindCollectionByNameOrId("alerts")
	require.NoError(t, err, "Failed to find alerts collection")
	assert.Equal(t, isUserMatchesUser, *alertsCollection.ListRule)
	assert.Nil(t, alertsCollection.ViewRule)
	assert.Equal(t, isUserMatchesUser, *alertsCollection.CreateRule)
	assert.Equal(t, isUserMatchesUser, *alertsCollection.UpdateRule)
	assert.Equal(t, isUserMatchesUser, *alertsCollection.DeleteRule)

	// alerts_history collection
	alertsHistoryCollection, err := hub.FindCollectionByNameOrId("alerts_history")
	require.NoError(t, err, "Failed to find alerts_history collection")
	assert.Equal(t, isUserMatchesUser, *alertsHistoryCollection.ListRule)
	assert.Nil(t, alertsHistoryCollection.ViewRule)
	assert.Nil(t, alertsHistoryCollection.CreateRule)
	assert.Nil(t, alertsHistoryCollection.UpdateRule)
	assert.Equal(t, isUserMatchesUser, *alertsHistoryCollection.DeleteRule)

	// containers collection
	containersCollection, err := hub.FindCollectionByNameOrId("containers")
	require.NoError(t, err, "Failed to find containers collection")
	assert.Equal(t, isAuthenticated, *containersCollection.ListRule)
	assert.Nil(t, containersCollection.ViewRule)
	assert.Nil(t, containersCollection.CreateRule)
	assert.Nil(t, containersCollection.UpdateRule)
	assert.Nil(t, containersCollection.DeleteRule)

	// container_stats collection
	containerStatsCollection, err := hub.FindCollectionByNameOrId("container_stats")
	require.NoError(t, err, "Failed to find container_stats collection")
	assert.Equal(t, isAuthenticated, *containerStatsCollection.ListRule)
	assert.Nil(t, containerStatsCollection.ViewRule)
	assert.Nil(t, containerStatsCollection.CreateRule)
	assert.Nil(t, containerStatsCollection.UpdateRule)
	assert.Nil(t, containerStatsCollection.DeleteRule)

	// fingerprints collection
	fingerprintsCollection, err := hub.FindCollectionByNameOrId("fingerprints")
	require.NoError(t, err, "Failed to find fingerprints collection")
	assert.Equal(t, isAuthenticated, *fingerprintsCollection.ListRule)
	assert.Equal(t, isAuthenticated, *fingerprintsCollection.ViewRule)
	assert.Equal(t, isAdmin, *fingerprintsCollection.CreateRule)
	assert.Equal(t, isAdmin, *fingerprintsCollection.UpdateRule)
	assert.Equal(t, isAdmin, *fingerprintsCollection.DeleteRule)

	// quiet_hours collection
	quietHoursCollection, err := hub.FindCollectionByNameOrId("quiet_hours")
	require.NoError(t, err, "Failed to find quiet_hours collection")
	assert.Equal(t, isUserMatchesUser, *quietHoursCollection.ListRule)
	assert.Equal(t, isUserMatchesUser, *quietHoursCollection.ViewRule)
	assert.Equal(t, isUserMatchesUser, *quietHoursCollection.CreateRule)
	assert.Equal(t, isUserMatchesUser, *quietHoursCollection.UpdateRule)
	assert.Equal(t, isUserMatchesUser, *quietHoursCollection.DeleteRule)

	// smart_devices collection
	smartDevicesCollection, err := hub.FindCollectionByNameOrId("smart_devices")
	require.NoError(t, err, "Failed to find smart_devices collection")
	assert.Equal(t, isAuthenticated, *smartDevicesCollection.ListRule)
	assert.Equal(t, isAuthenticated, *smartDevicesCollection.ViewRule)
	assert.Nil(t, smartDevicesCollection.CreateRule)
	assert.Nil(t, smartDevicesCollection.UpdateRule)
	assert.Equal(t, isAdmin, *smartDevicesCollection.DeleteRule)

	// system_details collection
	systemDetailsCollection, err := hub.FindCollectionByNameOrId("system_details")
	require.NoError(t, err, "Failed to find system_details collection")
	assert.Equal(t, isAuthenticated, *systemDetailsCollection.ListRule)
	assert.Equal(t, isAuthenticated, *systemDetailsCollection.ViewRule)
	assert.Nil(t, systemDetailsCollection.CreateRule)
	assert.Nil(t, systemDetailsCollection.UpdateRule)
	assert.Nil(t, systemDetailsCollection.DeleteRule)

	// system_stats collection
	systemStatsCollection, err := hub.FindCollectionByNameOrId("system_stats")
	require.NoError(t, err, "Failed to find system_stats collection")
	assert.Equal(t, isAuthenticated, *systemStatsCollection.ListRule)
	assert.Nil(t, systemStatsCollection.ViewRule)
	assert.Nil(t, systemStatsCollection.CreateRule)
	assert.Nil(t, systemStatsCollection.UpdateRule)
	assert.Nil(t, systemStatsCollection.DeleteRule)

	// systemd_services collection
	systemdServicesCollection, err := hub.FindCollectionByNameOrId("systemd_services")
	require.NoError(t, err, "Failed to find systemd_services collection")
	assert.Equal(t, isAuthenticated, *systemdServicesCollection.ListRule)
	assert.Nil(t, systemdServicesCollection.ViewRule)
	assert.Nil(t, systemdServicesCollection.CreateRule)
	assert.Nil(t, systemdServicesCollection.UpdateRule)
	assert.Nil(t, systemdServicesCollection.DeleteRule)

	// systems collection
	systemsCollection, err := hub.FindCollectionByNameOrId("systems")
	require.NoError(t, err, "Failed to find systems collection")
	assert.Equal(t, isAuthenticated, *systemsCollection.ListRule)
	assert.Equal(t, isAuthenticated, *systemsCollection.ViewRule)
	assert.Equal(t, isAdmin, *systemsCollection.CreateRule)
	assert.Equal(t, isAdmin, *systemsCollection.UpdateRule)
	assert.Equal(t, isAdmin, *systemsCollection.DeleteRule)

	// universal_tokens collection
	universalTokensCollection, err := hub.FindCollectionByNameOrId("universal_tokens")
	require.NoError(t, err, "Failed to find universal_tokens collection")
	assert.Nil(t, universalTokensCollection.ListRule)
	assert.Nil(t, universalTokensCollection.ViewRule)
	assert.Nil(t, universalTokensCollection.CreateRule)
	assert.Nil(t, universalTokensCollection.UpdateRule)
	assert.Nil(t, universalTokensCollection.DeleteRule)

	// user_settings collection
	userSettingsCollection, err := hub.FindCollectionByNameOrId("user_settings")
	require.NoError(t, err, "Failed to find user_settings collection")
	assert.Equal(t, isUserMatchesUser, *userSettingsCollection.ListRule)
	assert.Nil(t, userSettingsCollection.ViewRule)
	assert.Equal(t, isUserMatchesUser, *userSettingsCollection.CreateRule)
	assert.Equal(t, isUserMatchesUser, *userSettingsCollection.UpdateRule)
	assert.Nil(t, userSettingsCollection.DeleteRule)
}

func TestDisablePasswordAuth(t *testing.T) {
	t.Setenv("DISABLE_PASSWORD_AUTH", "true")
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	usersCollection, err := hub.FindCollectionByNameOrId("users")
	assert.NoError(t, err)
	assert.False(t, usersCollection.PasswordAuth.Enabled)
}

func TestUserCreation(t *testing.T) {
	t.Setenv("USER_CREATION", "true")
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	usersCollection, err := hub.FindCollectionByNameOrId("users")
	assert.NoError(t, err)
	assert.Equal(t, "@request.context = 'oauth2'", *usersCollection.CreateRule)
}

func TestMFAOtp(t *testing.T) {
	t.Setenv("MFA_OTP", "true")
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	usersCollection, err := hub.FindCollectionByNameOrId("users")
	assert.NoError(t, err)
	assert.True(t, usersCollection.OTP.Enabled)
	assert.True(t, usersCollection.MFA.Enabled)

	superusersCollection, err := hub.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	assert.NoError(t, err)
	assert.True(t, superusersCollection.OTP.Enabled)
	assert.True(t, superusersCollection.MFA.Enabled)
}

func TestApiCollectionsAuthRules(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	hub.StartHub()

	adminUser, _ := beszelTests.CreateUserWithRole(hub, "admin@example.com", "password", "admin")
	adminToken, _ := adminUser.NewAuthToken()

	regularUser, _ := beszelTests.CreateUser(hub, "user@example.com", "password")
	regularToken, _ := regularUser.NewAuthToken()

	readonlyUser, _ := beszelTests.CreateUserWithRole(hub, "readonly@example.com", "password", "readonly")
	readonlyToken, _ := readonlyUser.NewAuthToken()

	system1, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{"name": "system1", "host": "127.0.0.1"})
	system2, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{"name": "system2", "host": "127.0.0.2"})

	systemCount, _ := hub.CountRecords("systems")
	assert.EqualValues(t, 2, systemCount)

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:               "Unauthenticated user cannot list systems",
			Method:             http.MethodGet,
			URL:                "/api/collections/systems/records",
			ExpectedStatus:     200,
			TestAppFactory:     testAppFactory,
			ExpectedContent:    []string{`"items":[]`, `"totalItems":0`},
			NotExpectedContent: []string{system1.Id, system2.Id},
		},
		{
			Name:   "Regular user can list all systems",
			Method: http.MethodGet,
			URL:    "/api/collections/systems/records",
			Headers: map[string]string{
				"Authorization": regularToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{system1.Id, system2.Id},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "Readonly user can list all systems",
			Method: http.MethodGet,
			URL:    "/api/collections/systems/records",
			Headers: map[string]string{
				"Authorization": readonlyToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{system1.Id, system2.Id},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:               "Unauthenticated user cannot delete a system",
			Method:             http.MethodDelete,
			URL:                fmt.Sprintf("/api/collections/systems/records/%s", system1.Id),
			ExpectedStatus:     404,
			TestAppFactory:     testAppFactory,
			ExpectedContent:    []string{"resource wasn't found"},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				count, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, count, "system should not be deleted")
			},
		},
		{
			Name:   "Regular user cannot delete a system",
			Method: http.MethodDelete,
			URL:    fmt.Sprintf("/api/collections/systems/records/%s", system1.Id),
			Headers: map[string]string{
				"Authorization": regularToken,
			},
			// PocketBase returns 404 when delete rule fails (doesn't reveal record existence)
			ExpectedStatus:  404,
			TestAppFactory:  testAppFactory,
			ExpectedContent: []string{"resource wasn't found"},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				count, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, count, "system should not be deleted")
			},
		},
		{
			Name:   "Readonly user cannot delete a system",
			Method: http.MethodDelete,
			URL:    fmt.Sprintf("/api/collections/systems/records/%s", system1.Id),
			Headers: map[string]string{
				"Authorization": readonlyToken,
			},
			ExpectedStatus:  404,
			TestAppFactory:  testAppFactory,
			ExpectedContent: []string{"resource wasn't found"},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				count, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, count, "system should not be deleted")
			},
		},
		{
			Name:   "Admin can delete any system",
			Method: http.MethodDelete,
			URL:    fmt.Sprintf("/api/collections/systems/records/%s", system1.Id),
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: testAppFactory,
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				count, _ := app.CountRecords("systems")
				assert.EqualValues(t, 1, count, "system should be deleted")
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
