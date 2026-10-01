package systems

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/store"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeAuthorization(t *testing.T) {
	t.Setenv("SHARE_ALL_SYSTEMS", "false")
	t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "")
	app, err := pbtests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)
	_, err = app.DB().NewQuery(`CREATE TABLE IF NOT EXISTS systems (id TEXT PRIMARY KEY, users TEXT)`).Execute()
	require.NoError(t, err)
	_, err = app.DB().NewQuery(`INSERT INTO systems (id, users) VALUES ('target', '["member"]')`).Execute()
	require.NoError(t, err)
	member := core.NewRecord(core.NewAuthCollection("users"))
	member.Id = "member"
	outsider := core.NewRecord(member.Collection())
	outsider.Id = "outsider"
	system := &System{Id: "target"}
	sm := newRealtimeTestManager()
	sm.systems.Set(system.Id, system)
	// Keep the lifecycle bookkeeping active without starting an agent worker.
	sm.realtimeWorkerRun = true
	sm.realtimeWorkerStop = make(chan struct{})
	t.Cleanup(sm.stopRealtimeWorker)
	topic := `rt_metrics?options={"query":{"system":"target"}}`

	for _, tc := range []struct {
		name    string
		auth    *core.Record
		topic   string
		share   bool
		allowed bool
	}{
		{"guest", nil, topic, false, false},
		{"outsider", outsider, topic, false, false},
		{"member", member, topic, false, true},
		{"missing system", member, `rt_metrics`, false, false},
		{"unknown system", member, `rt_metrics?options={"query":{"system":"missing"}}`, false, false},
		{"malformed options", member, `rt_metrics?options=invalid`, false, false},
		{"prefix variant", outsider, `rt_metrics_extra?options={"query":{"system":"target"}}`, false, false},
		{"shared outsider", outsider, topic, true, true},
		{"shared guest", nil, topic, true, false},
		{"other topic", nil, "systems/*", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.share {
				t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "true")
			}
			client := subscriptions.NewDefaultClient()
			client.Subscribe("existing")
			e := &core.RealtimeSubscribeRequestEvent{
				RequestEvent: &core.RequestEvent{App: app, Auth: tc.auth},
				Client:       client, Subscriptions: []string{tc.topic},
			}
			called := false
			h := &hook.Hook[*core.RealtimeSubscribeRequestEvent]{}
			h.BindFunc(sm.onRealtimeSubscribeRequest)
			err := h.Trigger(e, func(e *core.RealtimeSubscribeRequestEvent) error {
				called = true
				client.Unsubscribe()
				client.Subscribe(e.Subscriptions...)
				return nil
			})
			if tc.allowed {
				require.NoError(t, err)
				assert.True(t, called)
			} else {
				require.Error(t, err)
				assert.False(t, called)
				assert.True(t, client.HasSubscription("existing"))
				assert.False(t, client.HasSubscription(tc.topic))
			}
		})
	}

	t.Run("broadcast checks current access", func(t *testing.T) {
		client := subscriptions.NewDefaultClient()
		client.Subscribe(topic)
		app.SubscriptionsBroker().Register(client)
		defer app.SubscriptionsBroker().Unregister(client.Id())
		secondClient := subscriptions.NewDefaultClient()
		secondClient.Subscribe(topic)
		app.SubscriptionsBroker().Register(secondClient)
		defer app.SubscriptionsBroker().Unregister(secondClient.Id())
		check := func(auth *core.Record, allowed bool) {
			t.Helper()
			client.Set(apis.RealtimeClientAuthKey, auth)
			secondClient.Set(apis.RealtimeClientAuthKey, auth)
			done := make(chan struct{})
			go func() {
				notify(app, system, topic, []byte(`{"cpu":42}`))
				close(done)
			}()
			// Even on failure, drain pending sends and join the broadcaster before
			// unregistering clients, which closes their channels.
			defer func() {
				for {
					select {
					case <-client.Channel():
					case <-secondClient.Channel():
					case <-done:
						return
					}
				}
			}()
			var received [2]int
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			for {
				select {
				case msg := <-client.Channel():
					received[0]++
					assert.Equal(t, topic, msg.Name)
				case msg := <-secondClient.Channel():
					received[1]++
					assert.Equal(t, topic, msg.Name)
				case <-done:
					want := [2]int{}
					if allowed {
						want = [2]int{1, 1}
					}
					assert.Equal(t, want, received)
					return
				case <-timer.C:
					t.Fatal("broadcast did not finish")
				}
			}
		}
		check(nil, false)
		check(outsider, false)
		check(member, true)
		_, err := app.DB().NewQuery(`UPDATE systems SET users = '[]'`).Execute()
		require.NoError(t, err)
		check(member, false)
		t.Setenv("BESZEL_HUB_SHARE_ALL_SYSTEMS", "true")
		check(outsider, true)
		check(nil, false)
	})
}

