package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const latencyConfigFile = "latency_config.json"

// LatencyScope controls which systems receive hub latency probes.
type LatencyScope string

const (
	LatencyScopeAll      LatencyScope = "all"
	LatencyScopeSelected LatencyScope = "selected"
)

// LatencyConfig is hub-wide latency probe configuration (Settings → Latency).
type LatencyConfig struct {
	// PingTargets is name=host:port list (newline or comma separated).
	PingTargets string `json:"ping_targets"`
	// Scope is "all" (default) or "selected".
	Scope LatencyScope `json:"scope"`
	// SystemIDs is used when Scope is "selected".
	SystemIDs []string `json:"system_ids"`
}

type latencyConfigStore struct {
	mu   sync.RWMutex
	path string
	cfg  LatencyConfig
}

func newLatencyConfigStore(dataDir string) *latencyConfigStore {
	s := &latencyConfigStore{
		path: filepath.Join(dataDir, latencyConfigFile),
	}
	_ = s.load()
	return s
}

func (s *latencyConfigStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.cfg = LatencyConfig{Scope: LatencyScopeAll}
			return nil
		}
		return err
	}
	var cfg LatencyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	s.cfg = normalizeLatencyConfig(cfg)
	return nil
}

func normalizeLatencyConfig(cfg LatencyConfig) LatencyConfig {
	cfg.PingTargets = strings.TrimSpace(cfg.PingTargets)
	if cfg.Scope != LatencyScopeSelected {
		cfg.Scope = LatencyScopeAll
	}
	if cfg.SystemIDs == nil {
		cfg.SystemIDs = []string{}
	}
	// dedupe system ids
	seen := make(map[string]struct{}, len(cfg.SystemIDs))
	out := make([]string, 0, len(cfg.SystemIDs))
	for _, id := range cfg.SystemIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	cfg.SystemIDs = out
	return cfg
}

func (s *latencyConfigStore) Get() LatencyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *latencyConfigStore) PingTargets() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.PingTargets
}

// AppliesTo reports whether the given system should receive hub probe targets.
// When no targets are configured, returns false (leave agent env alone).
func (s *latencyConfigStore) AppliesTo(systemID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(s.cfg.PingTargets) == "" {
		return false
	}
	if s.cfg.Scope != LatencyScopeSelected {
		return true
	}
	for _, id := range s.cfg.SystemIDs {
		if id == systemID {
			return true
		}
	}
	return false
}

// HasTargets reports whether hub has any configured probe targets.
func (s *latencyConfigStore) HasTargets() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.PingTargets) != ""
}

func (s *latencyConfigStore) Save(cfg LatencyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg = normalizeLatencyConfig(cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}
