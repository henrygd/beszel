package agent

import (
	"beszel/internal/entities/container"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
)

const (
	dockerAPIBase              = "http://localhost"
	dockerContainersEndpoint   = dockerAPIBase + "/containers/json?all=1"
	dockerVersionEndpoint      = dockerAPIBase + "/version"
	dockerSystemDfEndpoint     = dockerAPIBase + "/system/df"
	dockerStatsEndpointPattern = dockerAPIBase + "/containers/%s/stats?stream=0&one-shot=1"
	dockerInfoEndpointPattern  = dockerAPIBase + "/containers/%s/json"
	
	defaultConcurrency         = 5
	defaultTimeout             = time.Millisecond * 2100
	requestTimeout             = time.Second * 2
	volumeRequestTimeout       = time.Second * 5
	maxConnsPerHost           = 100
	maxIdleConns              = 50
	idleConnTimeout           = 90 * time.Second
	
	containerIDShortLength     = 12
	dockerMajorVersionMin      = 24
	bytesToMB                  = 1024 * 1024
	
	dockerSocketPath           = "/var/run/docker.sock"
	podmanSocketPathPattern    = "/run/user/%v/podman/podman.sock"
	unixScheme                 = "unix://"
	
	composeProjectLabel        = "com.docker.compose.project"
	dockerClientUserAgent      = "Docker-Client/"
	podmanIdentifier          = "podman"
	
	healthStatusNone          = "none"
	containerStateRunning     = "running"
	containerStateUnknown     = "unknown"
	
	volumeTypeVolume          = "volume"
	diskOpRead               = "read"
	diskOpReadCap            = "Read" 
	diskOpWrite              = "write"
	diskOpWriteCap           = "Write"
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
	excludePatterns     []string                    // Container name patterns to exclude from monitoring
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

// shouldExcludeContainer checks if a container should be excluded based on name patterns
func (dm *dockerManager) shouldExcludeContainer(name string) bool {
	for _, pattern := range dm.excludePatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}
	return false
}

// filterContainers removes excluded containers from the list
func (dm *dockerManager) filterContainers(apiContainers []*container.ApiInfo) []*container.ApiInfo {
	filteredContainers := make([]*container.ApiInfo, 0, len(apiContainers))
	for _, ctr := range apiContainers {
		if ctr == nil || len(ctr.Names) == 0 {
			slog.Warn("Skipping container with invalid name data")
			continue
		}
		name := ctr.Names[0][1:]
		if !dm.shouldExcludeContainer(name) {
			filteredContainers = append(filteredContainers, ctr)
		} else {
			slog.Debug("Excluding container from monitoring", "name", name)
		}
	}
	return filteredContainers
}

// buildVolumeContainerMapping creates a mapping of volumes to containers and sets short IDs
func (dm *dockerManager) buildVolumeContainerMapping(apiContainers []*container.ApiInfo) map[string][]string {
	volumeContainers := make(map[string][]string, len(apiContainers))
	for _, ctr := range apiContainers {
		if ctr == nil || len(ctr.Id) < containerIDShortLength || len(ctr.Names) == 0 {
			slog.Warn("Skipping container with invalid data", "id", ctr.Id)
			continue
		}
		ctr.IdShort = ctr.Id[:containerIDShortLength]
		name := ctr.Names[0][1:]
		for _, mount := range ctr.Mounts {
			if mount.Type == volumeTypeVolume && mount.Name != "" {
				volumeContainers[mount.Name] = append(volumeContainers[mount.Name], name)
			}
		}
	}
	return volumeContainers
}

