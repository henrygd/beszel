package systems

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/container"
	"github.com/henrygd/beszel/internal/entities/probe"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// realtimePayload wraps system data with optional network probe results for realtime broadcast.
type realtimePayload struct {
	Stats      system.Stats             `json:"stats"`
	Info       system.Info              `json:"info"`
	Containers []*container.Stats       `json:"container"`
	Probes     map[string]probe.Result  `json:"probes,omitempty"`
}

type subscriptionInfo struct {
	subscription     string
	connectedClients uint8
}

var (
	activeSubscriptions = make(map[string]*subscriptionInfo)
	workerRunning       bool
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
// It ensures only one worker runs at a time.
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
	tick := time.Tick(1 * time.Second)

	for {
		select {
		case <-tickerStopChan:
			return
		case <-tick:
			if len(activeSubscriptions) == 0 {
				return
			}
			sm.fetchRealtimeDataAndNotify()
		}
	}
}

// fetchRealtimeDataAndNotify fetches realtime data for all active subscriptions and notifies the clients.
func (sm *SystemManager) fetchRealtimeDataAndNotify() {
	for systemId, info := range activeSubscriptions {
		sys, err := sm.GetSystem(systemId)
		if err != nil {
			continue
		}
		go func() {
			data, err := sys.fetchDataFromAgent(common.DataRequestOptions{CacheTimeMs: 1000})
			if err != nil {
				return
			}
			payload := realtimePayload{
				Stats:      data.Stats,
				Info:       data.Info,
				Containers: data.Containers,
			}
			// Fetch network probe results (lightweight in-memory read on agent)
			if sys.hasEnabledProbes() {
				if probes, err := sys.FetchNetworkProbeResults(); err == nil && len(probes) > 0 {
					payload.Probes = probes
				}
			}
			bytes, err := json.Marshal(payload)
			if err == nil {
				notify(sm.hub, info.subscription, bytes)
			}
		}()
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
