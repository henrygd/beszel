package agent

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
)

const (
	minVolumeInterval = time.Minute
	volumeTimeout     = 5 * time.Minute
)

// Docker disk usage from /system/df. Engines that ignore the `type` filter also
// return images, containers and build cache, which are dropped when decoding.
type systemDfResponse struct {
	Volumes []struct {
		Name      string
		UsageData struct {
			Size int64
		}
	}
}

// volumeManager collects Docker volume sizes off the metrics path. /system/df
// can take many seconds while gatherStats holds the agent lock, so a worker
// refreshes a snapshot in the background and gatherStats reads the last one.
type volumeManager struct {
	client   *http.Client
	interval time.Duration
	mu       sync.RWMutex
	sizes    map[string]float64
}

// newVolumeManager returns nil unless DOCKER_VOLUME_INTERVAL requests collection.
// It shares the Docker transport but needs its own client for the longer timeout.
func newVolumeManager(transport http.RoundTripper) *volumeManager {
	intervalEnv, exists := utils.GetEnv("DOCKER_VOLUME_INTERVAL")
	if !exists {
		return nil
	}
	interval, err := time.ParseDuration(intervalEnv)
	if err != nil {
		slog.Warn("Invalid DOCKER_VOLUME_INTERVAL", "err", err)
		return nil
	}
	if interval <= 0 {
		slog.Warn("Invalid DOCKER_VOLUME_INTERVAL", "duration", interval)
		return nil
	}
	interval = max(interval, minVolumeInterval)
	slog.Info("DOCKER_VOLUME_INTERVAL", "duration", interval)

	return &volumeManager{
		client: &http.Client{
			Timeout:   volumeTimeout,
			Transport: transport,
		},
		interval: interval,
	}
}

func (vm *volumeManager) startWorker() {
	go func() {
		for {
			vm.refresh()
			time.Sleep(vm.interval)
		}
	}()
}

// refresh replaces the snapshot with current volume sizes. Failures keep the
// previous snapshot so the chart doesn't gap out on a transient engine error.
func (vm *volumeManager) refresh() {
	resp, err := vm.client.Get("http://localhost/system/df?type=volume")
	if err != nil {
		slog.Debug("Docker volumes", "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug("Docker volumes", "status", resp.Status)
		return
	}

	var df systemDfResponse
	if err := json.NewDecoder(resp.Body).Decode(&df); err != nil {
		slog.Debug("Docker volumes", "err", err)
		return
	}

	sizes := make(map[string]float64, len(df.Volumes))
	for _, volume := range df.Volumes {
		// Docker reports -1 when it did not compute usage for a volume
		if volume.Name == "" || volume.UsageData.Size <= 0 {
			continue
		}
		sizes[volume.Name] = utils.BytesToGigabytes(uint64(volume.UsageData.Size))
	}
	slog.Debug("Docker volumes", "data", sizes)

	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.sizes = sizes
}

// snapshot returns a copy of the latest volume sizes, or nil if there are none.
func (vm *volumeManager) snapshot() map[string]float64 {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	if len(vm.sizes) == 0 {
		return nil
	}
	sizes := make(map[string]float64, len(vm.sizes))
	for name, size := range vm.sizes {
		sizes[name] = size
	}
	return sizes
}

// volumeSizes returns nil when volume collection is disabled.
func (dm *dockerManager) volumeSizes() map[string]float64 {
	if dm == nil || dm.volumeManager == nil {
		return nil
	}
	return dm.volumeManager.snapshot()
}
