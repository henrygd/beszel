package agent

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/container"
)

// dockerManagers monitors one or more Docker / Podman engines and presents them to the
// agent as a single source of containers. Each engine keeps its own dockerManager, so
// per-engine state (version checks, CPU / network deltas, image update cache) is never shared.
//
// The first manager is the primary engine. Host-level details (system info, Podman flag)
// are reported from it only.
type dockerManagers struct {
	managers []*dockerManager

	ownersMutex sync.RWMutex
	owners      map[string]*dockerManager // short container id -> engine it was last seen on
}

// newDockerManagers creates a manager for each endpoint in DOCKER_HOST.
// Returns nil if Docker monitoring is disabled.
func newDockerManagers(agent *Agent) *dockerManagers {
	hosts := dockerHostsFromEnv()
	if len(hosts) == 0 {
		return nil
	}
	opts := readDockerOptions()
	dms := &dockerManagers{
		managers: make([]*dockerManager, 0, len(hosts)),
		owners:   make(map[string]*dockerManager),
	}
	for i, host := range hosts {
		// Only the primary engine reports host-level details to the agent.
		var owner *Agent
		if i == 0 {
			owner = agent
		}
		dm := newDockerManager(owner, host, opts)
		if len(hosts) > 1 {
			dm.engine = strings.TrimPrefix(host, "unix://")
		}
		dms.managers = append(dms.managers, dm)
	}
	if len(hosts) > 1 {
		slog.Info("DOCKER_HOST", "endpoints", hosts)
	}
	return dms
}

// dockerHostsFromEnv returns the Docker endpoints to monitor. DOCKER_HOST may hold several
// comma-separated endpoints. If it is unset, the first available default socket is used.
// An empty value disables Docker monitoring and returns nil.
func dockerHostsFromEnv() []string {
	raw, exists := utils.GetEnv("DOCKER_HOST")
	if !exists {
		return []string{getDockerHost()}
	}
	return parseDockerHosts(raw)
}

// parseDockerHosts splits a comma-separated list of endpoints, dropping blanks and duplicates.
func parseDockerHosts(raw string) []string {
	var hosts []string
	seen := make(map[string]struct{})
	for part := range strings.SplitSeq(raw, ",") {
		host := strings.TrimSpace(part)
		if _, dup := seen[host]; host == "" || dup {
			continue
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}
	return hosts
}

// getDockerStats returns stats for the running containers of every engine. An engine that
// fails is skipped so the others keep reporting; an error is returned only if all fail.
func (d *dockerManagers) getDockerStats(cacheTimeMs uint16) ([]*container.Stats, error) {
	if len(d.managers) == 1 {
		return d.managers[0].getDockerStats(cacheTimeMs)
	}

	type result struct {
		stats []*container.Stats
		err   error
	}
	results := make([]result, len(d.managers))
	var wg sync.WaitGroup
	for i, dm := range d.managers {
		wg.Go(func() {
			results[i].stats, results[i].err = dm.getDockerStats(cacheTimeMs)
		})
	}
	wg.Wait()

	var all []*container.Stats
	var errs []error
	owners := make(map[string]*dockerManager)
	for i, res := range results {
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		for _, stats := range res.stats {
			owners[stats.Id] = d.managers[i]
		}
		all = append(all, res.stats...)
	}
	if len(errs) == len(results) {
		return nil, errors.Join(errs...)
	}
	if len(errs) > 0 {
		slog.Debug("Containers", "err", errors.Join(errs...))
	}

	d.ownersMutex.Lock()
	d.owners = owners
	d.ownersMutex.Unlock()
	return all, nil
}

// forContainer runs fn against the engine that owns the container. If the owner is unknown
// (e.g. a stopped container, which has no stats) every engine is tried in order.
func forContainer[T any](d *dockerManagers, id string, fn func(*dockerManager) (T, error)) (T, error) {
	d.ownersMutex.RLock()
	owner := d.owners[id[:min(len(id), 12)]]
	d.ownersMutex.RUnlock()
	if owner != nil {
		return fn(owner)
	}

	var zero T
	var errs []error
	for _, dm := range d.managers {
		res, err := fn(dm)
		if err == nil {
			return res, nil
		}
		errs = append(errs, err)
	}
	return zero, errors.Join(errs...)
}

func (d *dockerManagers) getLogs(ctx context.Context, containerID string) (string, error) {
	return forContainer(d, containerID, func(dm *dockerManager) (string, error) {
		return dm.getLogs(ctx, containerID)
	})
}

func (d *dockerManagers) getContainerInfo(ctx context.Context, containerID string) ([]byte, error) {
	return forContainer(d, containerID, func(dm *dockerManager) ([]byte, error) {
		return dm.getContainerInfo(ctx, containerID)
	})
}

// IsPodman reports whether the primary engine is Podman.
func (d *dockerManagers) IsPodman() bool {
	return d.managers[0].IsPodman()
}

// GetHostInfo returns host info from the primary engine.
func (d *dockerManagers) GetHostInfo() (container.HostInfo, error) {
	return d.managers[0].GetHostInfo()
}
