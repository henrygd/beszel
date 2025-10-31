package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/internal/entities/container"

	"github.com/blang/semver"
)

const (
	// Docker API timeout in milliseconds
	dockerTimeoutMs = 2100
	// Maximum realistic network speed (5 GB/s) to detect bad deltas
	maxNetworkSpeedBps uint64 = 5e9
	// Maximum conceivable memory usage of a container (100TB) to detect bad memory stats
	maxMemoryUsage uint64 = 100 * 1024 * 1024 * 1024 * 1024
	// Number of log lines to request when fetching container logs
	dockerLogsTail = 200
	// Maximum size of a single log frame (1MB) to prevent memory exhaustion
	// A single log line larger than 1MB is likely an error or misconfiguration
	maxLogFrameSize = 1024 * 1024
	// Maximum total log content size (5MB) to prevent memory exhaustion
	// This provides a reasonable limit for network transfer and browser rendering
	maxTotalLogSize = 5 * 1024 * 1024
)

type dockerManager struct {
	client              *http.Client                // Client to query Docker API
	wg                  sync.WaitGroup              // WaitGroup to wait for all goroutines to finish
	sem                 chan struct{}               // Semaphore to limit concurrent container requests
	containerStatsMutex sync.RWMutex                // Mutex to prevent concurrent access to containerStatsMap
	apiContainerList    []*container.ApiInfo        // List of containers from Docker API
	containerStatsMap   map[string]*container.Stats // Keeps track of container stats
	validIds            map[string]struct{}         // Map of valid container ids, used to prune invalid containers from containerStatsMap
	goodDockerVersion   bool                        // Whether docker version is at least 25.0.0 (one-shot works correctly)
	isWindows           bool                        // Whether the Docker Engine API is running on Windows
	buf                 *bytes.Buffer               // Buffer to store and read response bodies
	decoder             *json.Decoder               // Reusable JSON decoder that reads from buf
	apiStats            *container.ApiStats         // Reusable API stats object
	excludeContainers   []string                    // Patterns to exclude containers by name

	// Cache-time-aware tracking for CPU stats (similar to cpu.go)
	// Maps cache time intervals to container-specific CPU usage tracking
	lastCpuContainer map[uint16]map[string]uint64    // cacheTimeMs -> containerId -> last cpu container usage
	lastCpuSystem    map[uint16]map[string]uint64    // cacheTimeMs -> containerId -> last cpu system usage
	lastCpuReadTime  map[uint16]map[string]time.Time // cacheTimeMs -> containerId -> last read time (Windows)

	// Network delta trackers - one per cache time to avoid interference
	// cacheTimeMs -> DeltaTracker for network bytes sent/received
	networkSentTrackers map[uint16]*deltatracker.DeltaTracker[string, uint64]
	networkRecvTrackers map[uint16]*deltatracker.DeltaTracker[string, uint64]
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

// shouldExcludeContainer checks if a container name matches any exclusion pattern
func (dm *dockerManager) shouldExcludeContainer(name string) bool {
	if len(dm.excludeContainers) == 0 {
		return false
	}
	for _, pattern := range dm.excludeContainers {
		if match, _ := path.Match(pattern, name); match {
			return true
		}
	}
	return false
}

// Returns stats for all running containers with cache-time-aware delta tracking
func (dm *dockerManager) getDockerStats(cacheTimeMs uint16) ([]*container.Stats, error) {
	resp, err := dm.client.Get("http://localhost/containers/json")
	if err != nil {
		return nil, err
	}

	dm.apiContainerList = dm.apiContainerList[:0]
	if err := dm.decode(resp, &dm.apiContainerList); err != nil {
		return nil, err
	}

	dm.isWindows = strings.Contains(resp.Header.Get("Server"), "windows")

	containersLength := len(dm.apiContainerList)

	// store valid ids to clean up old container ids from map
	if dm.validIds == nil {
		dm.validIds = make(map[string]struct{}, containersLength)
	} else {
		clear(dm.validIds)
	}

	var failedContainers []*container.ApiInfo

	for _, ctr := range dm.apiContainerList {
		ctr.IdShort = ctr.Id[:12]

		// Skip this container if it matches the exclusion pattern
		if dm.shouldExcludeContainer(ctr.Names[0][1:]) {
			slog.Debug("Excluding container", "name", ctr.Names[0][1:])
			continue
		}

		dm.validIds[ctr.IdShort] = struct{}{}
		// check if container is less than 1 minute old (possible restart)
		// note: can't use Created field because it's not updated on restart
		if strings.Contains(ctr.Status, "second") {
			// if so, remove old container data
			dm.deleteContainerStatsSync(ctr.IdShort)
		}
		dm.queue()
		go func(ctr *container.ApiInfo) {
			defer dm.dequeue()
			err := dm.updateContainerStats(ctr, cacheTimeMs)
			// if error, delete from map and add to failed list to retry
			if err != nil {
				dm.containerStatsMutex.Lock()
				delete(dm.containerStatsMap, ctr.IdShort)
				failedContainers = append(failedContainers, ctr)
				dm.containerStatsMutex.Unlock()
			}
		}(ctr)
	}

	dm.wg.Wait()

	// retry failed containers separately so we can run them in parallel (docker 24 bug)
	if len(failedContainers) > 0 {
		slog.Debug("Retrying failed containers", "count", len(failedContainers))
		for i := range failedContainers {
			ctr := failedContainers[i]
			dm.queue()
			go func(ctr *container.ApiInfo) {
				defer dm.dequeue()
				if err2 := dm.updateContainerStats(ctr, cacheTimeMs); err2 != nil {
					slog.Error("Error getting container stats", "err", err2)
				}
			}(ctr)
		}
		dm.wg.Wait()
	}

	// populate final stats and remove old / invalid container stats
	stats := make([]*container.Stats, 0, containersLength)
	for id, v := range dm.containerStatsMap {
		if _, exists := dm.validIds[id]; !exists {
			delete(dm.containerStatsMap, id)
		} else {
			stats = append(stats, v)
		}
	}

	// prepare network trackers for next interval for this cache time
	dm.cycleNetworkDeltasForCacheTime(cacheTimeMs)

	return stats, nil
}

// initializeCpuTracking initializes CPU tracking maps for a specific cache time interval
func (dm *dockerManager) initializeCpuTracking(cacheTimeMs uint16) {
	// Initialize cache time maps if they don't exist
	if dm.lastCpuContainer[cacheTimeMs] == nil {
		dm.lastCpuContainer[cacheTimeMs] = make(map[string]uint64)
	}
	if dm.lastCpuSystem[cacheTimeMs] == nil {
		dm.lastCpuSystem[cacheTimeMs] = make(map[string]uint64)
	}
	// Ensure the outer map exists before indexing
	if dm.lastCpuReadTime == nil {
		dm.lastCpuReadTime = make(map[uint16]map[string]time.Time)
	}
	if dm.lastCpuReadTime[cacheTimeMs] == nil {
		dm.lastCpuReadTime[cacheTimeMs] = make(map[string]time.Time)
	}
}

// getCpuPreviousValues returns previous CPU values for a container and cache time interval
func (dm *dockerManager) getCpuPreviousValues(cacheTimeMs uint16, containerId string) (uint64, uint64) {
	return dm.lastCpuContainer[cacheTimeMs][containerId], dm.lastCpuSystem[cacheTimeMs][containerId]
}

// setCpuCurrentValues stores current CPU values for a container and cache time interval
func (dm *dockerManager) setCpuCurrentValues(cacheTimeMs uint16, containerId string, cpuContainer, cpuSystem uint64) {
	dm.lastCpuContainer[cacheTimeMs][containerId] = cpuContainer
	dm.lastCpuSystem[cacheTimeMs][containerId] = cpuSystem
}

// calculateMemoryUsage calculates memory usage from Docker API stats
func calculateMemoryUsage(apiStats *container.ApiStats, isWindows bool) (uint64, error) {
	if isWindows {
		return apiStats.MemoryStats.PrivateWorkingSet, nil
	}

	memCache := apiStats.MemoryStats.Stats.InactiveFile
	if memCache == 0 {
		memCache = apiStats.MemoryStats.Stats.Cache
	}

	usedDelta := apiStats.MemoryStats.Usage - memCache
	if usedDelta <= 0 || usedDelta > maxMemoryUsage {
		return 0, fmt.Errorf("bad memory stats")
	}

	return usedDelta, nil
}

// getNetworkTracker returns the DeltaTracker for a specific cache time, creating it if needed
func (dm *dockerManager) getNetworkTracker(cacheTimeMs uint16, isSent bool) *deltatracker.DeltaTracker[string, uint64] {
	var trackers map[uint16]*deltatracker.DeltaTracker[string, uint64]
	if isSent {
		trackers = dm.networkSentTrackers
	} else {
		trackers = dm.networkRecvTrackers
	}

	if trackers[cacheTimeMs] == nil {
		trackers[cacheTimeMs] = deltatracker.NewDeltaTracker[string, uint64]()
	}

	return trackers[cacheTimeMs]
}

// cycleNetworkDeltasForCacheTime cycles the network delta trackers for a specific cache time
func (dm *dockerManager) cycleNetworkDeltasForCacheTime(cacheTimeMs uint16) {
	if dm.networkSentTrackers[cacheTimeMs] != nil {
		dm.networkSentTrackers[cacheTimeMs].Cycle()
	}
	if dm.networkRecvTrackers[cacheTimeMs] != nil {
		dm.networkRecvTrackers[cacheTimeMs].Cycle()
	}
}

// calculateNetworkStats calculates network sent/receive deltas using DeltaTracker
func (dm *dockerManager) calculateNetworkStats(ctr *container.ApiInfo, apiStats *container.ApiStats, stats *container.Stats, initialized bool, name string, cacheTimeMs uint16) (uint64, uint64) {
	var total_sent, total_recv uint64
	for _, v := range apiStats.Networks {
		total_sent += v.TxBytes
		total_recv += v.RxBytes
	}

	// Get the DeltaTracker for this specific cache time
	sentTracker := dm.getNetworkTracker(cacheTimeMs, true)
	recvTracker := dm.getNetworkTracker(cacheTimeMs, false)

	// Set current values in the cache-time-specific DeltaTracker
	sentTracker.Set(ctr.IdShort, total_sent)
	recvTracker.Set(ctr.IdShort, total_recv)

	// Get deltas (bytes since last measurement)
	sent_delta_raw := sentTracker.Delta(ctr.IdShort)
	recv_delta_raw := recvTracker.Delta(ctr.IdShort)

	// Calculate bytes per second independently for Tx and Rx if we have previous data
	var sent_delta, recv_delta uint64
	if initialized {
		millisecondsElapsed := uint64(time.Since(stats.PrevReadTime).Milliseconds())
		if millisecondsElapsed > 0 {
			if sent_delta_raw > 0 {
				sent_delta = sent_delta_raw * 1000 / millisecondsElapsed
				if sent_delta > maxNetworkSpeedBps {
					slog.Warn("Bad network delta", "container", name)
					sent_delta = 0
				}
			}
			if recv_delta_raw > 0 {
				recv_delta = recv_delta_raw * 1000 / millisecondsElapsed
				if recv_delta > maxNetworkSpeedBps {
					slog.Warn("Bad network delta", "container", name)
					recv_delta = 0
				}
			}
		}
	}

	return sent_delta, recv_delta
}

// validateCpuPercentage checks if CPU percentage is within valid range
func validateCpuPercentage(cpuPct float64, containerName string) error {
	if cpuPct > 100 {
		return fmt.Errorf("%s cpu pct greater than 100: %+v", containerName, cpuPct)
	}
	return nil
}

// updateContainerStatsValues updates the final stats values
func updateContainerStatsValues(stats *container.Stats, cpuPct float64, usedMemory uint64, sent_delta, recv_delta uint64, readTime time.Time) {
	stats.Cpu = twoDecimals(cpuPct)
	stats.Mem = bytesToMegabytes(float64(usedMemory))
	stats.NetworkSent = bytesToMegabytes(float64(sent_delta))
	stats.NetworkRecv = bytesToMegabytes(float64(recv_delta))
	stats.PrevReadTime = readTime
}

func parseDockerStatus(status string) (string, container.DockerHealth) {
	trimmed := strings.TrimSpace(status)
	if trimmed == "" {
		return "", container.DockerHealthNone
	}

	// Remove "About " from status
	trimmed = strings.Replace(trimmed, "About ", "", 1)

	openIdx := strings.LastIndex(trimmed, "(")
	if openIdx == -1 || !strings.HasSuffix(trimmed, ")") {
		return trimmed, container.DockerHealthNone
	}

	statusText := strings.TrimSpace(trimmed[:openIdx])
	if statusText == "" {
		statusText = trimmed
	}

	healthText := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(trimmed[openIdx+1:], ")")))
	// Some Docker statuses include a "health:" prefix inside the parentheses.
	// Strip it so it maps correctly to the known health states.
	if colonIdx := strings.IndexRune(healthText, ':'); colonIdx != -1 {
		prefix := strings.TrimSpace(healthText[:colonIdx])
		if prefix == "health" || prefix == "health status" {
			healthText = strings.TrimSpace(healthText[colonIdx+1:])
		}
	}
	if health, ok := container.DockerHealthStrings[healthText]; ok {
		return statusText, health
	}

	return trimmed, container.DockerHealthNone
}

