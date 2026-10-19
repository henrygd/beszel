//go:build testing

package alerts_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/henrygd/beszel/internal/alerts"
	"github.com/henrygd/beszel/internal/entities/monitor"
	"github.com/henrygd/beszel/internal/entities/system"
	beszelTests "github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alertAPIRouter(t *testing.T, hub *beszelTests.TestHub) http.Handler {
	t.Helper()
	router, err := apis.NewRouter(hub)
	require.NoError(t, err)
	require.NoError(t, hub.OnServe().Trigger(&core.ServeEvent{App: hub, Router: router}))
	mux, err := router.BuildMux()
	require.NoError(t, err)
	return mux
}

func alertAPIRequest(t *testing.T, handler http.Handler, token, method, path string, body any, status int) map[string]any {
	t.Helper()
	req := httptest.NewRequest(method, path, jsonReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, status, res.Code, "%s %s: %s", method, path, res.Body.String())
	var result map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &result))
	return result
}

func TestAlertRecordSystemAccess(t *testing.T) {
	for _, shareAll := range []string{"false", "true"} {
		t.Run("share_all="+shareAll, func(t *testing.T) {
			t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", shareAll)
			hub, err := beszelTests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer hub.Cleanup()
			hub.StartHub()
			user, err := beszelTests.CreateUser(hub, "subscriber@example.com", "password")
			require.NoError(t, err)
			other, err := beszelTests.CreateUser(hub, "owner@example.com", "password")
			require.NoError(t, err)
			token, err := user.NewAuthToken()
			require.NoError(t, err)
			own, err := beszelTests.CreateSystems(hub, 2, user.Id, "paused")
			require.NoError(t, err)
			foreign, err := beszelTests.CreateSystems(hub, 1, other.Id, "paused")
			require.NoError(t, err)
			handler := alertAPIRouter(t, hub)
			const records = "/api/collections/alerts/records"
			// Collection rules reject inaccessible creates as 400 and hide inaccessible updates as 404.
			readStatus, createStatus, updateStatus := 404, 400, 404
			if shareAll == "true" {
				readStatus, createStatus, updateStatus = 200, 200, 200
			}
			alertAPIRequest(t, handler, token, "GET", "/api/collections/systems/records/"+own[0].Id, nil, 200)
			alertAPIRequest(t, handler, token, "GET", "/api/collections/systems/records/"+foreign[0].Id, nil, readStatus)
			collection, err := hub.FindCollectionByNameOrId("alerts")
			require.NoError(t, err)
			for _, name := range collection.Fields.GetByName("name").(*core.SelectField).Values {
				t.Run(name, func(t *testing.T) {
					// Scalar and array representations both pass through PocketBase's relation binding.
					for _, relation := range []any{foreign[0].Id, []string{foreign[0].Id}} {
						result := alertAPIRequest(t, handler, token, "POST", records, map[string]any{
							"name": name, "user": user.Id, "system": relation, "value": 50,
						}, createStatus)
						if id, ok := result["id"].(string); ok {
							record, err := hub.FindRecordById("alerts", id)
							require.NoError(t, err)
							require.NoError(t, hub.Delete(record))
						}
					}
					result := alertAPIRequest(t, handler, token, "POST", records, map[string]any{
						"name": name, "user": user.Id, "system": own[0].Id, "value": 50,
					}, 200)
					id := result["id"].(string)
					defer func() {
						record, err := hub.FindRecordById("alerts", id)
						require.NoError(t, err)
						require.NoError(t, hub.Delete(record))
					}()
					alertAPIRequest(t, handler, token, "PATCH", records+"/"+id, map[string]any{"value": 51}, 200)
					// The system of an existing alert is immutable through the API, even between accessible systems.
					for _, body := range []map[string]any{
						{"system": foreign[0].Id},
						{"system+": foreign[0].Id},
						{"system": []string{own[1].Id}},
					} {
						alertAPIRequest(t, handler, token, "PATCH", records+"/"+id, body, 404)
					}
					saved, err := hub.FindRecordById("alerts", id)
					require.NoError(t, err)
					assert.Equal(t, own[0].Id, saved.GetString("system"))
					// Internal saves can still move an alert, which must remove its previous cache binding.
					saved.Set("system", own[1].Id)
					require.NoError(t, hub.Save(saved))
					cache := hub.GetAlertManager().GetSystemAlertsCache()
					assert.Empty(t, cache.GetSystemAlerts(own[0].Id), "moving an alert must remove its previous cache binding")
					assert.Len(t, cache.GetSystemAlerts(own[1].Id), 1)
				})
			}
			alertAPIRequest(t, handler, token, "POST", records, map[string]any{"name": "CPU", "user": other.Id, "system": own[0].Id}, 400)
			alertAPIRequest(t, handler, token, "POST", records, map[string]any{"name": "CPU", "user": user.Id, "system": "missing00000000"}, 400)
			alertAPIRequest(t, handler, "", "POST", records, map[string]any{"name": "CPU", "user": user.Id, "system": own[0].Id}, 400)
			// An old subscription must still be checked when PATCH omits the relation.
			oldAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
				"name": "CPU", "user": user.Id, "system": foreign[0].Id, "value": 50,
			})
			require.NoError(t, err)
			alertAPIRequest(t, handler, token, "PATCH", records+"/"+oldAlert.Id, map[string]any{"value": 51}, updateStatus)
			require.NoError(t, hub.Delete(oldAlert))
			otherAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
				"name": "CPU", "user": other.Id, "system": foreign[0].Id,
			})
			require.NoError(t, err)
			alertAPIRequest(t, handler, token, "PATCH", records+"/"+otherAlert.Id, map[string]any{"system": own[0].Id}, 404)
			require.NoError(t, hub.Delete(otherAlert))
			// Alerts cannot be handed to another user.
			ownAlert, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
				"name": "CPU", "user": user.Id, "system": own[0].Id,
			})
			require.NoError(t, err)
			alertAPIRequest(t, handler, token, "PATCH", records+"/"+ownAlert.Id, map[string]any{"user": other.Id}, 404)
			ownAlert, err = hub.FindRecordById("alerts", ownAlert.Id)
			require.NoError(t, err)
			assert.Equal(t, user.Id, ownAlert.GetString("user"))
			require.NoError(t, hub.Delete(ownAlert))
			// Readonly users retain their own alert preferences; superusers retain their bypass.
			user.Set("role", "readonly")
			require.NoError(t, hub.Save(user))
			result := alertAPIRequest(t, handler, token, "POST", records, map[string]any{"name": "CPU", "user": user.Id, "system": own[0].Id}, 200)
			alertAPIRequest(t, handler, token, "PATCH", records+"/"+result["id"].(string), map[string]any{"name": "Memory"}, 200)
			alertAPIRequest(t, handler, token, "PATCH", records+"/"+result["id"].(string), map[string]any{"name": "NetworkMonitorLoss"}, 400)
			record, err := hub.FindRecordById("alerts", result["id"].(string))
			require.NoError(t, err)
			require.NoError(t, hub.Delete(record))
			superuser, err := beszelTests.CreateSuperuser(hub, "superuser@example.com", "password123")
			require.NoError(t, err)
			superToken, err := superuser.NewAuthToken()
			require.NoError(t, err)
			result = alertAPIRequest(t, handler, superToken, "POST", records, map[string]any{"name": "CPU", "user": other.Id, "system": foreign[0].Id}, 200)
			record, err = hub.FindRecordById("alerts", result["id"].(string))
			require.NoError(t, err)
			require.NoError(t, hub.Delete(record))
			// Bulk requests continue to skip inaccessible systems and accept accessible ones.
			alertAPIRequest(t, handler, token, "POST", "/api/beszel/user-alerts", map[string]any{
				"name": "CPU", "systems": []string{own[0].Id, foreign[0].Id}, "value": 50,
			}, 200)
			count, err := hub.CountRecords("alerts")
			require.NoError(t, err)
			expectedCount := 1
			if shareAll == "true" {
				expectedCount = 2
			}
			assert.EqualValues(t, expectedCount, count)
		})
	}
}

