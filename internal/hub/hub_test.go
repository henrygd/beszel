//go:build testing
// +build testing

package hub_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/henrygd/beszel/internal/migrations"
	beszelTests "github.com/henrygd/beszel/internal/tests"

	"github.com/pocketbase/pocketbase/core"
	pbTests "github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// marshal to json and return an io.Reader (for use in ApiScenario.Body)
func jsonReader(v any) io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(data)
}

func TestMakeLink(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())

	tests := []struct {
		name     string
		appURL   string
		parts    []string
		expected string
	}{
		{
			name:     "no parts, no trailing slash in AppURL",
			appURL:   "http://localhost:8090",
			parts:    []string{},
			expected: "http://localhost:8090",
		},
		{
			name:     "no parts, with trailing slash in AppURL",
			appURL:   "http://localhost:8090/",
			parts:    []string{},
			expected: "http://localhost:8090", // TrimSuffix should handle the trailing slash
		},
		{
			name:     "one part",
			appURL:   "http://example.com",
			parts:    []string{"one"},
			expected: "http://example.com/one",
		},
		{
			name:     "multiple parts",
			appURL:   "http://example.com",
			parts:    []string{"alpha", "beta", "gamma"},
			expected: "http://example.com/alpha/beta/gamma",
		},
		{
			name:     "parts with spaces needing escaping",
			appURL:   "http://example.com",
			parts:    []string{"path with spaces", "another part"},
			expected: "http://example.com/path%20with%20spaces/another%20part",
		},
		{
			name:     "parts with slashes needing escaping",
			appURL:   "http://example.com",
			parts:    []string{"a/b", "c"},
			expected: "http://example.com/a%2Fb/c", // url.PathEscape escapes '/'
		},
		{
			name:     "AppURL with subpath, no trailing slash",
			appURL:   "http://localhost/sub",
			parts:    []string{"resource"},
			expected: "http://localhost/sub/resource",
		},
		{
			name:     "AppURL with subpath, with trailing slash",
			appURL:   "http://localhost/sub/",
			parts:    []string{"item"},
			expected: "http://localhost/sub/item",
		},
		{
			name:     "empty parts in the middle",
			appURL:   "http://localhost",
			parts:    []string{"first", "", "third"},
			expected: "http://localhost/first/third",
		},
		{
			name:     "leading and trailing empty parts",
			appURL:   "http://localhost",
			parts:    []string{"", "path", ""},
			expected: "http://localhost/path",
		},
		{
			name:     "parts with various special characters",
			appURL:   "https://test.dev/",
			parts:    []string{"p@th?", "key=value&"},
			expected: "https://test.dev/p@th%3F/key=value&",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original app URL and restore it after the test
			originalAppURL := hub.Settings().Meta.AppURL
			hub.Settings().Meta.AppURL = tt.appURL
			defer func() { hub.Settings().Meta.AppURL = originalAppURL }()

			got := hub.MakeLink(tt.parts...)
			assert.Equal(t, tt.expected, got, "MakeLink generated URL does not match expected")
		})
	}
}