// Updates stats for individual container with cache-time-aware delta tracking
func (dm *dockerManager) updateContainerStats(ctr *container.ApiInfo, cacheTimeMs uint16) error {
	name := ctr.Names[0][1:]

	resp, err := dm.client.Get(fmt.Sprintf("http://localhost/containers/%s/stats?stream=0&one-shot=1", ctr.IdShort))
	if err != nil {
		return err
	}

	dm.containerStatsMutex.Lock()
	defer dm.containerStatsMutex.Unlock()

	// add empty values if they doesn't exist in map
	stats, initialized := dm.containerStatsMap[ctr.IdShort]
	if !initialized {
		stats = &container.Stats{Name: name, Id: ctr.IdShort, Image: ctr.Image}
		dm.containerStatsMap[ctr.IdShort] = stats
	}

	stats.Id = ctr.IdShort

	statusText, health := parseDockerStatus(ctr.Status)
	stats.Status = statusText
	stats.Health = health

	// reset current stats
	stats.Cpu = 0
	stats.Mem = 0
	stats.NetworkSent = 0
	stats.NetworkRecv = 0

	res := dm.apiStats
	res.Networks = nil
	if err := dm.decode(resp, res); err != nil {
		return err
	}

	// Initialize CPU tracking for this cache time interval
	dm.initializeCpuTracking(cacheTimeMs)

	// Get previous CPU values
	prevCpuContainer, prevCpuSystem := dm.getCpuPreviousValues(cacheTimeMs, ctr.IdShort)

	// Calculate CPU percentage based on platform
	var cpuPct float64
	if dm.isWindows {
		prevRead := dm.lastCpuReadTime[cacheTimeMs][ctr.IdShort]
		cpuPct = res.CalculateCpuPercentWindows(prevCpuContainer, prevRead)
	} else {
		cpuPct = res.CalculateCpuPercentLinux(prevCpuContainer, prevCpuSystem)
	}

	// Calculate memory usage
	usedMemory, err := calculateMemoryUsage(res, dm.isWindows)
	if err != nil {
		return fmt.Errorf("%s - %w - see https://github.com/henrygd/beszel/issues/144", name, err)
	}

	// Store current CPU stats for next calculation
	currentCpuContainer := res.CPUStats.CPUUsage.TotalUsage
	currentCpuSystem := res.CPUStats.SystemUsage
	dm.setCpuCurrentValues(cacheTimeMs, ctr.IdShort, currentCpuContainer, currentCpuSystem)

	// Validate CPU percentage
	if err := validateCpuPercentage(cpuPct, name); err != nil {
		return err
	}

	// Calculate network stats using DeltaTracker
	sent_delta, recv_delta := dm.calculateNetworkStats(ctr, res, stats, initialized, name, cacheTimeMs)

	// Store current network values for legacy compatibility
	var total_sent, total_recv uint64
	for _, v := range res.Networks {
		total_sent += v.TxBytes
		total_recv += v.RxBytes
	}
	stats.PrevNet.Sent, stats.PrevNet.Recv = total_sent, total_recv

	// Update final stats values
	updateContainerStatsValues(stats, cpuPct, usedMemory, sent_delta, recv_delta, res.Read)
	// store per-cache-time read time for Windows CPU percent calc
	dm.lastCpuReadTime[cacheTimeMs][ctr.IdShort] = res.Read

	return nil
}

