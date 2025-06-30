package agent

import (
	"beszel/internal/entities/container"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
)

type dockerManager struct {
	client              *http.Client                // Client to query Docker API
	wg                  sync.WaitGroup              // WaitGroup to wait for all goroutines to finish
	sem                 chan struct{}               // Semaphore to limit concurrent container requests
	containerStatsMutex sync.RWMutex                // Mutex to prevent concurrent access to containerStatsMap
	apiContainerList    []*container.ApiInfo        // List of containers from Docker API (no pointer)
	containerStatsMap   map[string]*container.Stats // Keeps track of container stats
	validIds            map[string]struct{}         // Map of valid container ids, used to prune invalid containers from containerStatsMap
	goodDockerVersion   bool                        // Whether docker version is at least 25.0.0 (one-shot works correctly)
	isWindows           bool                        // Whether the Docker Engine API is running on Windows
}

// userAgentRoundTripper is a custom http.RoundTripper that adds a User-Agent header to all requests
type userAgentRoundTripper struct {
	rt        http.RoundTripper
	userAgent string
}

// RoundTrip implements the http.RoundTripper interface
func (u *userAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", u.userAgent)
	return u.rt.RoundTrip(req)
}

// Add goroutine to the queue
func (d *dockerManager) queue() {
	d.wg.Add(1)
	if d.goodDockerVersion {
		d.sem <- struct{}{}
	}
}

// Remove goroutine from the queue
func (d *dockerManager) dequeue() {
	d.wg.Done()
	if d.goodDockerVersion {
		<-d.sem
	}
}

// Returns stats for all running containers and volume-to-container mapping
func (dm *dockerManager) getDockerStats() ([]*container.Stats, map[string][]string, error) {
	resp, err := dm.client.Get("http://localhost/containers/json?all=1")
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var apiContainers []*container.ApiInfo
	if err := json.NewDecoder(resp.Body).Decode(&apiContainers); err != nil {
		return nil, nil, err
	}

	// Get all volume sizes and names in one call (includes all volumes, not just those attached to containers)
	volumeSizes, _ := dm.getAllVolumeSizes()

	// Build volume-to-container mapping for all containers (running and stopped)
	volumeContainers := make(map[string][]string)
	for _, ctr := range apiContainers {
		ctr.IdShort = ctr.Id[:12]
		name := ctr.Names[0][1:]
		for _, mount := range ctr.Mounts {
			if mount.Type == "volume" && mount.Name != "" {
				volumeContainers[mount.Name] = append(volumeContainers[mount.Name], name)
			}
		}
	}

	type result struct {
		id    string
		stats *container.Stats
		err   error
	}
	results := make(chan result, len(apiContainers))
	sem := make(chan struct{}, 5) // configurable concurrency

	var wg sync.WaitGroup
	for _, ctr := range apiContainers {
		wg.Add(1)
		go func(ctr *container.ApiInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			stats, err := dm.collectContainerStats(ctr, volumeSizes)
			results <- result{id: ctr.IdShort, stats: stats, err: err}
		}(ctr)
	}
	wg.Wait()
	close(results)

	statsList := make([]*container.Stats, 0, len(apiContainers))
	for res := range results {
		if res.err == nil && res.stats != nil {
			statsList = append(statsList, res.stats)
		}
	}

	return statsList, volumeContainers, nil
}

