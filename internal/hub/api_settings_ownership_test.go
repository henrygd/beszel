//go:build testing

package hub_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/stretchr/testify/require"
)

func TestSettingsOwnershipUpdates(t *testing.T) {
	for _, shareAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("share_all_systems=%t", shareAll), func(t *testing.T) {
			t.Setenv("SHARE_ALL_SYSTEMS", fmt.Sprint(shareAll))
			hub, handler := firstUserTestMux(t)
			defer hub.Cleanup()
			a, err := beszelTests.CreateUser(hub, "owner@example.invalid", "password123")
			require.NoError(t, err)
			b, err := beszelTests.CreateUser(hub, "other@example.invalid", "password123")
			require.NoError(t, err)
			empty, err := beszelTests.CreateUser(hub, "empty@example.invalid", "password123")
			require.NoError(t, err)
			auth, err := a.NewAuthToken()
			require.NoError(t, err)
			superuser, err := beszelTests.CreateSuperuser(hub, "maintenance@example.invalid", "password123")
			require.NoError(t, err)
			superAuth, err := superuser.NewAuthToken()
			require.NoError(t, err)
			now := time.Now().UTC()

			type testCase struct {
				collection string
				field      string
				original   string
				target     string
				create     map[string]any
				edit       map[string]any
			}
			systems, err := beszelTests.CreateSystems(hub, 1, a.Id, "paused")
			require.NoError(t, err)
			targets, err := beszelTests.CreateSystems(hub, 1, b.Id, "paused")
			require.NoError(t, err)
			// The settings recipient has no settings record, and the target system has
			// no fingerprint, so uniqueness constraints cannot mask an ownership bypass.
			_, err = beszelTests.CreateRecord(hub, "quiet_hours", map[string]any{
				"user": b.Id, "type": "one-time",
				"start": now.Add(time.Hour).Format(time.RFC3339), "end": now.Add(2 * time.Hour).Format(time.RFC3339),
			})
			require.NoError(t, err)
			cases := []testCase{
				{
					collection: "user_settings", field: "user", original: a.Id, target: empty.Id,
					create: map[string]any{"user": a.Id, "settings": map[string]any{"chartTime": "1h"}},
					edit:   map[string]any{"settings": map[string]any{"chartTime": "12h"}},
				},
				{
					collection: "quiet_hours", field: "user", original: a.Id, target: b.Id,
					create: map[string]any{
						"user": a.Id, "type": "one-time", "start": now.Add(-time.Minute).Format(time.RFC3339),
						"end": now.Add(time.Hour).Format(time.RFC3339),
					},
					edit: map[string]any{"end": now.Add(3 * time.Hour).Format(time.RFC3339)},
				},
				{
					collection: "fingerprints", field: "system", original: systems[0].Id, target: targets[0].Id,
					create: map[string]any{"system": systems[0].Id, "token": "initial-dummy-token", "fingerprint": "dummy-fingerprint"},
					edit:   map[string]any{"token": "rotated-dummy-token", "fingerprint": ""},
				},
			}
			for _, tc := range cases {
				t.Run(tc.collection, func(t *testing.T) {
					record, err := beszelTests.CreateRecord(hub, tc.collection, tc.create)
					require.NoError(t, err)
					path := "/api/collections/" + tc.collection + "/records/" + record.Id
					patch := func(body map[string]any, token string) *httptest.ResponseRecorder {
						req := httptest.NewRequest(http.MethodPatch, path, jsonReader(body))
						req.Header.Set("Authorization", token)
						req.Header.Set("Content-Type", "application/json")
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, req)
						return response
					}
					t.Run("normal edit with unchanged relationship", func(t *testing.T) {
						body := map[string]any{tc.field: tc.original}
						for key, value := range tc.edit {
							body[key] = value
						}
						response := patch(body, auth)
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					})
					t.Run("normal edit without relationship", func(t *testing.T) {
						response := patch(tc.edit, auth)
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					})
					before, err := hub.FindRecordById(tc.collection, record.Id)
					require.NoError(t, err)
					for _, modification := range []struct {
						name string
						body map[string]any
					}{
						{"reassign", map[string]any{tc.field: tc.target}},
						{"clear", map[string]any{tc.field: ""}},
						{"append relation", map[string]any{tc.field + "+": tc.target}},
						{"remove relation", map[string]any{tc.field + "-": tc.original}},
					} {
						t.Run(modification.name, func(t *testing.T) {
							response := patch(modification.body, auth)
							require.Contains(t, []int{400, 403, 404}, response.Code, response.Body.String())
							after, err := hub.FindRecordById(tc.collection, record.Id)
							require.NoError(t, err)
							require.Equal(t, tc.original, after.GetString(tc.field))
							for field := range tc.edit {
								require.Equal(t, before.Get(field), after.Get(field))
							}
						})
					}
					t.Run("batch reassignment", func(t *testing.T) {
						req := httptest.NewRequest(http.MethodPost, "/api/batch", jsonReader(map[string]any{
							"requests": []map[string]any{{"method": "PATCH", "url": path, "body": map[string]any{tc.field: tc.target}}},
						}))
						req.Header.Set("Authorization", auth)
						req.Header.Set("Content-Type", "application/json")
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, req)
						require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
						after, err := hub.FindRecordById(tc.collection, record.Id)
						require.NoError(t, err)
						require.Equal(t, tc.original, after.GetString(tc.field))
					})
					if tc.collection == "quiet_hours" {
						t.Run("recipient notification state remains unchanged", func(t *testing.T) {
							require.False(t, hub.IsNotificationSilenced(b.Id, ""))
							response := patch(map[string]any{"user": b.Id}, auth)
							require.Equal(t, http.StatusNotFound, response.Code)
							require.False(t, hub.IsNotificationSilenced(b.Id, ""))
						})
					}
					t.Run("superuser maintenance", func(t *testing.T) {
						response := patch(map[string]any{tc.field: tc.target}, superAuth)
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						var data map[string]any
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &data))
						require.Equal(t, tc.target, data[tc.field])
					})
				})
			}
		})
	}
}
