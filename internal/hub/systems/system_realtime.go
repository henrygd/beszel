package systems

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/hub/utils"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

type subscriptionInfo struct {
	subscription     string
	connectedClients int
	fetching         bool
}

type realtimeFetch struct {
	systemID     string
	subscription string
	info         *subscriptionInfo
}

// onRealtimeConnectRequest handles client connection events for realtime subscriptions.
// It cleans up existing subscriptions when a client connects.
func (sm *SystemManager) onRealtimeConnectRequest(e *core.RealtimeConnectRequestEvent) error {
	// after e.Next() is the client disconnection
	e.Next()
	subscriptions := e.Client.Subscriptions()
	for k := range subscriptions {
		sm.removeRealtimeSubscription(k, subscriptions[k])
	}
	return nil
}

// onRealtimeSubscribeRequest handles client subscription events for realtime metrics.
// It tracks new subscriptions and unsubscriptions to manage the realtime worker lifecycle.
func (sm *SystemManager) onRealtimeSubscribeRequest(e *core.RealtimeSubscribeRequestEvent) error {
	// Parse with PocketBase's own subscription parser before changing the real
	// client. Reject the entire request if any metrics target is inaccessible.
	requested := subscriptions.NewDefaultClient()
	requested.Subscribe(e.Subscriptions...)
	for topic, options := range requested.Subscriptions() {
		if !strings.HasPrefix(topic, "rt_metrics") {
			continue
		}
		system, err := sm.GetSystem(options.Query["system"])
		if err != nil || !system.HasUser(e.App, e.Auth) {
			return e.NotFoundError("", nil)
		}
	}
	oldSubs := e.Client.Subscriptions()
	// after e.Next() is the result of the subscribe request
	err := e.Next()
	newSubs := e.Client.Subscriptions()

	// handle new subscriptions
	for k, options := range newSubs {
		if _, ok := oldSubs[k]; !ok {
			if strings.HasPrefix(k, "rt_metrics") {
				sm.addRealtimeSubscription(options.Query["system"], k)
			}
		}
	}
	// handle unsubscriptions
	for k := range oldSubs {
		if _, ok := newSubs[k]; !ok {
			sm.removeRealtimeSubscription(k, oldSubs[k])
		}
	}

	return err
}

// addRealtimeSubscription tracks a subscriber and starts a worker if necessary.
func (sm *SystemManager) addRealtimeSubscription(systemID, subscription string) {
	sm.realtimeMutex.Lock()
	defer sm.realtimeMutex.Unlock()

	if sm.activeSubscriptions == nil {
		sm.activeSubscriptions = make(map[string]*subscriptionInfo)
	}
	info, ok := sm.activeSubscriptions[systemID]
	if !ok {
		info = &subscriptionInfo{subscription: subscription}
		sm.activeSubscriptions[systemID] = info
	}
	info.connectedClients++

	if !sm.realtimeWorkerRun {
		sm.realtimeWorkerRun = true
		stop := make(chan struct{})
		sm.realtimeWorkerStop = stop
		go sm.startRealtimeWorker(stop)
	}
}

// stopRealtimeWorker stops the current worker generation, if any.
func (sm *SystemManager) stopRealtimeWorker() {
	sm.realtimeMutex.Lock()
	defer sm.realtimeMutex.Unlock()
	sm.stopRealtimeWorkerLocked()
}

func (sm *SystemManager) stopRealtimeWorkerLocked() {
	if !sm.realtimeWorkerRun {
		return
	}
	close(sm.realtimeWorkerStop)
	sm.realtimeWorkerStop = nil
	sm.realtimeWorkerRun = false
}

// removeRealtimeSubscription removes a realtime subscription and checks if the worker should be stopped.
// It only processes subscriptions with the "rt_metrics" prefix and triggers cleanup when subscriptions are removed.
func (sm *SystemManager) removeRealtimeSubscription(subscription string, options subscriptions.SubscriptionOptions) {
	if strings.HasPrefix(subscription, "rt_metrics") {
		systemID := options.Query["system"]
		sm.realtimeMutex.Lock()
		if info, ok := sm.activeSubscriptions[systemID]; ok {
			info.connectedClients--
			if info.connectedClients <= 0 {
				delete(sm.activeSubscriptions, systemID)
			}
		}
		if len(sm.activeSubscriptions) == 0 {
			sm.stopRealtimeWorkerLocked()
		}
		sm.realtimeMutex.Unlock()
	}
}

