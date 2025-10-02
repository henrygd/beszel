package systems

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

type subscriptionInfo struct {
	subscription     string
	connectedClients uint8
}

var (
	activeSubscriptions = make(map[string]*subscriptionInfo)
	workerRunning       bool
	realtimeTicker      *time.Ticker
	tickerStopChan      chan struct{}
	realtimeMutex       sync.Mutex
)

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
	oldSubs := e.Client.Subscriptions()
	// after e.Next() is the result of the subscribe request
	err := e.Next()
	newSubs := e.Client.Subscriptions()

	// handle new subscriptions
	for k, options := range newSubs {
		if _, ok := oldSubs[k]; !ok {
			if strings.HasPrefix(k, "rt_metrics") {
				systemId := options.Query["system"]
				if _, ok := activeSubscriptions[systemId]; !ok {
					activeSubscriptions[systemId] = &subscriptionInfo{
						subscription: k,
					}
				}
				activeSubscriptions[systemId].connectedClients += 1
				sm.onRealtimeSubscriptionAdded()
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

// onRealtimeSubscriptionAdded initializes or starts the realtime worker when the first subscription is added.
// It ensures only one worker runs at a time and creates the ticker for periodic data fetching.
func (sm *SystemManager) onRealtimeSubscriptionAdded() {
	realtimeMutex.Lock()
	defer realtimeMutex.Unlock()

	// Start the worker if it's not already running
	if !workerRunning {
		workerRunning = true
		// Create a new stop channel for this worker instance
		tickerStopChan = make(chan struct{})
		go sm.startRealtimeWorker()
	}

	// If no ticker exists, create one
	if realtimeTicker == nil {
		realtimeTicker = time.NewTicker(1 * time.Second)
	}
}

// checkSubscriptions stops the realtime worker when there are no active subscriptions.
// This prevents unnecessary resource usage when no clients are listening for realtime data.
func (sm *SystemManager) checkSubscriptions() {
	if !workerRunning || len(activeSubscriptions) > 0 {
		return
	}

	realtimeMutex.Lock()
	defer realtimeMutex.Unlock()

	// Signal the worker to stop
	if tickerStopChan != nil {
		select {
		case tickerStopChan <- struct{}{}:
		default:
		}
	}

	if realtimeTicker != nil {
		realtimeTicker.Stop()
		realtimeTicker = nil
	}

	// Mark worker as stopped (will be reset when next subscription comes in)
	workerRunning = false
}

// removeRealtimeSubscription removes a realtime subscription and checks if the worker should be stopped.
// It only processes subscriptions with the "rt_metrics" prefix and triggers cleanup when subscriptions are removed.
func (sm *SystemManager) removeRealtimeSubscription(subscription string, options subscriptions.SubscriptionOptions) {
	if strings.HasPrefix(subscription, "rt_metrics") {
		systemId := options.Query["system"]
		if info, ok := activeSubscriptions[systemId]; ok {
			info.connectedClients -= 1
			if info.connectedClients <= 0 {
				delete(activeSubscriptions, systemId)
			}
		}
		sm.checkSubscriptions()
	}
}

// startRealtimeWorker runs the main loop for fetching realtime data from agents.
// It continuously fetches system data and broadcasts it to subscribed clients via WebSocket.
func (sm *SystemManager) startRealtimeWorker() {
	sm.fetchRealtimeDataAndNotify()

	for {
		select {
		case <-tickerStopChan:
			return
		case <-realtimeTicker.C:
			// Check if ticker is still valid (might have been stopped)
			if realtimeTicker == nil || len(activeSubscriptions) == 0 {
				return
			}
			// slog.Debug("activeSubscriptions", "count", len(activeSubscriptions))
			sm.fetchRealtimeDataAndNotify()
		}
	}
}

// fetchRealtimeDataAndNotify fetches realtime data for all active subscriptions and notifies the clients.
func (sm *SystemManager) fetchRealtimeDataAndNotify() {
	for systemId, info := range activeSubscriptions {
		system, ok := sm.systems.GetOk(systemId)
		if ok {
			go func() {
				data, err := system.fetchDataFromAgent(common.DataRequestOptions{CacheTimeMs: 1000})
				if err != nil {
					return
				}
				bytes, err := json.Marshal(data)
				if err == nil {
					notify(sm.hub, info.subscription, bytes)
				}
			}()
		}
	}
}

// notify broadcasts realtime data to all clients subscribed to a specific subscription.
// It iterates through all connected clients and sends the data only to those with matching subscriptions.
func notify(app core.App, subscription string, data []byte) error {
	message := subscriptions.Message{
		Name: subscription,
		Data: data,
	}
	for _, client := range app.SubscriptionsBroker().Clients() {
		if !client.HasSubscription(subscription) {
			continue
		}
		client.Send(message)
	}
	return nil
}
