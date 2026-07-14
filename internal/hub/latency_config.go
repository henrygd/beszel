package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const latencyConfigFile = "latency_config.json"

// LatencyConfig is hub-wide latency probe configuration (Settings → Latency).
type LatencyConfig struct {
	// PingTargets is comma-separated host or host:port list pushed to all agents.
	PingTargets string `json:"ping_targets"`
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
			s.cfg = LatencyConfig{}
			return nil
		}
		return err
	}
	var cfg LatencyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	s.cfg = cfg
	return nil
}

func (s *latencyConfigStore) Get() LatencyConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *latencyConfigStore) PingTargets() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.PingTargets)
}

func (s *latencyConfigStore) Save(cfg LatencyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg.PingTargets = strings.TrimSpace(cfg.PingTargets)
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