// Delete container stats from map using mutex
func (dm *dockerManager) deleteContainerStatsSync(id string) {
	dm.containerStatsMutex.Lock()
	defer dm.containerStatsMutex.Unlock()
	delete(dm.containerStatsMap, id)
	for ct := range dm.lastCpuContainer {
		delete(dm.lastCpuContainer[ct], id)
	}
	for ct := range dm.lastCpuSystem {
		delete(dm.lastCpuSystem[ct], id)
	}
	for ct := range dm.lastCpuReadTime {
		delete(dm.lastCpuReadTime[ct], id)
	}
}

// Creates a new http client for Docker or Podman API
func newDockerManager(a *Agent) *dockerManager {
	dockerHost, exists := GetEnv("DOCKER_HOST")
	if exists {
		// return nil if set to empty string
		if dockerHost == "" {
			return nil
		}
	} else {
		dockerHost = getDockerHost()
	}

	parsedURL, err := url.Parse(dockerHost)
	if err != nil {
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
	timeout := time.Millisecond * time.Duration(dockerTimeoutMs)
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

	// Read container exclusion patterns from environment variable
	var excludeContainers []string
	if excludeStr, set := GetEnv("EXCLUDE_CONTAINERS"); set && excludeStr != "" {
		parts := strings.SplitSeq(excludeStr, ",")
		for part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				excludeContainers = append(excludeContainers, trimmed)
			}
		}
		slog.Info("EXCLUDE_CONTAINERS", "patterns", excludeContainers)
	}

	manager := &dockerManager{
		client: &http.Client{
			Timeout:   timeout,
			Transport: userAgentTransport,
		},
		containerStatsMap: make(map[string]*container.Stats),
		sem:               make(chan struct{}, 5),
		apiContainerList:  []*container.ApiInfo{},
		apiStats:          &container.ApiStats{},
		excludeContainers: excludeContainers,

		// Initialize cache-time-aware tracking structures
		lastCpuContainer:    make(map[uint16]map[string]uint64),
		lastCpuSystem:       make(map[uint16]map[string]uint64),
		lastCpuReadTime:     make(map[uint16]map[string]time.Time),
		networkSentTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
		networkRecvTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	// If using podman, return client
	if strings.Contains(dockerHost, "podman") {
		a.systemInfo.Podman = true
		manager.goodDockerVersion = true
		return manager
	}

	// this can take up to 5 seconds with retry, so run in goroutine
	go manager.checkDockerVersion()

	// give version check a chance to complete before returning
	time.Sleep(50 * time.Millisecond)

	return manager
}