func TestGetSSHKey(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())

	// Test Case 1: Key generation (no existing key)
	t.Run("KeyGeneration", func(t *testing.T) {
		tempDir := t.TempDir()

		// Ensure pubKey is initially empty or different to ensure GetSSHKey sets it
		hub.SetPubkey("")

		signer, err := hub.GetSSHKey(tempDir)
		assert.NoError(t, err, "GetSSHKey should not error when generating a new key")
		assert.NotNil(t, signer, "GetSSHKey should return a non-nil signer")

		// Check if private key file was created
		privateKeyPath := filepath.Join(tempDir, "id_ed25519")
		info, err := os.Stat(privateKeyPath)
		assert.NoError(t, err, "Private key file should be created")
		assert.False(t, info.IsDir(), "Private key path should be a file, not a directory")

		// Check if h.pubKey was set
		assert.NotEmpty(t, hub.GetPubkey(), "h.pubKey should be set after key generation")
		assert.True(t, strings.HasPrefix(hub.GetPubkey(), "ssh-ed25519 "), "h.pubKey should start with 'ssh-ed25519 '")

		// Verify the generated private key is parsable
		keyData, err := os.ReadFile(privateKeyPath)
		require.NoError(t, err)
		_, err = ssh.ParsePrivateKey(keyData)
		assert.NoError(t, err, "Generated private key should be parsable by ssh.ParsePrivateKey")
	})

	// Test Case 2: Existing key
	t.Run("ExistingKey", func(t *testing.T) {
		tempDir := t.TempDir()

		// Manually create a valid key pair for the test
		rawPubKey, rawPrivKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err, "Failed to generate raw ed25519 key pair for pre-existing key test")

		// Marshal the private key into OpenSSH PEM format
		pemBlock, err := ssh.MarshalPrivateKey(rawPrivKey, "")
		require.NoError(t, err, "Failed to marshal private key to PEM block for pre-existing key test")

		privateKeyBytes := pem.EncodeToMemory(pemBlock)
		require.NotNil(t, privateKeyBytes, "PEM encoded private key bytes should not be nil")

		privateKeyPath := filepath.Join(tempDir, "id_ed25519")
		err = os.WriteFile(privateKeyPath, privateKeyBytes, 0600)
		require.NoError(t, err, "Failed to write pre-existing private key")

		// Determine the expected public key string
		sshPubKey, err := ssh.NewPublicKey(rawPubKey)
		require.NoError(t, err)
		expectedPubKeyStr := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPubKey)))

		// Reset h.pubKey to ensure it's set by GetSSHKey from the file
		hub.SetPubkey("")

		signer, err := hub.GetSSHKey(tempDir)
		assert.NoError(t, err, "GetSSHKey should not error when reading an existing key")
		assert.NotNil(t, signer, "GetSSHKey should return a non-nil signer for an existing key")

		// Check if h.pubKey was set correctly to the public key from the file
		assert.Equal(t, expectedPubKeyStr, hub.GetPubkey(), "h.pubKey should match the existing public key")

		// Verify the signer's public key matches the original public key
		signerPubKey := signer.PublicKey()
		marshaledSignerPubKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signerPubKey)))
		assert.Equal(t, expectedPubKeyStr, marshaledSignerPubKey, "Signer's public key should match the existing public key")
	})

	// Test Case 3: Error cases
	t.Run("ErrorCases", func(t *testing.T) {
		tests := []struct {
			name       string
			setupFunc  func(dir string) error
			errorCheck func(t *testing.T, err error)
		}{
			{
				name: "CorruptedKey",
				setupFunc: func(dir string) error {
					return os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("this is not a valid SSH key"), 0600)
				},
				errorCheck: func(t *testing.T, err error) {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "ssh: no key found")
				},
			},
			{
				name: "PermissionDenied",
				setupFunc: func(dir string) error {
					// Create the key file
					keyPath := filepath.Join(dir, "id_ed25519")
					if err := os.WriteFile(keyPath, []byte("dummy content"), 0600); err != nil {
						return err
					}
					// Make it read-only (can't be opened for writing in case a new key needs to be written)
					return os.Chmod(keyPath, 0400)
				},
				errorCheck: func(t *testing.T, err error) {
					// On read-only key, the parser will attempt to parse it and fail with "ssh: no key found"
					assert.Error(t, err)
				},
			},
			{
				name: "EmptyFile",
				setupFunc: func(dir string) error {
					// Create an empty file
					return os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte{}, 0600)
				},
				errorCheck: func(t *testing.T, err error) {
					assert.Error(t, err)
					// The error from attempting to parse an empty file
					assert.Contains(t, err.Error(), "ssh: no key found")
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				tempDir := t.TempDir()

				// Setup the test case
				err := tc.setupFunc(tempDir)
				require.NoError(t, err, "Setup failed")

				// Reset h.pubKey before each test case
				hub.SetPubkey("")

				// Attempt to get SSH key
				_, err = hub.GetSSHKey(tempDir)

				// Verify the error
				tc.errorCheck(t, err)

				// Check that pubKey was not set in error cases
				assert.Empty(t, hub.GetPubkey(), "h.pubKey should not be set if there was an error")
			})
		}
	})
}