func TestAlertRecordForeignSystemDelivery(t *testing.T) {
	t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "false")
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	hub.StartHub()
	user, err := beszelTests.CreateUser(hub, "subscriber@example.com", "password")
	require.NoError(t, err)
	owner, err := beszelTests.CreateUser(hub, "owner@example.com", "password")
	require.NoError(t, err)
	for _, recipient := range []*core.Record{user, owner} {
		_, err := beszelTests.CreateRecord(hub, "user_settings", map[string]any{
			"user": recipient.Id, "settings": map[string]any{"emails": []string{recipient.GetString("email")}, "webhooks": []string{}},
		})
		require.NoError(t, err)
	}
	foreign, err := beszelTests.CreateSystems(hub, 1, owner.Id, "paused")
	require.NoError(t, err)
	foreign[0].Set("name", "private-system")
	require.NoError(t, hub.Save(foreign[0]))
	handler := alertAPIRouter(t, hub)
	for _, recipient := range []*core.Record{user, owner} {
		token, err := recipient.NewAuthToken()
		require.NoError(t, err)
		status := 200
		if recipient.Id == user.Id {
			status = 400
		}
		alertAPIRequest(t, handler, token, "POST", "/api/collections/alerts/records", map[string]any{
			"name": "CPU", "user": recipient.Id, "system": foreign[0].Id, "min": 1, "value": 50,
		}, status)
	}
	synctest.Test(t, func(t *testing.T) {
		require.NoError(t, hub.GetAlertManager().HandleSystemAlerts(foreign[0], &system.CombinedData{Info: system.Info{Cpu: 91}}))
		synctest.Wait()
		messages := hub.TestMailer.Messages()
		assert.Len(t, messages, 1, "only the authorised subscriber should receive telemetry")
		for _, message := range messages {
			assert.Equal(t, "owner@example.com", message.To[0].Address)
			assert.Contains(t, message.Subject, "private-system CPU above threshold")
			assert.Contains(t, message.Text, "91.00%")
			t.Logf("captured synthetic email: recipient=%s subject=%q body=%q", message.To[0].Address, message.Subject, message.Text)
		}
		history, err := hub.FindAllRecords("alerts_history", dbx.HashExp{"system": foreign[0].Id})
		require.NoError(t, err)
		assert.Len(t, history, 1)
		for _, record := range history {
			assert.Equal(t, owner.Id, record.GetString("user"))
		}
		t.Logf("captured synthetic history entries=%d", len(history))
	})
}

