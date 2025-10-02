package agent

import (
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

type systemDataCache struct {
	sync.RWMutex
	cache map[uint16]*cacheNode
}

type cacheNode struct {
	data       *system.CombinedData
	lastUpdate time.Time
}

// NewSystemDataCache creates a cache keyed by the polling interval in milliseconds.
func NewSystemDataCache() *systemDataCache {
	return &systemDataCache{
		cache: make(map[uint16]*cacheNode),
	}
}

// Get returns cached combined data when the entry is still considered fresh.
func (c *systemDataCache) Get(cacheTimeMs uint16) (stats *system.CombinedData, isCached bool) {
	c.RLock()
	defer c.RUnlock()

	node, ok := c.cache[cacheTimeMs]
	if !ok {
		return &system.CombinedData{}, false
	}
	// allowedSkew := time.Second
	// isFresh := time.Since(node.lastUpdate) < time.Duration(cacheTimeMs)*time.Millisecond-allowedSkew
	// allow a 50% skew of the cache time
	isFresh := time.Since(node.lastUpdate) < time.Duration(cacheTimeMs/2)*time.Millisecond
	return node.data, isFresh
}

// Set stores the latest combined data snapshot for the given interval.
func (c *systemDataCache) Set(data *system.CombinedData, cacheTimeMs uint16) {
	c.Lock()
	defer c.Unlock()

	node, ok := c.cache[cacheTimeMs]
	if !ok {
		node = &cacheNode{}
		c.cache[cacheTimeMs] = node
	}
	node.data = data
	node.lastUpdate = time.Now()
}