// Collect stats for a single container
func (dm *dockerManager) collectContainerStats(ctr *container.ApiInfo, volumeSizes map[string]float64) (*container.Stats, error) {
	name := ctr.Names[0][1:]
	stats := &container.Stats{Name: name}

	// Always fetch /json to get canonical status and health
	detailResp, err := dm.client.Get("http://localhost/containers/" + ctr.IdShort + "/json")
	if err == nil {
		defer detailResp.Body.Close()
		var detail struct {
			State struct {
				Status    string                  `json:"Status"`
				StartedAt string                  `json:"StartedAt"`
				Health    struct{ Status string } `json:"Health"`
			} `json:"State"`
		}
		if err := json.NewDecoder(detailResp.Body).Decode(&detail); err == nil {
			stats.Status = detail.State.Status // canonical state: running, exited, etc.
			if detail.State.Health.Status != "" {
				stats.Health = detail.State.Health.Status
			} else {
				stats.Health = "none"
			}
			if detail.State.StartedAt != "" && ctr.StartedAt == 0 {
				if t, err := time.Parse(time.RFC3339Nano, detail.State.StartedAt); err == nil {
					ctr.StartedAt = t.Unix()
				}
			}
		}
	}

	if stats.Status == "" {
		// fallback to previous logic if /json failed
		stats.Status = ctr.State
		if stats.Status == "" {
			stats.Status = "unknown"
		}
	}

	if stats.Health == "" {
		stats.Health = "none"
		if ctr.Health != "" {
			stats.Health = ctr.Health
		}
	}

	if ctr.Labels != nil {
		if projectName, exists := ctr.Labels["com.docker.compose.project"]; exists {
			stats.Project = projectName
		}
	}
	stats.Volumes = make(map[string]float64)
	for _, mount := range ctr.Mounts {
		if mount.Type == "volume" && mount.Name != "" {
			stats.Volumes[mount.Name] = volumeSizes[mount.Name]
		}
	}

	isRunning := stats.Status == "running"

	// If running, fetch /stats
	if isRunning {
		resp, err := dm.client.Get("http://localhost/containers/" + ctr.IdShort + "/stats?stream=0&one-shot=1")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var res container.ApiStats
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, err
		}
		var usedMemory uint64
		var cpuPct float64
		if dm.isWindows {
			usedMemory = res.MemoryStats.PrivateWorkingSet
			cpuPct = res.CalculateCpuPercentWindows(stats.PrevCpu[0], stats.PrevRead)
		} else {
			if res.MemoryStats.Usage == 0 {
				return nil, fmt.Errorf("%s - no memory stats", name)
			}
			memCache := res.MemoryStats.Stats.InactiveFile
			if memCache == 0 {
				memCache = res.MemoryStats.Stats.Cache
			}
			usedMemory = res.MemoryStats.Usage - memCache
			cpuPct = res.CalculateCpuPercentLinux(stats.PrevCpu)
		}
		if cpuPct > 100 {
			return nil, fmt.Errorf("%s cpu pct greater than 100: %+v", name, cpuPct)
		}
		stats.PrevCpu = [2]uint64{res.CPUStats.CPUUsage.TotalUsage, res.CPUStats.SystemUsage}
		var total_sent, total_recv uint64
		for _, v := range res.Networks {
			total_sent += v.TxBytes
			total_recv += v.RxBytes
		}
		var sent_delta, recv_delta float64
		if stats.PrevRead.IsZero() {
			sent_delta = 0
			recv_delta = 0
		} else {
			secondsElapsed := time.Since(stats.PrevRead).Seconds()
			sent_delta = float64(total_sent-stats.PrevNet.Sent) / secondsElapsed
			recv_delta = float64(total_recv-stats.PrevNet.Recv) / secondsElapsed
		}
		stats.PrevNet.Sent = total_sent
		stats.PrevNet.Recv = total_recv
		stats.Cpu = twoDecimals(cpuPct)
		stats.Mem = bytesToMegabytes(float64(usedMemory))
		stats.NetworkSent = bytesToMegabytes(sent_delta)
		stats.NetworkRecv = bytesToMegabytes(recv_delta)
		stats.PrevRead = res.Read
	}

	// Uptime calculation
	if ctr.StartedAt > 0 {
		startedTime := time.Unix(ctr.StartedAt, 0)
		stats.Uptime = twoDecimals(time.Since(startedTime).Seconds())
	}

	if !isRunning {
		stats.Cpu = 0
		stats.Mem = 0
		stats.NetworkSent = 0
		stats.NetworkRecv = 0
	}

	return stats, nil
}

// Delete container stats from map using mutex
func (dm *dockerManager) deleteContainerStatsSync(id string) {
	dm.containerStatsMutex.Lock()
	defer dm.containerStatsMutex.Unlock()
	delete(dm.containerStatsMap, id)
}

