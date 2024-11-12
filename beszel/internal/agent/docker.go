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
	apiContainerList    *[]container.ApiInfo        // List of containers from Docker API
	containerStatsMap   map[string]*container.Stats // Keeps track of container stats
	validIds            map[string]struct{}         // Map of valid container ids, used to prune invalid containers from containerStatsMap
	goodDockerVersion   bool                        // Whether docker version is at least 25.0.0 (one-shot works correctly)
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
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&dm.apiContainerList); err != nil {
		return nil, err
	}

	containersLength := len(*dm.apiContainerList)

	// store valid ids to clean up old container ids from map
	if dm.validIds == nil {
		dm.validIds = make(map[string]struct{}, containersLength)
	} else {
		clear(dm.validIds)
	}

	var failedContainters []container.ApiInfo

	for _, ctr := range *dm.apiContainerList {
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
				failedContainters = append(failedContainters, ctr)
				dm.containerStatsMutex.Unlock()
			}
		}()
	}

	dm.wg.Wait()

	// retry failed containers separately so we can run them in parallel (docker 24 bug)
	if len(failedContainters) > 0 {
		slog.Debug("Retrying failed containers", "count", len(failedContainters))
		for _, ctr := range failedContainters {
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
func (dm *dockerManager) updateContainerStats(ctr container.ApiInfo) error {
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
	var res container.ApiStats
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return err
	}

	// check if container has valid data, otherwise may be in restart loop (#103)
	if res.MemoryStats.Usage == 0 {
		return fmt.Errorf("%s - no memory stats - see https://github.com/henrygd/beszel/issues/144", name)
	}

	// memory (https://docs.docker.com/reference/cli/docker/container/stats/)
	memCache := res.MemoryStats.Stats.InactiveFile
	if memCache == 0 {
		memCache = res.MemoryStats.Stats.Cache
	}
	usedMemory := res.MemoryStats.Usage - memCache

	// cpu
	cpuDelta := res.CPUStats.CPUUsage.TotalUsage - stats.PrevCpu[0]
	systemDelta := res.CPUStats.SystemUsage - stats.PrevCpu[1]
	cpuPct := float64(cpuDelta) / float64(systemDelta) * 100
	if cpuPct > 100 {
		return fmt.Errorf("%s cpu pct greater than 100: %+v", name, cpuPct)
	}
	stats.PrevCpu = [2]uint64{res.CPUStats.CPUUsage.TotalUsage, res.CPUStats.SystemUsage}

	// network
	var total_sent, total_recv uint64
	for _, v := range res.Networks {
		total_sent += v.TxBytes
		total_recv += v.RxBytes
	}
	var sent_delta, recv_delta float64
	// prevent first run from sending all prev sent/recv bytes
	if initialized {
		secondsElapsed := time.Since(stats.PrevNet.Time).Seconds()
		sent_delta = float64(total_sent-stats.PrevNet.Sent) / secondsElapsed
		recv_delta = float64(total_recv-stats.PrevNet.Recv) / secondsElapsed
	}
	stats.PrevNet.Sent = total_sent
	stats.PrevNet.Recv = total_recv
	stats.PrevNet.Time = time.Now()

	stats.Cpu = twoDecimals(cpuPct)
	stats.Mem = bytesToMegabytes(float64(usedMemory))
	stats.NetworkSent = bytesToMegabytes(sent_delta)
	stats.NetworkRecv = bytesToMegabytes(recv_delta)

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
	dockerHost, exists := os.LookupEnv("DOCKER_HOST")
	if exists {
		slog.Info("DOCKER_HOST", "host", dockerHost)
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
	if t, set := os.LookupEnv("DOCKER_TIMEOUT"); set {
		timeout, err = time.ParseDuration(t)
		if err != nil {
			slog.Error(err.Error())
			os.Exit(1)
		}
		slog.Info("DOCKER_TIMEOUT", "timeout", timeout)
	}

	dockerClient := &dockerManager{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		containerStatsMap: make(map[string]*container.Stats),
		sem:               make(chan struct{}, 5),
	}

	// If using podman, return client
	if strings.Contains(dockerHost, "podman") {
		a.systemInfo.Podman = true
		dockerClient.goodDockerVersion = true
		return dockerClient
	}

	// Check docker version
	// (versions before 25.0.0 have a bug with one-shot which requires all requests to be made in one batch)
	var versionInfo struct {
		Version string `json:"Version"`
	}
	resp, err := dockerClient.client.Get("http://localhost/version")
	if err != nil {
		return dockerClient
	}

	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return dockerClient
	}

	// if version > 24, one-shot works correctly and we can limit concurrent operations
	if dockerVersion, err := semver.Parse(versionInfo.Version); err == nil && dockerVersion.Major > 24 {
		dockerClient.goodDockerVersion = true
	} else {
		slog.Info(fmt.Sprintf("Docker %s is outdated. Upgrade if possible. See https://github.com/henrygd/beszel/issues/58", versionInfo.Version))
	}

	return dockerClient
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
