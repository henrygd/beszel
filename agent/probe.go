package agent

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"sync"
	"time"

	"log/slog"

	"github.com/henrygd/beszel/internal/entities/probe"
)

// ProbeManager manages network probe tasks.
type ProbeManager struct {
	mu         sync.RWMutex
	probes     map[string]*probeTask // key = probe.Config.Key()
	httpClient *http.Client
}

type probeTask struct {
	config  probe.Config
	cancel  chan struct{}
	mu      sync.Mutex
	samples []probeSample
}

type probeSample struct {
	latencyMs float64 // -1 means loss
	timestamp time.Time
}

func newProbeManager() *ProbeManager {
	return &ProbeManager{
		probes:     make(map[string]*probeTask),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SyncProbes replaces all probe tasks with the given configs.
func (pm *ProbeManager) SyncProbes(configs []probe.Config) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Build set of new keys
	newKeys := make(map[string]probe.Config, len(configs))
	for _, cfg := range configs {
		newKeys[cfg.Key()] = cfg
	}

	// Stop removed probes
	for key, task := range pm.probes {
		if _, exists := newKeys[key]; !exists {
			close(task.cancel)
			delete(pm.probes, key)
		}
	}

	// Start new probes (skip existing ones with same key)
	for key, cfg := range newKeys {
		if _, exists := pm.probes[key]; exists {
			continue
		}
		task := &probeTask{
			config:  cfg,
			cancel:  make(chan struct{}),
			samples: make([]probeSample, 0, 64),
		}
		pm.probes[key] = task
		go pm.runProbe(task)
	}
}

// GetResults returns aggregated results for all probes over the last supplied duration in ms.
func (pm *ProbeManager) GetResults(durationMs uint16) map[string]probe.Result {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	results := make(map[string]probe.Result, len(pm.probes))
	cutoff := time.Now().Add(-time.Duration(durationMs) * time.Millisecond)

	for key, task := range pm.probes {
		task.mu.Lock()
		var sum, minMs, maxMs float64
		var count, lossCount int
		minMs = math.MaxFloat64

		for _, s := range task.samples {
			if s.timestamp.Before(cutoff) {
				continue
			}
			count++
			if s.latencyMs < 0 {
				lossCount++
				continue
			}
			sum += s.latencyMs
			if s.latencyMs < minMs {
				minMs = s.latencyMs
			}
			if s.latencyMs > maxMs {
				maxMs = s.latencyMs
			}
		}
		task.mu.Unlock()

		if count == 0 {
			continue
		}

		successCount := count - lossCount
		var avg float64
		if successCount > 0 {
			avg = math.Round(sum/float64(successCount)*100) / 100
		}
		if minMs == math.MaxFloat64 {
			minMs = 0
		}

		results[key] = probe.Result{
			avg,                         // average latency in ms
			math.Round(minMs*100) / 100, // min latency in ms
			math.Round(maxMs*100) / 100, // max latency in ms
			math.Round(float64(lossCount)/float64(count)*10000) / 100, // packet loss percentage
		}
	}

	return results
}

// Stop stops all probe tasks.
func (pm *ProbeManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for key, task := range pm.probes {
		close(task.cancel)
		delete(pm.probes, key)
	}
}

// runProbe executes a single probe task in a loop.
func (pm *ProbeManager) runProbe(task *probeTask) {
	interval := time.Duration(task.config.Interval) * time.Second
	if interval < time.Second {
		interval = 10 * time.Second
	}
	ticker := time.Tick(interval)

	// Run immediately on start
	pm.executeProbe(task)

	for {
		select {
		case <-task.cancel:
			return
		case <-ticker:
			pm.executeProbe(task)
		}
	}
}

func (pm *ProbeManager) executeProbe(task *probeTask) {
	var latencyMs float64

	switch task.config.Protocol {
	case "icmp":
		latencyMs = probeICMP(task.config.Target)
	case "tcp":
		latencyMs = probeTCP(task.config.Target, task.config.Port)
	case "http":
		latencyMs = probeHTTP(pm.httpClient, task.config.Target)
	default:
		slog.Warn("unknown probe protocol", "protocol", task.config.Protocol)
		return
	}

	sample := probeSample{
		latencyMs: latencyMs,
		timestamp: time.Now(),
	}

	task.mu.Lock()
	// Trim old samples beyond 120s to bound memory
	cutoff := time.Now().Add(-120 * time.Second)
	start := 0
	for i := range task.samples {
		if task.samples[i].timestamp.After(cutoff) {
			start = i
			break
		}
		if i == len(task.samples)-1 {
			start = len(task.samples)
		}
	}
	if start > 0 {
		size := copy(task.samples, task.samples[start:])
		task.samples = task.samples[:size]
	}
	task.samples = append(task.samples, sample)
	task.mu.Unlock()
}

// probeTCP measures pure TCP handshake latency (excluding DNS resolution).
// Returns -1 on failure.
func probeTCP(target string, port uint16) float64 {
	// Resolve DNS first, outside the timing window
	ips, err := net.LookupHost(target)
	if err != nil || len(ips) == 0 {
		return -1
	}
	addr := net.JoinHostPort(ips[0], fmt.Sprintf("%d", port))

	// Measure only the TCP handshake
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return -1
	}
	conn.Close()
	return float64(time.Since(start).Microseconds()) / 1000.0
}

// probeHTTP measures HTTP GET request latency. Returns -1 on failure.
func probeHTTP(client *http.Client, url string) float64 {
	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		return -1
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return -1
	}
	return float64(time.Since(start).Microseconds()) / 1000.0
}
