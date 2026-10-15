// Package utils provides utility functions for the hub.
package utils

import (
	"os"

	"github.com/pocketbase/pocketbase/core"
)

// GetEnv retrieves an environment variable with a "BESZEL_HUB_" prefix, or falls back to the unprefixed key.
func GetEnv(key string) (value string, exists bool) {
	if value, exists = os.LookupEnv("BESZEL_HUB_" + key); exists {
		return value, exists
	}
	return os.LookupEnv(key)
}

// realtimeActiveForCollection checks if there are active WebSocket subscriptions for the given collection.
func RealtimeActiveForCollection(app core.App, collectionName string, validateFn func(filterQuery string) bool) bool {
	broker := app.SubscriptionsBroker()
	if broker.TotalClients() == 0 {
		return false
	}
	for _, client := range broker.Clients() {
		subs := client.Subscriptions(collectionName)
		if len(subs) > 0 {
			if validateFn == nil {
				return true
			}
			for k := range subs {
				filter := subs[k].Query["filter"]
				if validateFn(filter) {
					return true
				}
			}
		}
	}
	return false
}