func TestApiRoutesAuthentication(t *testing.T) {
	hub, _ := beszelTests.NewTestHub(t.TempDir())
	defer hub.Cleanup()

	hub.StartHub()

	// Create test user and get auth token
	user, err := beszelTests.CreateUser(hub, "testuser@example.com", "password123")
	require.NoError(t, err, "Failed to create test user")

	adminUser, err := beszelTests.CreateRecord(hub, "users", map[string]any{
		"email":    "admin@example.com",
		"password": "password123",
		"role":     "admin",
	})
	require.NoError(t, err, "Failed to create admin user")
	adminUserToken, err := adminUser.NewAuthToken()

	// superUser, err := beszelTests.CreateRecord(hub, core.CollectionNameSuperusers, map[string]any{
	// 	"email":    "superuser@example.com",
	// 	"password": "password123",
	// })
	// require.NoError(t, err, "Failed to create superuser")

	userToken, err := user.NewAuthToken()
	require.NoError(t, err, "Failed to create auth token")

	// Create test system for user-alerts endpoints
	system, err := beszelTests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"users": []string{user.Id},
		"host":  "127.0.0.1",
	})
	require.NoError(t, err, "Failed to create test system")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		// Auth Protected Routes - Should require authentication
		{
			Name:            "POST /test-notification - no auth should fail",
			Method:          http.MethodPost,
			URL:             "/api/beszel/test-notification",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
		},
		{
			Name:           "POST /test-notification - with auth should succeed",
			Method:         http.MethodPost,
			URL:            "/api/beszel/test-notification",
			TestAppFactory: testAppFactory,
			Headers: map[string]string{
				"Authorization": userToken,
			},
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"sending message"},
		},
		{
			Name:            "GET /config-yaml - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/config-yaml",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /config-yaml - with user auth should fail",
			Method: http.MethodGet,
			URL:    "/api/beszel/config-yaml",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  403,
			ExpectedContent: []string{"Requires admin"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /config-yaml - with admin auth should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/config-yaml",
			Headers: map[string]string{
				"Authorization": adminUserToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"test-system"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /universal-token - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/universal-token",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /universal-token - with auth should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/universal-token",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"active", "token"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "POST /user-alerts - no auth should fail",
			Method:          http.MethodPost,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"value":   80,
				"min":     10,
				"systems": []string{system.Id},
			}),
		},
		{
			Name:   "POST /user-alerts - with auth should succeed",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"value":   80,
				"min":     10,
				"systems": []string{system.Id},
			}),
		},
		{
			Name:            "DELETE /user-alerts - no auth should fail",
			Method:          http.MethodDelete,
			URL:             "/api/beszel/user-alerts",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{system.Id},
			}),
		},
		{
			Name:   "DELETE /user-alerts - with auth should succeed",
			Method: http.MethodDelete,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"success\":true"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"systems": []string{system.Id},
			}),
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				// Create an alert to delete
				beszelTests.CreateRecord(app, "alerts", map[string]any{
					"name":   "CPU",
					"system": system.Id,
					"user":   user.Id,
					"value":  80,
					"min":    10,
				})
			},
		},

		// Auth Optional Routes - Should work without authentication
		{
			Name:            "GET /getkey - no auth should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /getkey - with auth should also succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/getkey",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"key\":", "\"v\":"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /first-run - no auth should succeed",
			Method:          http.MethodGet,
			URL:             "/api/beszel/first-run",
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"firstRun\":false"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /first-run - with auth should also succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/first-run",
			Headers: map[string]string{
				"Authorization": userToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"firstRun\":false"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /agent-connect - no auth should succeed (websocket upgrade fails but route is accessible)",
			Method:          http.MethodGet,
			URL:             "/api/beszel/agent-connect",
			ExpectedStatus:  400,
			ExpectedContent: []string{},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /test-notification - invalid auth token should fail",
			Method: http.MethodPost,
			URL:    "/api/beszel/test-notification",
			Body: jsonReader(map[string]any{
				"url": "generic://127.0.0.1",
			}),
			Headers: map[string]string{
				"Authorization": "invalid-token",
			},
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "POST /user-alerts - invalid auth token should fail",
			Method: http.MethodPost,
			URL:    "/api/beszel/user-alerts",
			Headers: map[string]string{
				"Authorization": "invalid-token",
			},
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
			Body: jsonReader(map[string]any{
				"name":    "CPU",
				"value":   80,
				"min":     10,
				"systems": []string{system.Id},
			}),
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestFirstUserCreation(t *testing.T) {
	t.Run("CreateUserEndpoint available when no users exist", func(t *testing.T) {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		hub.StartHub()

		testAppFactoryExisting := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenarios := []beszelTests.ApiScenario{
			{
				Name:   "POST /create-user - should be available when no users exist",
				Method: http.MethodPost,
				URL:    "/api/beszel/create-user",
				Body: jsonReader(map[string]any{
					"email":    "firstuser@example.com",
					"password": "password123",
				}),
				ExpectedStatus:  200,
				ExpectedContent: []string{"User created"},
				TestAppFactory:  testAppFactoryExisting,
				BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
					userCount, err := hub.CountRecords("users")
					require.NoError(t, err)
					require.Zero(t, userCount, "Should start with no users")
					superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
					require.NoError(t, err)
					require.EqualValues(t, 1, len(superusers), "Should start with one temporary superuser")
					require.EqualValues(t, migrations.TempAdminEmail, superusers[0].GetString("email"), "Should have created one temporary superuser")
				},
				AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
					userCount, err := hub.CountRecords("users")
					require.NoError(t, err)
					require.EqualValues(t, 1, userCount, "Should have created one user")
					superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
					require.NoError(t, err)
					require.EqualValues(t, 1, len(superusers), "Should have created one superuser")
					require.EqualValues(t, "firstuser@example.com", superusers[0].GetString("email"), "Should have created one superuser")
				},
			},
			{
				Name:   "POST /create-user - should not be available when users exist",
				Method: http.MethodPost,
				URL:    "/api/beszel/create-user",
				Body: jsonReader(map[string]any{
					"email":    "firstuser@example.com",
					"password": "password123",
				}),
				ExpectedStatus:  404,
				ExpectedContent: []string{"wasn't found"},
				TestAppFactory:  testAppFactoryExisting,
			},
		}

		for _, scenario := range scenarios {
			scenario.Test(t)
		}
	})

	t.Run("CreateUserEndpoint not available when USER_EMAIL, USER_PASSWORD are set", func(t *testing.T) {
		os.Setenv("BESZEL_HUB_USER_EMAIL", "me@example.com")
		os.Setenv("BESZEL_HUB_USER_PASSWORD", "password123")
		defer os.Unsetenv("BESZEL_HUB_USER_EMAIL")
		defer os.Unsetenv("BESZEL_HUB_USER_PASSWORD")

		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		hub.StartHub()

		testAppFactory := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenario := beszelTests.ApiScenario{
			Name:            "POST /create-user - should not be available when USER_EMAIL, USER_PASSWORD are set",
			Method:          http.MethodPost,
			URL:             "/api/beszel/create-user",
			ExpectedStatus:  404,
			ExpectedContent: []string{"wasn't found"},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				users, err := hub.FindAllRecords("users")
				require.NoError(t, err)
				require.EqualValues(t, 1, len(users), "Should start with one user")
				require.EqualValues(t, "me@example.com", users[0].GetString("email"), "Should have created one user")
				superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
				require.NoError(t, err)
				require.EqualValues(t, 1, len(superusers), "Should start with one superuser")
				require.EqualValues(t, "me@example.com", superusers[0].GetString("email"), "Should have created one superuser")
			},
			AfterTestFunc: func(t testing.TB, app *pbTests.TestApp, res *http.Response) {
				users, err := hub.FindAllRecords("users")
				require.NoError(t, err)
				require.EqualValues(t, 1, len(users), "Should still have one user")
				require.EqualValues(t, "me@example.com", users[0].GetString("email"), "Should have created one user")
				superusers, err := hub.FindAllRecords(core.CollectionNameSuperusers)
				require.NoError(t, err)
				require.EqualValues(t, 1, len(superusers), "Should still have one superuser")
				require.EqualValues(t, "me@example.com", superusers[0].GetString("email"), "Should have created one superuser")
			},
		}

		scenario.Test(t)
	})
}

