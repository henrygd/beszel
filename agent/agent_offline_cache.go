package agent

import (
	"sync"

	"github.com/henrygd/beszel/internal/entities/system"
)

type systemOfflineCache struct {
	sync.Mutex
	cache []*system.CombinedData
}

func NewSystemOfflineCache() *systemOfflineCache {
	return &systemOfflineCache{
		cache: make([]*system.CombinedData, 0),
	}
}

// GetAll retrieves all cached combined data and clears the cache.
func (c *systemOfflineCache) GetAll() (result []*system.CombinedData) {
	c.Lock()
	defer c.Unlock()

	copy(result, c.cache)
	c.cache = make([]*system.CombinedData, 0)

	return result
}

// Add appends a new combined data snapshot to the cache.
func (c *systemOfflineCache) Add(data *system.CombinedData) {
	c.Lock()
	defer c.Unlock()

	c.cache = append(c.cache, data)
}
