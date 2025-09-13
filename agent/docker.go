package agent

import (
	"bytes"
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

	"github.com/henrygd/beszel/internal/entities/container"

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
	buf                 *bytes.Buffer               // Buffer to store and read response bodies
	decoder             *json.Decoder               // Reusable JSON decoder that reads from buf
	apiStats            *container.ApiStats         // Reusable API stats object
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

// Returns stats for all running containers
func (dm *dockerManager) getDockerStats() ([]*container.Stats, error) {
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

	for i := range dm.apiContainerList {
		ctr := dm.apiContainerList[i]
		ctr.IdShort = ctr.Id[:12]
		dm.validIds[ctr.IdShort] = struct{}{}
		// check if container is less than 1 minute old (possible restart)
		// note: can't use Created field because it's not updated on restart
		if strings.Contains(ctr.Status, "second") {
			// if so, remove old container data
			dm.deleteContainerStatsSync(ctr.IdShort)
		}
		dm.queue()
		go func() {
			defer dm.dequeue()
			err := dm.updateContainerStats(ctr)
			// if error, delete from map and add to failed list to retry
			if err != nil {
				dm.containerStatsMutex.Lock()
				delete(dm.containerStatsMap, ctr.IdShort)
				failedContainers = append(failedContainers, ctr)
				dm.containerStatsMutex.Unlock()
			}
		}()
	}

	dm.wg.Wait()

	// retry failed containers separately so we can run them in parallel (docker 24 bug)
	if len(failedContainers) > 0 {
		slog.Debug("Retrying failed containers", "count", len(failedContainers))
		for i := range failedContainers {
			ctr := failedContainers[i]
			dm.queue()
			go func() {
				defer dm.dequeue()
				err = dm.updateContainerStats(ctr)
				if err != nil {
					slog.Error("Error getting container stats", "err", err)
				}
			}()
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

	return stats, nil
}

// Updates stats for individual container
func (dm *dockerManager) updateContainerStats(ctr *container.ApiInfo) error {
	name := ctr.Names[0][1:]

	resp, err := dm.client.Get("http://localhost/containers/" + ctr.IdShort + "/stats?stream=0&one-shot=1")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	dm.containerStatsMutex.Lock()
	defer dm.containerStatsMutex.Unlock()

	// add empty values if they doesn't exist in map
	stats, initialized := dm.containerStatsMap[ctr.IdShort]
	if !initialized {
		stats = &container.Stats{Name: name}
		dm.containerStatsMap[ctr.IdShort] = stats
	}

	// reset current stats
	stats.Cpu = 0
	stats.Mem = 0
	stats.NetworkSent = 0
	stats.NetworkRecv = 0

	// docker host container stats response
	// res := dm.getApiStats()
	// defer dm.putApiStats(res)
	//

	res := dm.apiStats
	res.Networks = nil
	if err := dm.decode(resp, res); err != nil {
		return err
	}

	// calculate cpu and memory stats
	var usedMemory uint64
	var cpuPct float64

	// store current cpu stats
	prevCpuContainer, prevCpuSystem := stats.CpuContainer, stats.CpuSystem
	stats.CpuContainer = res.CPUStats.CPUUsage.TotalUsage
	stats.CpuSystem = res.CPUStats.SystemUsage

	if dm.isWindows {
		usedMemory = res.MemoryStats.PrivateWorkingSet
		cpuPct = res.CalculateCpuPercentWindows(prevCpuContainer, stats.PrevReadTime)
	} else {
		// check if container has valid data, otherwise may be in restart loop (#103)
		if res.MemoryStats.Usage == 0 {
			return fmt.Errorf("%s - no memory stats - see https://github.com/henrygd/beszel/issues/144", name)
		}
		memCache := res.MemoryStats.Stats.InactiveFile
		if memCache == 0 {
			memCache = res.MemoryStats.Stats.Cache
		}
		usedMemory = res.MemoryStats.Usage - memCache

		cpuPct = res.CalculateCpuPercentLinux(prevCpuContainer, prevCpuSystem)
	}

	if cpuPct > 100 {
		return fmt.Errorf("%s cpu pct greater than 100: %+v", name, cpuPct)
	}

	// network
	var total_sent, total_recv uint64
	for _, v := range res.Networks {
		total_sent += v.TxBytes
		total_recv += v.RxBytes
	}
	var sent_delta, recv_delta uint64
	millisecondsElapsed := uint64(time.Since(stats.PrevReadTime).Milliseconds())
	if initialized && millisecondsElapsed > 0 {
		// get bytes per second
		sent_delta = (total_sent - stats.PrevNet.Sent) * 1000 / millisecondsElapsed
		recv_delta = (total_recv - stats.PrevNet.Recv) * 1000 / millisecondsElapsed
		// check for unrealistic network values (> 5GB/s)
		if sent_delta > 5e9 || recv_delta > 5e9 {
			slog.Warn("Bad network delta", "container", name)
			sent_delta, recv_delta = 0, 0
		}
	}
	stats.PrevNet.Sent, stats.PrevNet.Recv = total_sent, total_recv

	stats.Cpu = twoDecimals(cpuPct)
	stats.Mem = bytesToMegabytes(float64(usedMemory))
	stats.NetworkSent = bytesToMegabytes(float64(sent_delta))
	stats.NetworkRecv = bytesToMegabytes(float64(recv_delta))
	stats.PrevReadTime = res.Read

	return nil
}

// Delete container stats from map using mutex
func (dm *dockerManager) deleteContainerStatsSync(id string) {
	dm.containerStatsMutex.Lock()
	defer dm.containerStatsMutex.Unlock()
	delete(dm.containerStatsMap, id)
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
		apiStats:          &container.ApiStats{},
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

	if err := manager.decode(resp, &versionInfo); err != nil {
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