// startRealtimeWorker runs the main loop for fetching realtime data from agents.
// It continuously fetches system data and broadcasts it to subscribed clients via WebSocket.
func (sm *SystemManager) startRealtimeWorker(stop <-chan struct{}) {
	sm.fetchRealtimeDataAndNotify()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			sm.fetchRealtimeDataAndNotify()
		}
	}
}

// fetchRealtimeDataAndNotify fetches realtime data for all active subscriptions and notifies the clients.
func (sm *SystemManager) fetchRealtimeDataAndNotify() {
	for _, fetch := range sm.claimRealtimeFetches() {
		system, err := sm.GetSystem(fetch.systemID)
		if err != nil {
			sm.finishRealtimeFetch(fetch)
			continue
		}
		go func(fetch realtimeFetch) {
			defer sm.finishRealtimeFetch(fetch)
			data, err := system.fetchDataFromAgent(common.DataRequestOptions{CacheTimeMs: 1000})
			if err != nil {
				return
			}
			bytes, err := json.Marshal(data)
			if err == nil {
				notify(sm.hub, system, fetch.subscription, bytes)
			}
		}(fetch)
	}
}

// claimRealtimeFetches takes a stable snapshot and marks each selected system as
// in flight. Slow agents are skipped on later ticks until their fetch completes.
func (sm *SystemManager) claimRealtimeFetches() []realtimeFetch {
	sm.realtimeMutex.Lock()
	defer sm.realtimeMutex.Unlock()

	fetches := make([]realtimeFetch, 0, len(sm.activeSubscriptions))
	for systemID, info := range sm.activeSubscriptions {
		if info.fetching {
			continue
		}
		info.fetching = true
		fetches = append(fetches, realtimeFetch{
			systemID:     systemID,
			subscription: info.subscription,
			info:         info,
		})
	}
	return fetches
}

func (sm *SystemManager) finishRealtimeFetch(fetch realtimeFetch) {
	sm.realtimeMutex.Lock()
	defer sm.realtimeMutex.Unlock()
	// A subscription may have been removed and recreated while the old request
	// was running. Only release the exact entry claimed by this request.
	if info := sm.activeSubscriptions[fetch.systemID]; info == fetch.info {
		info.fetching = false
	}
}

// notify broadcasts realtime data to all clients subscribed to a specific subscription.
// Custom topics bypass collection rules, so check current access for every
// recipient, including clients whose authentication or membership was revoked.
func notify(app core.App, system *System, subscription string, data []byte) error {
	shareAll, _ := utils.GetEnv("SHARE_ALL_SYSTEMS")
	members := make(map[string]struct{})
	if shareAll != "true" {
		// Refresh once per broadcast so membership changes take effect on the
		// next update without querying the database for every recipient.
		var recordData struct{ Users string }
		if err := app.DB().NewQuery("SELECT users FROM systems WHERE id={:id}").
			Bind(dbx.Params{"id": system.Id}).One(&recordData); err != nil {
			return err
		}
		var userIDs []string
		if err := json.Unmarshal([]byte(recordData.Users), &userIDs); err != nil {
			return err
		}
		for _, id := range userIDs {
			members[id] = struct{}{}
		}
	}
	message := subscriptions.Message{
		Name: subscription,
		Data: data,
	}
	for _, client := range app.SubscriptionsBroker().Clients() {
		if !client.HasSubscription(subscription) {
			continue
		}
		auth, _ := client.Get(apis.RealtimeClientAuthKey).(*core.Record)
		if auth == nil {
			continue
		}
		if _, member := members[auth.Id]; shareAll != "true" && !member {
			continue
		}
		client.Send(message)
	}
	return nil
}
