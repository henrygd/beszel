package agent

import (
	"log/slog"
	"sync"
	"time"

	"github.com/distribution/reference"
	"github.com/henrygd/beszel/internal/entities/container"
)

const imageUpdateInterval = time.Hour

type imageUpdateStatus struct {
	available bool
	checkedAt time.Time
}

func normalizedImageReference(image string) string {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return ""
	}
	// Digest-pinned references cannot move to a new version.
	if _, pinned := named.(reference.Digested); pinned {
		return ""
	}
	return reference.TagNameOnly(named).String()
}

// refreshImageUpdates starts at most one background batch. Neither its network
// work nor its completion is part of the container metrics wait group.
func (dm *dockerManager) refreshImageUpdates(containers []*container.ApiInfo, now time.Time) {
	dm.imageUpdatesMutex.Lock()
	defer dm.imageUpdatesMutex.Unlock()
	if dm.imageUpdatesRunning {
		return
	}
	if dm.imageUpdates == nil {
		dm.imageUpdates = make(map[string]*imageUpdateStatus)
	}
	active := make(map[string]struct{}, len(containers))
	pending := make(map[string]*imageUpdateStatus)
	for _, ctr := range containers {
		if len(ctr.Names) > 0 && dm.shouldExcludeContainer(ctr.Names[0][1:]) {
			continue
		}
		key := normalizedImageReference(ctr.Image)
		if key == "" {
			continue
		}
		active[key] = struct{}{}
		entry := dm.imageUpdates[key]
		if entry == nil {
			entry = &imageUpdateStatus{}
			dm.imageUpdates[key] = entry
		}
		if entry.checkedAt.IsZero() || now.Sub(entry.checkedAt) >= imageUpdateInterval {
			pending[key] = entry
		}
	}
	for key := range dm.imageUpdates {
		if _, ok := active[key]; !ok {
			delete(dm.imageUpdates, key)
		}
	}
	if len(pending) == 0 {
		return
	}
	dm.imageUpdatesRunning = true
	go func() {
		// Limit auxiliary requests even on hosts running many different images.
		sem := make(chan struct{}, 2)
		var wg sync.WaitGroup
		for key, entry := range pending {
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				available, err := dm.checkImageUpdate(key)
				if err != nil {
					available = false
					slog.Debug("Image update check failed", "image", key, "err", err)
				}
				dm.imageUpdatesMutex.Lock()
				entry.available = available
				entry.checkedAt = time.Now()
				dm.imageUpdatesMutex.Unlock()
			}()
		}
		wg.Wait()
		dm.imageUpdatesMutex.Lock()
		dm.imageUpdatesRunning = false
		dm.imageUpdatesMutex.Unlock()
	}()
}

func (dm *dockerManager) cachedImageUpdate(image string) bool {
	key := normalizedImageReference(image)
	dm.imageUpdatesMutex.RLock()
	defer dm.imageUpdatesMutex.RUnlock()
	entry := dm.imageUpdates[key]
	return entry != nil && entry.available
}
