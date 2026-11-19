//go:build testing

package hub_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/dbutils"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestEmailIndexesAreNocase verifies the migration added COLLATE NOCASE to
// the unique email index of both auth collections.
func TestEmailIndexesAreNocase(t *testing.T) {
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	for _, name := range []string{"users", core.CollectionNameSuperusers} {
		collection, err := hub.FindCollectionByNameOrId(name)
		require.NoError(t, err)
		idx, ok := dbutils.FindSingleColumnUniqueIndex(collection.Indexes, core.FieldNameEmail)
		require.True(t, ok, name)
		assert.Equal(t, "nocase", strings.ToLower(idx.Columns[0].Collate), name)
	}
}

// TestCaseVariantDuplicateEmailRejected verifies the NOCASE unique index
// prevents creating accounts that differ only in email case.
func TestCaseVariantDuplicateEmailRejected(t *testing.T) {
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	_, err = beszelTests.CreateUser(hub, "Mixed@Case.com", "password123")
	require.NoError(t, err)
	_, err = beszelTests.CreateUser(hub, "mixed@CASE.com", "password123")
	require.Error(t, err)

	_, err = beszelTests.CreateSuperuser(hub, "Admin@Case.com", "password123")
	require.NoError(t, err)
	_, err = beszelTests.CreateSuperuser(hub, "ADMIN@case.com", "password123")
	require.Error(t, err)
}

// TestCaseInsensitiveEmailLogin verifies users and superusers can log in
// regardless of the email case used (#1887).
func TestCaseInsensitiveEmailLogin(t *testing.T) {
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	user, err := beszelTests.CreateUser(hub, "Legacy@Example.com", "password123")
	require.NoError(t, err)
	user.SetVerified(true)
	require.NoError(t, hub.Save(user))

	superuser, err := beszelTests.CreateSuperuser(hub, "Admin@Example.com", "password123")
	require.NoError(t, err)

	factory := func(t testing.TB) *pbTests.TestApp { return hub.TestApp }
	userOk := []string{`"token":`, user.Id}
	superuserOk := []string{`"token":`, superuser.Id}
	fail := []string{"Failed to authenticate"}

	scenarios := []beszelTests.ApiScenario{
		authWithPasswordScenario("user lowercase", "users", "legacy@example.com", "password123", 200, userOk, factory),
		authWithPasswordScenario("user uppercase", "users", "LEGACY@EXAMPLE.COM", "password123", 200, userOk, factory),
		authWithPasswordScenario("user original case", "users", "Legacy@Example.com", "password123", 200, userOk, factory),
		authWithPasswordScenario("user wrong password", "users", "legacy@example.com", "wrong", 400, fail, factory),
		authWithPasswordScenario("user unknown email", "users", "nobody@example.com", "password123", 400, fail, factory),
		authWithPasswordScenario("superuser lowercase", "_superusers", "admin@example.com", "password123", 200, superuserOk, factory),
		authWithPasswordScenario("superuser uppercase", "_superusers", "ADMIN@EXAMPLE.COM", "password123", 200, superuserOk, factory),
		authWithPasswordScenario("superuser wrong password", "_superusers", "ADMIN@example.com", "wrong", 400, fail, factory),
	}
	for _, s := range scenarios {
		s.Test(t)
	}
}