func newRealtimeTestManager() *SystemManager {
	return &SystemManager{
		systems:             store.New(map[string]*System{}),
		activeSubscriptions: make(map[string]*subscriptionInfo),
	}
}

func TestRealtimeFetchesDoNotOverlapPerSystem(t *testing.T) {
	sm := newRealtimeTestManager()
	sm.activeSubscriptions["one"] = &subscriptionInfo{subscription: "rt_metrics_one"}
	sm.activeSubscriptions["two"] = &subscriptionInfo{subscription: "rt_metrics_two"}

	first := sm.claimRealtimeFetches()
	require.Len(t, first, 2)
	assert.Empty(t, sm.claimRealtimeFetches())

	sm.finishRealtimeFetch(first[0])
	next := sm.claimRealtimeFetches()
	require.Len(t, next, 1)
	assert.Equal(t, first[0].systemID, next[0].systemID)

	sm.finishRealtimeFetch(first[1])
	sm.finishRealtimeFetch(next[0])
}

func TestFinishingOldRealtimeFetchDoesNotReleaseReplacement(t *testing.T) {
	sm := newRealtimeTestManager()
	oldInfo := &subscriptionInfo{subscription: "old"}
	sm.activeSubscriptions["system"] = oldInfo

	fetch := sm.claimRealtimeFetches()[0]
	newInfo := &subscriptionInfo{subscription: "new", fetching: true}
	sm.activeSubscriptions["system"] = newInfo

	sm.finishRealtimeFetch(fetch)
	assert.True(t, newInfo.fetching)
}

func TestRealtimeSubscriptionLifecycle(t *testing.T) {
	sm := newRealtimeTestManager()
	options := subscriptions.SubscriptionOptions{Query: map[string]string{"system": "system"}}

	sm.addRealtimeSubscription("system", "rt_metrics")
	sm.addRealtimeSubscription("system", "rt_metrics")

	sm.realtimeMutex.Lock()
	firstStop := sm.realtimeWorkerStop
	assert.True(t, sm.realtimeWorkerRun)
	assert.Equal(t, 2, sm.activeSubscriptions["system"].connectedClients)
	sm.realtimeMutex.Unlock()

	sm.removeRealtimeSubscription("rt_metrics", options)
	sm.realtimeMutex.Lock()
	assert.True(t, sm.realtimeWorkerRun)
	assert.Equal(t, 1, sm.activeSubscriptions["system"].connectedClients)
	sm.realtimeMutex.Unlock()

	sm.removeRealtimeSubscription("rt_metrics", options)
	sm.realtimeMutex.Lock()
	assert.False(t, sm.realtimeWorkerRun)
	assert.Empty(t, sm.activeSubscriptions)
	sm.realtimeMutex.Unlock()
	select {
	case <-firstStop:
	default:
		t.Fatal("worker stop channel was not closed")
	}

	// A later subscription must get a new stop channel owned by its worker.
	sm.addRealtimeSubscription("system", "rt_metrics")
	sm.realtimeMutex.Lock()
	secondStop := sm.realtimeWorkerStop
	assert.NotEqual(t, firstStop, secondStop)
	sm.realtimeMutex.Unlock()
	sm.stopRealtimeWorker()
}