func TestAlertStoredSubscriptionAccess(t *testing.T) {
	for _, mode := range []string{"foreign", "revoked", "shared", "share_all", "revoked_active"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "false")
			if mode == "share_all" {
				t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "true")
			}
			hub, err := beszelTests.NewTestHub(t.TempDir())
			require.NoError(t, err)
			defer hub.Cleanup()
			hub.StartHub()
			owner, err := beszelTests.CreateUser(hub, "owner@example.com", "password")
			require.NoError(t, err)
			subscriber, err := beszelTests.CreateUser(hub, "subscriber@example.com", "password")
			require.NoError(t, err)
			systems, err := beszelTests.CreateSystems(hub, 1, owner.Id, "paused")
			require.NoError(t, err)
			systemRecord := systems[0]
			if mode == "revoked" || mode == "shared" || mode == "revoked_active" {
				systemRecord.Set("users", []string{owner.Id, subscriber.Id})
				require.NoError(t, hub.Save(systemRecord))
			}
			for _, user := range []*core.Record{owner, subscriber} {
				_, err = beszelTests.CreateRecord(hub, "user_settings", map[string]any{
					"user": user.Id, "settings": map[string]any{"emails": []string{user.GetString("email")}, "webhooks": []string{}},
				})
				require.NoError(t, err)
				// Direct saves model records already stored before the request-hook fix.
				_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
					"name": "CPU", "user": user.Id, "system": systemRecord.Id, "value": 50, "min": 1,
				})
				require.NoError(t, err)
			}
			require.NoError(t, hub.GetAlertManager().GetSystemAlertsCache().PopulateFromDB(true))
			if mode == "revoked" {
				systemRecord.Set("users", []string{owner.Id})
				require.NoError(t, hub.Save(systemRecord))
			}
			synctest.Test(t, func(t *testing.T) {
				require.NoError(t, hub.GetAlertManager().HandleSystemAlerts(systemRecord, &system.CombinedData{Info: system.Info{Cpu: 91}}))
				synctest.Wait()
			})
			allowed := mode == "shared" || mode == "share_all" || mode == "revoked_active"
			want := 1
			if allowed {
				want = 2
			}
			assert.Equal(t, want, hub.TestMailer.TotalSend())
			for _, message := range hub.TestMailer.Messages() {
				if !allowed {
					assert.Equal(t, "owner@example.com", message.To[0].Address)
				}
				t.Logf("%s captured recipient=%s body=%q", mode, message.To[0].Address, message.Text)
			}
			history, err := hub.FindAllRecords("alerts_history")
			require.NoError(t, err)
			assert.Len(t, history, want)
			for _, record := range history {
				if !allowed {
					assert.Equal(t, owner.Id, record.GetString("user"))
				}
			}
			count, err := hub.CountRecords("alerts")
			require.NoError(t, err)
			assert.EqualValues(t, 2, count, "stored subscriptions are preserved")
			if mode == "revoked_active" {
				systemRecord.Set("users", []string{owner.Id})
				require.NoError(t, hub.Save(systemRecord))
				synctest.Test(t, func(t *testing.T) {
					require.NoError(t, hub.GetAlertManager().HandleSystemAlerts(systemRecord, &system.CombinedData{Info: system.Info{Cpu: 40}}))
					synctest.Wait()
				})
				assert.Equal(t, 3, hub.TestMailer.TotalSend(), "only the owner should receive recovery after revocation")
				previous, err := hub.FindFirstRecordByFilter("alerts_history", "user={:user}", dbx.Params{"user": subscriber.Id})
				require.NoError(t, err)
				assert.True(t, previous.GetDateTime("resolved").IsZero(), "revoked subscribers must not learn recovery timing through history")
			}
		})
	}
}

