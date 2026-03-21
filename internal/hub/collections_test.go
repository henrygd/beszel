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

	const isUserMatchesUser = `@request.auth.id != "" && user = @request.auth.id`

	const isUserInUsers = `@request.auth.id != "" && users.id ?= @request.auth.id`
	const isUserInUsersNotReadonly = `@request.auth.id != "" && users.id ?= @request.auth.id && @request.auth.role != "readonly"`

	const isUserInSystemUsers = `@request.auth.id != "" && system.users.id ?= @request.auth.id`
	const isUserInSystemUsersNotReadonly = `@request.auth.id != "" && system.users.id ?= @request.auth.id && @request.auth.role != "readonly"`

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
	assert.Equal(t, isUserInSystemUsers, *containersCollection.ListRule)
	assert.Nil(t, containersCollection.ViewRule)
	assert.Nil(t, containersCollection.CreateRule)
	assert.Nil(t, containersCollection.UpdateRule)
	assert.Nil(t, containersCollection.DeleteRule)

	// container_stats collection
	containerStatsCollection, err := hub.FindCollectionByNameOrId("container_stats")
	require.NoError(t, err, "Failed to find container_stats collection")
	assert.Equal(t, isUserInSystemUsers, *containerStatsCollection.ListRule)
	assert.Nil(t, containerStatsCollection.ViewRule)
	assert.Nil(t, containerStatsCollection.CreateRule)
	assert.Nil(t, containerStatsCollection.UpdateRule)
	assert.Nil(t, containerStatsCollection.DeleteRule)

	// fingerprints collection
	fingerprintsCollection, err := hub.FindCollectionByNameOrId("fingerprints")
	require.NoError(t, err, "Failed to find fingerprints collection")
	assert.Equal(t, isUserInSystemUsers, *fingerprintsCollection.ListRule)
	assert.Equal(t, isUserInSystemUsers, *fingerprintsCollection.ViewRule)
	assert.Equal(t, isUserInSystemUsersNotReadonly, *fingerprintsCollection.CreateRule)
	assert.Equal(t, isUserInSystemUsersNotReadonly, *fingerprintsCollection.UpdateRule)
	assert.Equal(t, isUserInSystemUsersNotReadonly, *fingerprintsCollection.DeleteRule)

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
	assert.Equal(t, isUserInSystemUsers, *smartDevicesCollection.ListRule)
	assert.Equal(t, isUserInSystemUsers, *smartDevicesCollection.ViewRule)
	assert.Nil(t, smartDevicesCollection.CreateRule)
	assert.Nil(t, smartDevicesCollection.UpdateRule)
	assert.Equal(t, isUserInSystemUsersNotReadonly, *smartDevicesCollection.DeleteRule)

	// system_details collection
	systemDetailsCollection, err := hub.FindCollectionByNameOrId("system_details")
	require.NoError(t, err, "Failed to find system_details collection")
	assert.Equal(t, isUserInSystemUsers, *systemDetailsCollection.ListRule)
	assert.Equal(t, isUserInSystemUsers, *systemDetailsCollection.ViewRule)
	assert.Nil(t, systemDetailsCollection.CreateRule)
	assert.Nil(t, systemDetailsCollection.UpdateRule)
	assert.Nil(t, systemDetailsCollection.DeleteRule)

	// system_stats collection
	systemStatsCollection, err := hub.FindCollectionByNameOrId("system_stats")
	require.NoError(t, err, "Failed to find system_stats collection")
	assert.Equal(t, isUserInSystemUsers, *systemStatsCollection.ListRule)
	assert.Nil(t, systemStatsCollection.ViewRule)
	assert.Nil(t, systemStatsCollection.CreateRule)
	assert.Nil(t, systemStatsCollection.UpdateRule)
	assert.Nil(t, systemStatsCollection.DeleteRule)

	// systemd_services collection
	systemdServicesCollection, err := hub.FindCollectionByNameOrId("systemd_services")
	require.NoError(t, err, "Failed to find systemd_services collection")
	assert.Equal(t, isUserInSystemUsers, *systemdServicesCollection.ListRule)
	assert.Nil(t, systemdServicesCollection.ViewRule)
	assert.Nil(t, systemdServicesCollection.CreateRule)
	assert.Nil(t, systemdServicesCollection.UpdateRule)
	assert.Nil(t, systemdServicesCollection.DeleteRule)

	// systems collection
	systemsCollection, err := hub.FindCollectionByNameOrId("systems")
	require.NoError(t, err, "Failed to find systems collection")
	assert.Equal(t, isUserInUsers, *systemsCollection.ListRule)
	assert.Equal(t, isUserInUsers, *systemsCollection.ViewRule)
	assert.Equal(t, isUserInUsersNotReadonly, *systemsCollection.CreateRule)
	assert.Equal(t, isUserInUsersNotReadonly, *systemsCollection.UpdateRule)
	assert.Equal(t, isUserInUsersNotReadonly, *systemsCollection.DeleteRule)

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

