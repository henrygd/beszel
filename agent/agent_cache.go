package agent

import (
	"time"

	"github.com/henrygd/beszel/src/entities/system"
)

// Not thread safe since we only access from gatherStats which is already locked
type SessionCache struct {
	data           *system.CombinedData
	lastUpdate     time.Time
	primarySession string
	leaseTime      time.Duration
}

func NewSessionCache(leaseTime time.Duration) *SessionCache {
	return &SessionCache{
		leaseTime: leaseTime,
		data:      &system.CombinedData{},
	}
}

func (c *SessionCache) Get(sessionID string) (stats *system.CombinedData, isCached bool) {
	if sessionID != c.primarySession && time.Since(c.lastUpdate) < c.leaseTime {
		return c.data, true
	}
	return c.data, false
}

func (c *SessionCache) Set(sessionID string, data *system.CombinedData) {
	if data != nil {
		*c.data = *data
	}
	c.primarySession = sessionID
	c.lastUpdate = time.Now()
}