// collectStatsInParallel collects container stats using goroutines with controlled concurrency
func (dm *dockerManager) collectStatsInParallel(containers []*container.ApiInfo, volumeSizes map[string]float64) []*container.Stats {
	type result struct {
		id    string
		stats *container.Stats
		err   error
	}
	results := make(chan result, len(containers))

	// Use adaptive concurrency based on container count
	concurrency := defaultConcurrency
	if len(containers) < concurrency {
		concurrency = len(containers)
	}
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	for _, ctr := range containers {
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

	statsList := make([]*container.Stats, 0, len(containers))
	for res := range results {
		if res.err == nil && res.stats != nil {
			statsList = append(statsList, res.stats)
		}
	}
	
	return statsList
}

// Returns stats for all running containers and volume-to-container mapping
func (dm *dockerManager) getDockerStats() ([]*container.Stats, map[string][]string, error) {
	// Use context with timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), dm.client.Timeout)
	defer cancel()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", dockerContainersEndpoint, nil)
	if err != nil {
		slog.Error("Failed to create containers list request", "error", err)
		return nil, nil, err
	}

	resp, err := dm.client.Do(req)
	if err != nil {
		slog.Error("Failed to get containers list from Docker API", "error", err)
		return nil, nil, err
	}
	defer resp.Body.Close()

	var apiContainers []*container.ApiInfo
	if err := json.NewDecoder(resp.Body).Decode(&apiContainers); err != nil {
		slog.Error("Failed to decode containers list response", "error", err)
		return nil, nil, err
	}

	// Get all volume sizes and names in one call (includes all volumes, not just those attached to containers)
	volumeSizes, err := dm.getAllVolumeSizes()
	if err != nil {
		slog.Warn("Failed to get volume sizes, continuing without volume data", "error", err)
		volumeSizes = make(map[string]float64)
	}

	// Build volume-to-container mapping and set short IDs for all containers
	volumeContainers := dm.buildVolumeContainerMapping(apiContainers)

	// Filter out excluded containers
	filteredContainers := dm.filterContainers(apiContainers)

	statsList := dm.collectStatsInParallel(filteredContainers, volumeSizes)

	slog.Debug("Container stats collection completed", "count", len(statsList))
	for _, stats := range statsList {
		slog.Debug("Container stats", 
			"name", stats.Name, 
			"cpu", stats.Cpu, 
			"mem", stats.Mem, 
			"disk_read", stats.DiskRead, 
			"disk_write", stats.DiskWrite)
	}
	
	return statsList, volumeContainers, nil
}

// Collect stats for a single container
func (dm *dockerManager) collectContainerStats(ctr *container.ApiInfo, volumeSizes map[string]float64) (*container.Stats, error) {
	name := ctr.Names[0][1:]
	
	// Get existing stats or create new one
	dm.containerStatsMutex.Lock()
	stats, exists := dm.containerStatsMap[ctr.IdShort]
	if !exists {
		stats = &container.Stats{Name: name}
		dm.containerStatsMap[ctr.IdShort] = stats
	}
	dm.containerStatsMutex.Unlock()
	
	// Update the name in case it changed
	stats.Name = name

	// Set the short ID for the frontend
	stats.IdShort = ctr.IdShort

	// Use container data from the list when possible to reduce API calls
	stats.Status = ctr.State
	if stats.Status == "" {
		stats.Status = containerStateUnknown
	}

	// Only fetch /json if we need additional details not available in the list
	needJsonCall := ctr.StartedAt == 0 || ctr.Health == "" || ctr.FinishedAt == 0
	if needJsonCall {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(dockerInfoEndpointPattern, ctr.IdShort), nil)
		if err == nil {
			detailResp, err := dm.client.Do(req)
			if err == nil {
				defer detailResp.Body.Close()
				var detail struct {
					State struct {
						Status     string                  `json:"Status"`
						StartedAt  string                  `json:"StartedAt"`
						FinishedAt string                  `json:"FinishedAt"`
						Health     struct{ Status string } `json:"Health"`
					} `json:"State"`
				}
				if err := json.NewDecoder(detailResp.Body).Decode(&detail); err == nil {
					stats.Status = detail.State.Status // canonical state: running, exited, etc.
					if detail.State.Health.Status != "" {
						stats.Health = detail.State.Health.Status
					} else {
						stats.Health = healthStatusNone
					}
					if detail.State.StartedAt != "" && ctr.StartedAt == 0 {
						if t, err := time.Parse(time.RFC3339Nano, detail.State.StartedAt); err == nil {
							ctr.StartedAt = t.Unix()
						}
					}
					// Parse FinishedAt for stopped containers
					if detail.State.FinishedAt != "" {
						if t, err := time.Parse(time.RFC3339Nano, detail.State.FinishedAt); err == nil {
							ctr.FinishedAt = t.Unix()
						}
					}
				}
			}
		}
	}

	if stats.Health == "" {
		stats.Health = healthStatusNone
		if ctr.Health != "" {
			stats.Health = ctr.Health
		}
	}

	if ctr.Labels != nil {
		if projectName, exists := ctr.Labels[composeProjectLabel]; exists {
			stats.Project = projectName
		}
	}

	// Pre-allocate volumes map with known size
	volumeCount := 0
	for _, mount := range ctr.Mounts {
		if mount.Type == volumeTypeVolume && mount.Name != "" {
			volumeCount++
		}
	}
	stats.Volumes = make(map[string]float64, volumeCount)

	for _, mount := range ctr.Mounts {
		if mount.Type == volumeTypeVolume && mount.Name != "" {
			stats.Volumes[mount.Name] = volumeSizes[mount.Name]
		}
	}

	isRunning := stats.Status == containerStateRunning

	// If running, fetch /stats
	if isRunning {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf(dockerStatsEndpointPattern, ctr.IdShort), nil)
		if err != nil {
			slog.Error("Failed to create stats request", "container", name, "error", err)
			return nil, err
		}

		resp, err := dm.client.Do(req)
		if err != nil {
			slog.Error("Failed to get stats from Docker API", "container", name, "error", err)
			return nil, err
		}
		defer resp.Body.Close()

		var res container.ApiStats
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			slog.Error("Failed to decode stats response", "container", name, "error", err)
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

		// Calculate disk I/O stats
		var total_read, total_write uint64
		for _, entry := range res.BlkioStats.IoServiceBytesRecursive {
			switch entry.Op {
			case diskOpRead, diskOpReadCap:
				total_read += entry.Value
			case diskOpWrite, diskOpWriteCap:
				total_write += entry.Value
			}
		}
		var read_delta, write_delta float64
		if stats.PrevRead.IsZero() {
			read_delta = 0
			write_delta = 0
			slog.Debug("First disk measurement for container", "name", name)
		} else {
			secondsElapsed := time.Since(stats.PrevRead).Seconds()
			read_delta = float64(total_read-stats.PrevDisk.Read) / secondsElapsed
			write_delta = float64(total_write-stats.PrevDisk.Write) / secondsElapsed
			if total_read > 0 || total_write > 0 {
				slog.Debug("Disk I/O calculation", 
					"name", name,
					"prev_read_bytes", stats.PrevDisk.Read,
					"total_read_bytes", total_read,
					"read_rate_mb_per_sec", read_delta/bytesToMB,
					"seconds_elapsed", secondsElapsed)
			}
		}
		stats.PrevDisk.Read = total_read
		stats.PrevDisk.Write = total_write

		stats.Cpu = twoDecimals(cpuPct)
		stats.Mem = bytesToMegabytes(float64(usedMemory))
		stats.NetworkSent = bytesToMegabytes(sent_delta)
		stats.NetworkRecv = bytesToMegabytes(recv_delta)
		stats.DiskRead = bytesToMegabytes(read_delta)
		stats.DiskWrite = bytesToMegabytes(write_delta)
		stats.PrevRead = res.Read
		
		if stats.DiskRead > 0 || stats.DiskWrite > 0 {
			slog.Debug("Final disk I/O stats", 
				"name", name, 
				"disk_read_mb_per_sec", stats.DiskRead, 
				"disk_write_mb_per_sec", stats.DiskWrite)
		}
	}

	// Uptime calculation
	if ctr.StartedAt > 0 && isRunning {
		startedTime := time.Unix(ctr.StartedAt, 0)
		stats.Uptime = twoDecimals(time.Since(startedTime).Seconds())
	}

	if !isRunning {
		stats.Cpu = 0
		stats.Mem = 0
		stats.NetworkSent = 0
		stats.NetworkRecv = 0
		stats.DiskRead = 0
		stats.DiskWrite = 0
	}

	return stats, nil
}

