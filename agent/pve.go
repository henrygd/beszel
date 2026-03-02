package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"time"

	"github.com/henrygd/beszel/internal/entities/container"

	"github.com/luthermonson/go-proxmox"
)

type pveManager struct {
	client            *proxmox.Client             // Client to query PVE API
	nodeName          string                      // Cluster node name
	cpuCount          int                         // CPU count on node
	containerStatsMap map[string]*container.Stats // Keeps track of container stats
}

// Returns stats for all running VMs/LXCs
func (pm *pveManager) getPVEStats() ([]*container.Stats, error) {
	if pm.client == nil {
		return nil, errors.New("PVE client not configured")
	}
	cluster, err := pm.client.Cluster(context.Background())
	if err != nil {
		return nil, err
	}
	resources, err := cluster.Resources(context.Background(), "vm")
	if err != nil {
		return nil, err
	}

	containersLength := len(resources)

	containerIds := make(map[string]struct{}, containersLength)

	// only include running vms and lxcs on selected node
	for _, resource := range resources {
		if resource.Node == pm.nodeName && resource.Status == "running" {
			containerIds[resource.ID] = struct{}{}
		}
	}
	// remove invalid container stats
	for id := range pm.containerStatsMap {
		if _, exists := containerIds[id]; !exists {
			delete(pm.containerStatsMap, id)
		}
	}

	// populate stats
	stats := make([]*container.Stats, 0, len(containerIds))
	for _, resource := range resources {
		if _, exists := containerIds[resource.ID]; !exists {
			continue
		}
		resourceStats, initialized := pm.containerStatsMap[resource.ID]
		if !initialized {
			resourceStats = &container.Stats{}
			pm.containerStatsMap[resource.ID] = resourceStats
		}
		// reset current stats
		resourceStats.Cpu = 0
		resourceStats.Mem = 0
		resourceStats.Bandwidth = [2]uint64{0, 0}
		// Store clean name (no type suffix)
		resourceStats.Name = resource.Name
		// Store resource ID (e.g. "qemu/100") in .Id (cbor key 7, json:"-")
		resourceStats.Id = resource.ID
		// Store type (e.g. "qemu" or "lxc") in .Image (cbor key 8, json:"-")
		resourceStats.Image = resource.Type
		// prevent first run from sending all prev sent/recv bytes
		total_sent := uint64(resource.NetOut)
		total_recv := uint64(resource.NetIn)
		var sent_delta, recv_delta float64
		if initialized {
			secondsElapsed := time.Since(resourceStats.PrevReadTime).Seconds()
			sent_delta = float64(total_sent-resourceStats.PrevNet.Sent) / secondsElapsed
			recv_delta = float64(total_recv-resourceStats.PrevNet.Recv) / secondsElapsed
		}
		resourceStats.PrevReadTime = time.Now()

		// Update final stats values
		resourceStats.Cpu = twoDecimals(100.0 * resource.CPU * float64(resource.MaxCPU) / float64(pm.cpuCount))
		resourceStats.Mem = float64(resource.Mem)
		resourceStats.Bandwidth = [2]uint64{uint64(sent_delta), uint64(recv_delta)}

		stats = append(stats, resourceStats)
	}

	return stats, nil
}

// Creates a new PVE manager
func newPVEManager(_ *Agent) *pveManager {
	url, exists := GetEnv("PROXMOX_URL")
	if !exists {
		url = "https://localhost:8006/api2/json"
	}
	nodeName, nodeNameExists := GetEnv("PROXMOX_NODE")
	tokenID, tokenIDExists := GetEnv("PROXMOX_TOKENID")
	secret, secretExists := GetEnv("PROXMOX_SECRET")

	// PROXMOX_INSECURE_TLS defaults to true; set to "false" to enable TLS verification
	insecureTLS := true
	if val, exists := GetEnv("PROXMOX_INSECURE_TLS"); exists {
		insecureTLS = val != "false"
	}

	var client *proxmox.Client
	if nodeNameExists && tokenIDExists && secretExists {
		httpClient := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: insecureTLS,
				},
			},
		}
		client = proxmox.NewClient(url,
			proxmox.WithHTTPClient(&httpClient),
			proxmox.WithAPIToken(tokenID, secret),
		)
	} else {
		client = nil
	}

	pveManager := &pveManager{
		client:            client,
		nodeName:          nodeName,
		containerStatsMap: make(map[string]*container.Stats),
	}
	// Retrieve node cpu count
	if client != nil {
		node, err := client.Node(context.Background(), nodeName)
		if err != nil {
			pveManager.client = nil
		} else {
			pveManager.cpuCount = node.CPUInfo.CPUs
		}
	}

	return pveManager
}