func TestAlertPendingStatusAccessRevoked(t *testing.T) {
	t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "false")
	hub, err := beszelTests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()
	hub.StartHub()
	user, err := beszelTests.CreateUser(hub, "subscriber@example.com", "password")
	require.NoError(t, err)
	owner, err := beszelTests.CreateUser(hub, "owner@example.com", "password")
	require.NoError(t, err)
	_, err = beszelTests.CreateRecord(hub, "user_settings", map[string]any{
		"user": user.Id, "settings": map[string]any{"emails": []string{"subscriber@example.com"}, "webhooks": []string{}},
	})
	require.NoError(t, err)
	systems, err := beszelTests.CreateSystems(hub, 1, user.Id, "paused")
	require.NoError(t, err)
	_, err = beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name": "Status", "user": user.Id, "system": systems[0].Id, "min": 1,
	})
	require.NoError(t, err)
	synctest.Test(t, func(t *testing.T) {
		defer hub.GetAlertManager().Stop()
		require.NoError(t, hub.GetAlertManager().HandleStatusAlerts("down", systems[0]))
		require.Equal(t, 1, hub.GetAlertManager().GetPendingAlertsCount())
		systems[0].Set("users", []string{owner.Id})
		require.NoError(t, hub.Save(systems[0]))
		time.Sleep(61 * time.Second)
		synctest.Wait()
		assert.Zero(t, hub.TestMailer.TotalSend())
		count, err := hub.CountRecords("alerts_history")
		require.NoError(t, err)
		assert.Zero(t, count)
	})
}

func TestAlertStoredNetworkMonitorAccess(t *testing.T) {
	t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "false")
	hub, systemRecord, _, monitors := networkAlertSetup(t)
	other, err := beszelTests.CreateUser(hub, "foreign@example.com", "password")
	require.NoError(t, err)
	foreign, err := beszelTests.CreateRecord(hub, "alerts", map[string]any{
		"name": "NetworkMonitorLoss", "user": other.Id, "system": systemRecord.Id, "value": 5,
	})
	require.NoError(t, err)
	am := alerts.NewTestAlertManagerWithoutWorker(hub)
	require.NoError(t, am.HandleNetworkMonitorAlerts(systemRecord, map[string]monitor.Result{monitors[0].Id: monitorResult(10)}))
	history, err := hub.FindAllRecords("alerts_history")
	require.NoError(t, err)
	require.Len(t, history, 1, "foreign subscriptions must not block the authorised incident transaction")
	assert.NotEqual(t, foreign.Id, history[0].GetString("alert_id"))
	assert.Equal(t, 1, hub.TestMailer.TotalSend())
}