// Delete container stats from map using mutex
func (dm *dockerManager) deleteContainerStatsSync(id string) {
	dm.containerStatsMutex.Lock()
	defer dm.containerStatsMutex.Unlock()
	delete(dm.containerStatsMap, id)
}

// Get all volume sizes in a single API call
func (dm *dockerManager) getAllVolumeSizes() (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), volumeRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", dockerSystemDfEndpoint, nil)
	if err != nil {
		slog.Error("Failed to create system df request", "error", err)
		return nil, err
	}

	resp, err := dm.client.Do(req)
	if err != nil {
		slog.Error("Failed to get system df from Docker API", "error", err)
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
		slog.Error("Failed to decode system df response", "error", err)
		return nil, err
	}

	volumeSizes := make(map[string]float64, len(dfData.Volumes))
	for _, volume := range dfData.Volumes {
		// Convert bytes to MB
		volumeSizes[volume.Name] = float64(volume.UsageData.Size) / bytesToMB
	}

	return volumeSizes, nil
}

// createHTTPClient creates an HTTP client configured for Docker/Podman API
func createHTTPClient(dockerHost string) *http.Client {
	transport := &http.Transport{
		DisableCompression: true,
		MaxConnsPerHost:    maxConnsPerHost,
		MaxIdleConns:       maxIdleConns,
		IdleConnTimeout:    idleConnTimeout,
	}

	// Always use Unix socket for local Docker/Podman
	transport.DialContext = func(ctx context.Context, proto, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", dockerHost[len(unixScheme):])
	}

	// configurable timeout
	timeout := defaultTimeout
	if t, set := GetEnv("DOCKER_TIMEOUT"); set {
		if parsedTimeout, err := time.ParseDuration(t); err == nil {
			timeout = parsedTimeout
			slog.Info("DOCKER_TIMEOUT", "timeout", timeout)
		} else {
			slog.Error("Invalid DOCKER_TIMEOUT", "error", err)
		}
	}

	// Custom user-agent to avoid docker bug: https://github.com/docker/for-mac/issues/7575
	userAgentTransport := &userAgentRoundTripper{
		rt:        transport,
		userAgent: dockerClientUserAgent,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: userAgentTransport,
	}
}

