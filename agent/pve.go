package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"

	"github.com/luthermonson/go-proxmox"
)

type pveManager struct {
	client       *proxmox.Client                    // Client to query PVE API
	nodeName     string                             // Cluster node name
	cpuCount     int                                // CPU count on node
	nodeStatsMap map[string]*container.PveNodeStats // Keeps track of pve node stats
	lastInitTry  time.Time                          // Last time node initialization was attempted
}

// newPVEManager creates a new PVE manager - may return nil if required environment variables
// are not set or if there is an error connecting to the API
func newPVEManager() *pveManager {
	url, exists := GetEnv("PROXMOX_URL")
	if !exists {
		url = "https://localhost:8006/api2/json"
	}
	const nodeEnvVar = "PROXMOX_NODE"
	const tokenIDEnvVar = "PROXMOX_TOKENID"
	const secretEnvVar = "PROXMOX_SECRET"

	nodeName, nodeNameExists := GetEnv(nodeEnvVar)
	tokenID, tokenIDExists := GetEnv(tokenIDEnvVar)
	secret, secretExists := GetEnv(secretEnvVar)

	if !nodeNameExists || !tokenIDExists || !secretExists {
		slog.Debug("Proxmox env vars unset", nodeEnvVar, nodeNameExists, tokenIDEnvVar, tokenIDExists, secretEnvVar, secretExists)
		return nil
	}

	// PROXMOX_INSECURE_TLS defaults to true; set to "false" to enable TLS verification
	insecureTLS := true
	if val, exists := GetEnv("PROXMOX_INSECURE_TLS"); exists {
		insecureTLS = val != "false"
	}

	httpClient := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureTLS,
			},
		},
	}
	client := proxmox.NewClient(url,
		proxmox.WithHTTPClient(&httpClient),
		proxmox.WithAPIToken(tokenID, secret),
	)

	pveManager := pveManager{
		client:       client,
		nodeName:     nodeName,
		nodeStatsMap: make(map[string]*container.PveNodeStats),
	}

	return &pveManager
}

// ensureInitialized checks if the PVE manager is initialized and attempts to initialize it if not.
// It returns an error if initialization fails or if a retry is pending.
func (pm *pveManager) ensureInitialized(ctx context.Context) error {
	if pm.client == nil {
		return errors.New("PVE client not configured")
	}
	if pm.cpuCount > 0 {
		return nil
	}

	if time.Since(pm.lastInitTry) < 30*time.Second {
		return errors.New("PVE initialization retry pending")
	}
	pm.lastInitTry = time.Now()

	node, err := pm.client.Node(ctx, pm.nodeName)
	if err != nil {
		return err
	}
	if node.CPUInfo.CPUs <= 0 {
		return errors.New("node returned zero CPUs")
	}

	pm.cpuCount = node.CPUInfo.CPUs
	return nil
}

// getPVEStats returns stats for all running VMs/LXCs
func (pm *pveManager) getPVEStats() ([]*container.PveNodeStats, error) {
	if err := pm.ensureInitialized(context.Background()); err != nil {
		slog.Warn("Proxmox API unavailable", "err", err)
		return nil, err
	}
	cluster, err := pm.client.Cluster(context.Background())
	if err != nil {
		slog.Error("Error getting cluster", "err", err)
		return nil, err
	}
	resources, err := cluster.Resources(context.Background(), "vm")
	if err != nil {
		slog.Error("Error getting resources", "err", err, "resources", resources)
		return nil, err
	}
	containersLength := len(resources)
	resourceIds := make(map[string]struct{}, containersLength)

	// only include running vms and lxcs on selected node
	for _, resource := range resources {
		if resource.Node == pm.nodeName && resource.Status == "running" {
			resourceIds[resource.ID] = struct{}{}
		}
	}
	// remove invalid container stats
	for id := range pm.nodeStatsMap {
		if _, exists := resourceIds[id]; !exists {
			delete(pm.nodeStatsMap, id)
		}
	}

	// populate stats
	stats := make([]*container.PveNodeStats, 0, len(resourceIds))
	for _, resource := range resources {
		if _, exists := resourceIds[resource.ID]; !exists {
			continue
		}
		resourceStats, initialized := pm.nodeStatsMap[resource.ID]
		if !initialized {
			resourceStats = &container.PveNodeStats{}
			pm.nodeStatsMap[resource.ID] = resourceStats
		}
		resourceStats.Name = resource.Name
		resourceStats.Id = resource.ID
		resourceStats.Type = resource.Type
		resourceStats.MaxCPU = resource.MaxCPU
		resourceStats.MaxMem = resource.MaxMem
		resourceStats.Uptime = resource.Uptime
		resourceStats.DiskRead = resource.DiskRead
		resourceStats.DiskWrite = resource.DiskWrite
		resourceStats.Disk = resource.MaxDisk

		// prevent first run from sending all prev sent/recv bytes
		total_sent := resource.NetOut
		total_recv := resource.NetIn
		var sent_delta, recv_delta float64
		if initialized {
			secondsElapsed := time.Since(resourceStats.PrevReadTime).Seconds()
			if secondsElapsed > 0 {
				sent_delta = float64(total_sent-resourceStats.PrevNet.Sent) / secondsElapsed
				recv_delta = float64(total_recv-resourceStats.PrevNet.Recv) / secondsElapsed
			}
		}
		resourceStats.PrevNet.Sent = total_sent
		resourceStats.PrevNet.Recv = total_recv
		resourceStats.PrevReadTime = time.Now()

		// Update final stats values
		resourceStats.Cpu = twoDecimals(100.0 * resource.CPU * float64(resource.MaxCPU) / float64(pm.cpuCount))
		resourceStats.Mem = bytesToMegabytes(float64(resource.Mem))
		resourceStats.Bandwidth = [2]uint64{uint64(sent_delta), uint64(recv_delta)}
		resourceStats.NetOut = total_sent
		resourceStats.NetIn = total_recv

		stats = append(stats, resourceStats)
	}

	return stats, nil
}
