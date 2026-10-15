//go:build testing

package users_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/auth"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

type roleTestProvider struct {
	auth.BaseProvider
}

func (p *roleTestProvider) FetchToken(string, ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: "test-token"}, nil
}

func (p *roleTestProvider) FetchAuthUser(*oauth2.Token) (*auth.AuthUser, error) {
	return &auth.AuthUser{Id: "role-test-user", Email: "oauth@example.com"}, nil
}

func TestOAuthUserRole(t *testing.T) {
	t.Setenv("USER_CREATION", "true")
	const provider = "beszel-role-test"
	auth.Providers[provider] = func() auth.Provider { return &roleTestProvider{} }
	t.Cleanup(func() { delete(auth.Providers, provider) })

	for _, tc := range []struct {
		createData, defaultRole, want string
	}{
		{`{}`, "", "user"},
		{`{"role":"admin"}`, "", "user"},
		{`{"role":"readonly"}`, "", "user"},
		{`{}`, "readonly", "readonly"},
		{`{"role":"admin"}`, "readonly", "readonly"},
		{`{}`, "user", "user"},
		// admin and unknown values must never be granted through self-registration
		{`{}`, "admin", "user"},
		{`{"role":"readonly"}`, "bogus", "user"},
	} {
		t.Run(tc.createData+"/default="+tc.defaultRole, func(t *testing.T) {
			t.Setenv("OAUTH_DEFAULT_USER_ROLE", tc.defaultRole)
			createData := tc.createData
			h, err := beszelTests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer h.Cleanup()
			h.StartHub()

			collection, err := h.FindCollectionByNameOrId("users")
			require.NoError(t, err)
			collection.OAuth2.Enabled = true
			collection.OAuth2.Providers = []core.OAuth2ProviderConfig{{
				Name: provider, ClientId: "test-client", ClientSecret: "test-secret",
			}}
			require.NoError(t, h.Save(collection))
			r, err := apis.NewRouter(h.App)
			require.NoError(t, err)
			mux, err := r.BuildMux()
			require.NoError(t, err)
			login := func() {
				body := `{"provider":"` + provider + `","code":"test-code","codeVerifier":"test-verifier","redirectUrl":"http://localhost/callback","createData":` + createData + `}`
				req := httptest.NewRequest("POST", "/api/collections/users/auth-with-oauth2", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				res := httptest.NewRecorder()
				mux.ServeHTTP(res, req)
				require.Equal(t, 200, res.Code, res.Body.String())
			}
			login()
			user, err := h.FindAuthRecordByEmail("users", "oauth@example.com")
			require.NoError(t, err)
			require.Equal(t, tc.want, user.GetString("role"))

			// A later OAuth login must preserve a role assigned by an administrator.
			user.Set("role", "admin")
			require.NoError(t, h.Save(user))
			login()
			user, err = h.FindRecordById("users", user.Id)
			require.NoError(t, err)
			require.Equal(t, "admin", user.GetString("role"))
		})
	}
}

func TestEnvBootstrapUserIsAdmin(t *testing.T) {
	t.Setenv("USER_EMAIL", "bootstrap@example.com")
	t.Setenv("USER_PASSWORD", "password12345")
	t.Setenv("OAUTH_DEFAULT_USER_ROLE", "readonly")
	h, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer h.Cleanup()
	h.StartHub()

	user, err := h.FindAuthRecordByEmail("users", "bootstrap@example.com")
	require.NoError(t, err)
	require.Equal(t, "admin", user.GetString("role"))
}

func TestInternalUserRole(t *testing.T) {
	// OAUTH_DEFAULT_USER_ROLE must not affect users created outside OAuth.
	t.Setenv("OAUTH_DEFAULT_USER_ROLE", "readonly")
	for _, role := range []string{"", "user", "admin", "readonly"} {
		t.Run("role="+role, func(t *testing.T) {
			h, err := beszelTests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer h.Cleanup()
			h.StartHub()
			collection, err := h.FindCollectionByNameOrId("users")
			require.NoError(t, err)
			user := core.NewRecord(collection)
			user.SetEmail("internal@example.com")
			user.SetPassword("password12345")
			user.Set("role", role)
			require.NoError(t, h.Save(user))
			user, err = h.FindRecordById("users", user.Id)
			require.NoError(t, err)
			if role == "" {
				role = "user"
			}
			require.Equal(t, role, user.GetString("role"))
		})
	}
}