func TestCreateUserEndpointAvailability(t *testing.T) {
	t.Run("CreateUserEndpoint available when no users exist", func(t *testing.T) {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		// Ensure no users exist
		userCount, err := hub.CountRecords("users")
		require.NoError(t, err)
		require.Zero(t, userCount, "Should start with no users")

		hub.StartHub()

		testAppFactory := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenario := beszelTests.ApiScenario{
			Name:   "POST /create-user - should be available when no users exist",
			Method: http.MethodPost,
			URL:    "/api/beszel/create-user",
			Body: jsonReader(map[string]any{
				"email":    "firstuser@example.com",
				"password": "password123",
			}),
			ExpectedStatus:  200,
			ExpectedContent: []string{"User created"},
			TestAppFactory:  testAppFactory,
		}

		scenario.Test(t)

		// Verify user was created
		userCount, err = hub.CountRecords("users")
		require.NoError(t, err)
		require.EqualValues(t, 1, userCount, "Should have created one user")
	})

	t.Run("CreateUserEndpoint not available when users exist", func(t *testing.T) {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		defer hub.Cleanup()

		// Create a user first
		_, err := beszelTests.CreateUser(hub, "existing@example.com", "password")
		require.NoError(t, err)

		hub.StartHub()

		testAppFactory := func(t testing.TB) *pbTests.TestApp {
			return hub.TestApp
		}

		scenario := beszelTests.ApiScenario{
			Name:   "POST /create-user - should not be available when users exist",
			Method: http.MethodPost,
			URL:    "/api/beszel/create-user",
			Body: jsonReader(map[string]any{
				"email":    "another@example.com",
				"password": "password123",
			}),
			ExpectedStatus:  404,
			ExpectedContent: []string{"wasn't found"},
			TestAppFactory:  testAppFactory,
		}

		scenario.Test(t)
	})
}