func TestCollectionRulesShareAllSystems(t *testing.T) {
	t.Setenv("SHARE_ALL_SYSTEMS", "true")
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	const isUser = `@request.auth.id != ""`
	const isUserNotReadonly = `@request.auth.id != "" && @request.auth.role != "readonly"`

	const isUserMatchesUser = `@request.auth.id != "" && user = @request.auth.id`

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
	assert.Equal(t, isUser, *containersCollection.ListRule)
	assert.Nil(t, containersCollection.ViewRule)
	assert.Nil(t, containersCollection.CreateRule)
	assert.Nil(t, containersCollection.UpdateRule)
	assert.Nil(t, containersCollection.DeleteRule)

	// container_stats collection
	containerStatsCollection, err := hub.FindCollectionByNameOrId("container_stats")
	require.NoError(t, err, "Failed to find container_stats collection")
	assert.Equal(t, isUser, *containerStatsCollection.ListRule)
	assert.Nil(t, containerStatsCollection.ViewRule)
	assert.Nil(t, containerStatsCollection.CreateRule)
	assert.Nil(t, containerStatsCollection.UpdateRule)
	assert.Nil(t, containerStatsCollection.DeleteRule)

	// fingerprints collection
	fingerprintsCollection, err := hub.FindCollectionByNameOrId("fingerprints")
	require.NoError(t, err, "Failed to find fingerprints collection")
	assert.Equal(t, isUser, *fingerprintsCollection.ListRule)
	assert.Equal(t, isUser, *fingerprintsCollection.ViewRule)
	assert.Equal(t, isUserNotReadonly, *fingerprintsCollection.CreateRule)
	assert.Equal(t, isUserNotReadonly, *fingerprintsCollection.UpdateRule)
	assert.Equal(t, isUserNotReadonly, *fingerprintsCollection.DeleteRule)

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
	assert.Equal(t, isUser, *smartDevicesCollection.ListRule)
	assert.Equal(t, isUser, *smartDevicesCollection.ViewRule)
	assert.Nil(t, smartDevicesCollection.CreateRule)
	assert.Nil(t, smartDevicesCollection.UpdateRule)
	assert.Equal(t, isUserNotReadonly, *smartDevicesCollection.DeleteRule)

	// system_details collection
	systemDetailsCollection, err := hub.FindCollectionByNameOrId("system_details")
	require.NoError(t, err, "Failed to find system_details collection")
	assert.Equal(t, isUser, *systemDetailsCollection.ListRule)
	assert.Equal(t, isUser, *systemDetailsCollection.ViewRule)
	assert.Nil(t, systemDetailsCollection.CreateRule)
	assert.Nil(t, systemDetailsCollection.UpdateRule)
	assert.Nil(t, systemDetailsCollection.DeleteRule)

	// system_stats collection
	systemStatsCollection, err := hub.FindCollectionByNameOrId("system_stats")
	require.NoError(t, err, "Failed to find system_stats collection")
	assert.Equal(t, isUser, *systemStatsCollection.ListRule)
	assert.Nil(t, systemStatsCollection.ViewRule)
	assert.Nil(t, systemStatsCollection.CreateRule)
	assert.Nil(t, systemStatsCollection.UpdateRule)
	assert.Nil(t, systemStatsCollection.DeleteRule)

	// systemd_services collection
	systemdServicesCollection, err := hub.FindCollectionByNameOrId("systemd_services")
	require.NoError(t, err, "Failed to find systemd_services collection")
	assert.Equal(t, isUser, *systemdServicesCollection.ListRule)
	assert.Nil(t, systemdServicesCollection.ViewRule)
	assert.Nil(t, systemdServicesCollection.CreateRule)
	assert.Nil(t, systemdServicesCollection.UpdateRule)
	assert.Nil(t, systemdServicesCollection.DeleteRule)

	// systems collection
	systemsCollection, err := hub.FindCollectionByNameOrId("systems")
	require.NoError(t, err, "Failed to find systems collection")
	assert.Equal(t, isUser, *systemsCollection.ListRule)
	assert.Equal(t, isUser, *systemsCollection.ViewRule)
	assert.Equal(t, isUserNotReadonly, *systemsCollection.CreateRule)
	assert.Equal(t, isUserNotReadonly, *systemsCollection.UpdateRule)
	assert.Equal(t, isUserNotReadonly, *systemsCollection.DeleteRule)

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

	user1, _ := beszelTests.CreateUser(hub, "user1@example.com", "password")
	user1Token, _ := user1.NewAuthToken()

	user2, _ := beszelTests.CreateUser(hub, "user2@example.com", "password")
	// user2Token, _ := user2.NewAuthToken()

	userReadonly, _ := beszelTests.CreateUserWithRole(hub, "userreadonly@example.com", "password", "readonly")
	userReadonlyToken, _ := userReadonly.NewAuthToken()

	userOneSystem, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system1",
		"users": []string{user1.Id},
		"host":  "127.0.0.1",
	})

	sharedSystem, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system2",
		"users": []string{user1.Id, user2.Id},
		"host":  "127.0.0.2",
	})

	userTwoSystem, _ := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "system3",
		"users": []string{user2.Id},
		"host":  "127.0.0.2",
	})

	userRecords, _ := hub.CountRecords("users")
	assert.EqualValues(t, 3, userRecords, "all users should be created")

	systemRecords, _ := hub.CountRecords("systems")
	assert.EqualValues(t, 3, systemRecords, "all systems should be created")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:               "Unauthorized user cannot list systems",
			Method:             http.MethodGet,
			URL:                "/api/collections/systems/records",
			ExpectedStatus:     200, // https://github.com/pocketbase/pocketbase/discussions/1570
			TestAppFactory:     testAppFactory,
			ExpectedContent:    []string{`"items":[]`, `"totalItems":0`},
			NotExpectedContent: []string{userOneSystem.Id, sharedSystem.Id, userTwoSystem.Id},
		},
		{
			Name:               "Unauthorized user cannot delete a system",
			Method:             http.MethodDelete,
			URL:                fmt.Sprintf("/api/collections/systems/records/%s", userOneSystem.Id),
			ExpectedStatus:     404,
			TestAppFactory:     testAppFactory,
			ExpectedContent:    []string{"resource wasn't found"},
			NotExpectedContent: []string{userOneSystem.Id},
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 3, systemsCount, "should have 3 systems before deletion")
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 3, systemsCount, "should still have 3 systems after failed deletion")
			},
		},
		{
			Name:   "User 1 can list their own systems",
			Method: http.MethodGet,
			URL:    "/api/collections/systems/records",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:     200,
			ExpectedContent:    []string{userOneSystem.Id, sharedSystem.Id},
			NotExpectedContent: []string{userTwoSystem.Id},
			TestAppFactory:     testAppFactory,
		},
		{
			Name:   "User 1 cannot list user 2's system",
			Method: http.MethodGet,
			URL:    "/api/collections/systems/records",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:     200,
			ExpectedContent:    []string{userOneSystem.Id, sharedSystem.Id},
			NotExpectedContent: []string{userTwoSystem.Id},
			TestAppFactory:     testAppFactory,
		},
		{
			Name:   "User 1 can see user 2's system if SHARE_ALL_SYSTEMS is enabled",
			Method: http.MethodGet,
			URL:    "/api/collections/systems/records",
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{userOneSystem.Id, sharedSystem.Id, userTwoSystem.Id},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				t.Setenv("SHARE_ALL_SYSTEMS", "true")
				hub.SetCollectionAuthSettings()
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				t.Setenv("SHARE_ALL_SYSTEMS", "")
				hub.SetCollectionAuthSettings()
			},
		},
		{
			Name:   "User 1 can delete their own system",
			Method: http.MethodDelete,
			URL:    fmt.Sprintf("/api/collections/systems/records/%s", userOneSystem.Id),
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus: 204,
			TestAppFactory: testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 3, systemsCount, "should have 3 systems before deletion")
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, systemsCount, "should have 2 systems after deletion")
			},
		},
		{
			Name:   "User 1 cannot delete user 2's system",
			Method: http.MethodDelete,
			URL:    fmt.Sprintf("/api/collections/systems/records/%s", userTwoSystem.Id),
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus:  404,
			TestAppFactory:  testAppFactory,
			ExpectedContent: []string{"resource wasn't found"},
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, systemsCount)
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, systemsCount)
			},
		},
		{
			Name:   "Readonly cannot delete a system even if SHARE_ALL_SYSTEMS is enabled",
			Method: http.MethodDelete,
			URL:    fmt.Sprintf("/api/collections/systems/records/%s", sharedSystem.Id),
			Headers: map[string]string{
				"Authorization": userReadonlyToken,
			},
			ExpectedStatus:  404,
			ExpectedContent: []string{"resource wasn't found"},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				t.Setenv("SHARE_ALL_SYSTEMS", "true")
				hub.SetCollectionAuthSettings()
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, systemsCount)
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				t.Setenv("SHARE_ALL_SYSTEMS", "")
				hub.SetCollectionAuthSettings()
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, systemsCount)
			},
		},
		{
			Name:   "User 1 can delete user 2's system if SHARE_ALL_SYSTEMS is enabled",
			Method: http.MethodDelete,
			URL:    fmt.Sprintf("/api/collections/systems/records/%s", userTwoSystem.Id),
			Headers: map[string]string{
				"Authorization": user1Token,
			},
			ExpectedStatus: 204,
			TestAppFactory: testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				t.Setenv("SHARE_ALL_SYSTEMS", "true")
				hub.SetCollectionAuthSettings()
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 2, systemsCount)
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				t.Setenv("SHARE_ALL_SYSTEMS", "")
				hub.SetCollectionAuthSettings()
				systemsCount, _ := app.CountRecords("systems")
				assert.EqualValues(t, 1, systemsCount)
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