// checkDockerVersion checks Docker version and sets goodDockerVersion if at least 25.0.0.
// Versions before 25.0.0 have a bug with one-shot which requires all requests to be made in one batch.
func (dm *dockerManager) checkDockerVersion() {
	var err error
	var resp *http.Response
	var versionInfo struct {
		Version string `json:"Version"`
	}
	const versionMaxTries = 2
	for i := 1; i <= versionMaxTries; i++ {
		resp, err = dm.client.Get("http://localhost/version")
		if err == nil {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if i < versionMaxTries {
			slog.Debug("Failed to get Docker version; retrying", "attempt", i, "error", err)
			time.Sleep(5 * time.Second)
		}
	}
	if err != nil {
		return
	}
	if err := dm.decode(resp, &versionInfo); err != nil {
		return
	}
	// if version > 24, one-shot works correctly and we can limit concurrent operations
	if dockerVersion, err := semver.Parse(versionInfo.Version); err == nil && dockerVersion.Major > 24 {
		dm.goodDockerVersion = true
	} else {
		slog.Info(fmt.Sprintf("Docker %s is outdated. Upgrade if possible. See https://github.com/henrygd/beszel/issues/58", versionInfo.Version))
	}
}

// Decodes Docker API JSON response using a reusable buffer and decoder. Not thread safe.
func (dm *dockerManager) decode(resp *http.Response, d any) error {
	if dm.buf == nil {
		// initialize buffer with 256kb starting size
		dm.buf = bytes.NewBuffer(make([]byte, 0, 1024*256))
		dm.decoder = json.NewDecoder(dm.buf)
	}
	defer resp.Body.Close()
	defer dm.buf.Reset()
	_, err := dm.buf.ReadFrom(resp.Body)
	if err != nil {
		return err
	}
	return dm.decoder.Decode(d)
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

// getContainerInfo fetches the inspection data for a container
func (dm *dockerManager) getContainerInfo(ctx context.Context, containerID string) ([]byte, error) {
	endpoint := fmt.Sprintf("http://localhost/containers/%s/json", containerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := dm.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("container info request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	// Remove sensitive environment variables from Config.Env
	var containerInfo map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&containerInfo); err != nil {
		return nil, err
	}
	if config, ok := containerInfo["Config"].(map[string]any); ok {
		delete(config, "Env")
	}

	return json.Marshal(containerInfo)
}

// getLogs fetches the logs for a container
func (dm *dockerManager) getLogs(ctx context.Context, containerID string) (string, error) {
	endpoint := fmt.Sprintf("http://localhost/containers/%s/logs?stdout=1&stderr=1&tail=%d", containerID, dockerLogsTail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := dm.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("logs request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var builder strings.Builder
	if err := decodeDockerLogStream(resp.Body, &builder); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func decodeDockerLogStream(reader io.Reader, builder *strings.Builder) error {
	const headerSize = 8
	var header [headerSize]byte
	buf := make([]byte, 0, dockerLogsTail*200)
	totalBytesRead := 0

	for {
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}

		frameLen := binary.BigEndian.Uint32(header[4:])
		if frameLen == 0 {
			continue
		}

		// Prevent memory exhaustion from excessively large frames
		if frameLen > maxLogFrameSize {
			return fmt.Errorf("log frame size (%d) exceeds maximum (%d)", frameLen, maxLogFrameSize)
		}

		// Check if reading this frame would exceed total log size limit
		if totalBytesRead+int(frameLen) > maxTotalLogSize {
			// Read and discard remaining data to avoid blocking
			_, _ = io.Copy(io.Discard, io.LimitReader(reader, int64(frameLen)))
			slog.Debug("Truncating logs: limit reached", "read", totalBytesRead, "limit", maxTotalLogSize)
			return nil
		}

		buf = allocateBuffer(buf, int(frameLen))
		if _, err := io.ReadFull(reader, buf[:frameLen]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				if len(buf) > 0 {
					builder.Write(buf[:min(int(frameLen), len(buf))])
				}
				return nil
			}
			return err
		}
		builder.Write(buf[:frameLen])
		totalBytesRead += int(frameLen)
	}
}

func allocateBuffer(current []byte, needed int) []byte {
	if cap(current) >= needed {
		return current[:needed]
	}
	return make([]byte, needed)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