// Creates a new http client for Docker or Podman API
func newDockerManager(a *Agent) *dockerManager {
	dockerHost := getDockerHost()

	manager := &dockerManager{
		client:            createHTTPClient(dockerHost),
		containerStatsMap: make(map[string]*container.Stats),
		sem:               make(chan struct{}, defaultConcurrency),
		apiContainerList:  []*container.ApiInfo{},
	}

	// Load container exclusion patterns from environment
	if excludePatterns, exists := GetEnv("CONTAINER_EXCLUDE"); exists && excludePatterns != "" {
		patterns := strings.Split(excludePatterns, ",")
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				manager.excludePatterns = append(manager.excludePatterns, pattern)
			}
		}
		if len(manager.excludePatterns) > 0 {
			slog.Info("Container exclusion patterns loaded", "patterns", manager.excludePatterns)
		}
	}

	// If using podman, return client
	if strings.Contains(dockerHost, podmanIdentifier) {
		a.systemInfo.Podman = true
		manager.goodDockerVersion = true
		return manager
	}

	// Check docker version
	// (versions before 25.0.0 have a bug with one-shot which requires all requests to be made in one batch)
	var versionInfo struct {
		Version string `json:"Version"`
	}
	resp, err := manager.client.Get(dockerVersionEndpoint)
	if err != nil {
		slog.Warn("Failed to get Docker version, assuming older version", "error", err)
		return manager
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		slog.Warn("Failed to decode Docker version response, assuming older version", "error", err)
		return manager
	}

	// if version > 24, one-shot works correctly and we can limit concurrent operations
	if dockerVersion, err := semver.Parse(versionInfo.Version); err == nil && dockerVersion.Major > dockerMajorVersionMin {
		manager.goodDockerVersion = true
	} else {
		slog.Info(fmt.Sprintf("Docker %s is outdated. Upgrade if possible. See https://github.com/henrygd/beszel/issues/58", versionInfo.Version))
	}

	return manager
}

// Test docker / podman sockets and return if one exists
func getDockerHost() string {
	socks := []string{dockerSocketPath, fmt.Sprintf(podmanSocketPathPattern, os.Getuid())}
	for _, sock := range socks {
		if _, err := os.Stat(sock); err == nil {
			return unixScheme + sock
		}
	}
	return unixScheme + socks[0]
}