func TestAutoLoginMiddleware(t *testing.T) {
	var hubs []*beszelTests.TestHub

	defer func() {
		defer os.Unsetenv("AUTO_LOGIN")
		for _, hub := range hubs {
			hub.Cleanup()
		}
	}()

	os.Setenv("AUTO_LOGIN", "user@test.com")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		hubs = append(hubs, hub)
		hub.StartHub()
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "GET /getkey - without auto login should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /getkey - with auto login should fail if no matching user",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:            "GET /getkey - with auto login should succeed",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"key\":", "\"v\":"},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.CreateUser(app, "user@test.com", "password123")
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestTrustedHeaderMiddleware(t *testing.T) {
	var hubs []*beszelTests.TestHub

	defer func() {
		defer os.Unsetenv("TRUSTED_AUTH_HEADER")
		for _, hub := range hubs {
			hub.Cleanup()
		}
	}()

	os.Setenv("TRUSTED_AUTH_HEADER", "X-Beszel-Trusted")

	testAppFactory := func(t testing.TB) *pbTests.TestApp {
		hub, _ := beszelTests.NewTestHub(t.TempDir())
		hubs = append(hubs, hub)
		hub.StartHub()
		return hub.TestApp
	}

	scenarios := []beszelTests.ApiScenario{
		{
			Name:            "GET /getkey - without trusted header should fail",
			Method:          http.MethodGet,
			URL:             "/api/beszel/getkey",
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /getkey - with trusted header should fail if no matching user",
			Method: http.MethodGet,
			URL:    "/api/beszel/getkey",
			Headers: map[string]string{
				"X-Beszel-Trusted": "user@test.com",
			},
			ExpectedStatus:  401,
			ExpectedContent: []string{"requires valid"},
			TestAppFactory:  testAppFactory,
		},
		{
			Name:   "GET /getkey - with trusted header should succeed",
			Method: http.MethodGet,
			URL:    "/api/beszel/getkey",
			Headers: map[string]string{
				"X-Beszel-Trusted": "user@test.com",
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{"\"key\":", "\"v\":"},
			TestAppFactory:  testAppFactory,
			BeforeTestFunc: func(t testing.TB, app *pbTests.TestApp, e *core.ServeEvent) {
				beszelTests.CreateUser(app, "user@test.com", "password123")
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