// Get size of a specific volume
func (dm *dockerManager) getVolumeSize(volumeName string) (float64, error) {
	// Use the system/df endpoint to get volume usage data
	resp, err := dm.client.Get("http://localhost/system/df")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var dfData struct {
		Volumes []struct {
			Name      string `json:"Name"`
			UsageData struct {
				Size int64 `json:"Size"`
			} `json:"UsageData"`
		} `json:"Volumes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&dfData); err != nil {
		return 0, err
	}

	// Find the specific volume
	for _, volume := range dfData.Volumes {
		if volume.Name == volumeName {
			// Convert bytes to MB
			return float64(volume.UsageData.Size) / (1024 * 1024), nil
		}
	}

	return 0, fmt.Errorf("volume %s not found", volumeName)
}

// Get all volume sizes in a single API call
func (dm *dockerManager) getAllVolumeSizes() (map[string]float64, error) {
	resp, err := dm.client.Get("http://localhost/system/df")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var dfData struct {
		Volumes []struct {
			Name      string `json:"Name"`
			UsageData struct {
				Size int64 `json:"Size"`
			} `json:"UsageData"`
		} `json:"Volumes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&dfData); err != nil {
		return nil, err
	}

	volumeSizes := make(map[string]float64)
	for _, volume := range dfData.Volumes {
		// Convert bytes to MB
		volumeSizes[volume.Name] = float64(volume.UsageData.Size) / (1024 * 1024)
	}

	return volumeSizes, nil
}

// Creates a new http client for Docker or Podman API
func newDockerManager(a *Agent) *dockerManager {
	dockerHost, exists := GetEnv("DOCKER_HOST")
	if exists {
		slog.Info("DOCKER_HOST", "host", dockerHost)
		// return nil if set to empty string
		if dockerHost == "" {
			return nil
		}
	} else {
		dockerHost = getDockerHost()
	}

	parsedURL, err := url.Parse(dockerHost)
	if err != nil {
		slog.Error("Error parsing DOCKER_HOST", "err", err)
		os.Exit(1)
	}

	transport := &http.Transport{
		DisableCompression: true,
		MaxConnsPerHost:    0,
	}

	switch parsedURL.Scheme {
	case "unix":
		transport.DialContext = func(ctx context.Context, proto, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", parsedURL.Path)
		}
	case "tcp", "http", "https":
		transport.DialContext = func(ctx context.Context, proto, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", parsedURL.Host)
		}
	default:
		slog.Error("Invalid DOCKER_HOST", "scheme", parsedURL.Scheme)
		os.Exit(1)
	}

	// configurable timeout
	timeout := time.Millisecond * 2100
	if t, set := GetEnv("DOCKER_TIMEOUT"); set {
		timeout, err = time.ParseDuration(t)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		slog.Info("DOCKER_TIMEOUT", "timeout", timeout)
	}

	// Custom user-agent to avoid docker bug: https://github.com/docker/for-mac/issues/7575
	userAgentTransport := &userAgentRoundTripper{
		rt:        transport,
		userAgent: "Docker-Client/",
	}

	manager := &dockerManager{
		client: &http.Client{
			Timeout:   timeout,
			Transport: userAgentTransport,
		},
		containerStatsMap: make(map[string]*container.Stats),
		sem:               make(chan struct{}, 5),
		apiContainerList:  []*container.ApiInfo{},
	}

	// If using podman, return client
	if strings.Contains(dockerHost, "podman") {
		a.systemInfo.Podman = true
		manager.goodDockerVersion = true
		return manager
	}

	// Check docker version
	// (versions before 25.0.0 have a bug with one-shot which requires all requests to be made in one batch)
	var versionInfo struct {
		Version string `json:"Version"`
	}
	resp, err := manager.client.Get("http://localhost/version")
	if err != nil {
		return manager
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return manager
	}

	// if version > 24, one-shot works correctly and we can limit concurrent operations
	if dockerVersion, err := semver.Parse(versionInfo.Version); err == nil && dockerVersion.Major > 24 {
		manager.goodDockerVersion = true
	} else {
		slog.Info(fmt.Sprintf("Docker %s is outdated. Upgrade if possible. See https://github.com/henrygd/beszel/issues/58", versionInfo.Version))
	}

	return manager
}

// Test docker / podman sockets and return if one exists
func getDockerHost() string {
	scheme := "unix://"
	socks := []string{"/var/run/docker.sock", fmt.Sprintf("/run/user/%v/podman/podman.sock", os.Getuid())}
	for _, sock := range socks {
		if _, err := os.Stat(sock); err == nil {
			return scheme + sock
		}
	}
	return scheme + socks[0]
}
