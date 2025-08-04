package tailscale

import (
	"beszel/internal/entities/tailscale"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	tsclient "tailscale.com/client/tailscale/v2"
)

// Manager handles Tailscale API interactions and data management
type Manager struct {
	hub       core.App
	client    *tsclient.Client
	config    *tailscale.TailscaleConfig
	network   *tailscale.TailscaleNetwork
	stats     *tailscale.TailscaleStats
	mutex     sync.RWMutex
	lastFetch time.Time
}

// NewManager creates a new Tailscale manager instance
func NewManager(hub core.App) *Manager {
	return &Manager{
		hub: hub,
	}
}

// Initialize sets up the Tailscale manager with configuration
func (m *Manager) Initialize() error {
	// Load configuration from environment or database
	config, err := m.loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load Tailscale config: %w", err)
	}

	if !config.Enabled {
		slog.Info("Tailscale monitoring is disabled")
		return nil
	}

	m.config = config

	// Initialize Tailscale client with appropriate authentication
	var client *tsclient.Client

	if config.APIKey != "" {
		// Use API key authentication
		slog.Info("Using API key authentication for Tailscale")
		client = &tsclient.Client{
			APIKey:  config.APIKey,
			Tailnet: config.Tailnet,
		}
	} else {
		// Use OAuth2 authentication
		slog.Info("Using OAuth2 authentication for Tailscale")
		oauthConfig := tsclient.OAuthConfig{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			Scopes:       []string{"read:devices", "read:tailnet"},
		}

		httpClient := oauthConfig.HTTPClient()
		client = &tsclient.Client{
			HTTP:    httpClient,
			Tailnet: config.Tailnet,
		}
	}

	m.client = client

	// Perform initial fetch
	if err := m.FetchNetworkData(); err != nil {
		slog.Warn("Failed to perform initial Tailscale data fetch", "error", err)
	}

	slog.Info("Tailscale manager initialized", "tailnet", config.Tailnet)
	return nil
}

// loadConfig loads Tailscale configuration from environment variables
func (m *Manager) loadConfig() (*tailscale.TailscaleConfig, error) {
	config := &tailscale.TailscaleConfig{}

	// Check if Tailscale monitoring is enabled
	if enabled := os.Getenv("TS_ENABLE"); enabled == "true" {
		config.Enabled = true
	} else {
		config.Enabled = false
		return config, nil
	}

	// Load tailnet name
	if tailnet := os.Getenv("TS_TAILNET"); tailnet != "" {
		config.Tailnet = tailnet
	} else {
		return nil, fmt.Errorf("TS_TAILNET environment variable is required")
	}

	// Load API key (optional)
	config.APIKey = os.Getenv("TS_API_KEY")

	// Load OAuth2 credentials (optional)
	config.ClientID = os.Getenv("TS_CLIENT_ID")
	config.ClientSecret = os.Getenv("TS_CLIENT_SECRET")

	// Check that at least one authentication method is provided
	if config.APIKey == "" && (config.ClientID == "" || config.ClientSecret == "") {
		return nil, fmt.Errorf("either TS_API_KEY or both TS_CLIENT_ID and TS_CLIENT_SECRET environment variables are required")
	}

	// If both are provided, prefer OAuth2
	if config.APIKey != "" && config.ClientID != "" && config.ClientSecret != "" {
		slog.Info("Both API key and OAuth2 credentials provided, using OAuth2")
		config.APIKey = "" // Clear API key to use OAuth2
	}

	return config, nil
}

// FetchNetworkData retrieves the current Tailscale network state
func (m *Manager) FetchNetworkData() error {

	if m.client == nil {
		return fmt.Errorf("Tailscale client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get devices (nodes) from Tailscale API with all fields including latency
	devices, err := m.client.Devices().ListWithAllFields(ctx)
	if err != nil {
		slog.Error("Failed to fetch devices from Tailscale API", "error", err)
		return fmt.Errorf("failed to fetch devices: %w", err)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Convert devices to our internal format
	nodes := make([]*tailscale.TailscaleNode, 0, len(devices))
	stats := &tailscale.TailscaleStats{
		LastUpdated: time.Now(),
	}

	for _, device := range devices {
		node := m.convertDeviceToNode(&device)
		nodes = append(nodes, node)

		// Update statistics
		stats.TotalNodes++
		if node.Online {
			stats.OnlineNodes++
		} else {
			stats.OfflineNodes++
		}
		if node.Expired {
			stats.ExpiredNodes++
		}
		if node.IsExitNode {
			stats.ExitNodes++
		}
		if node.IsSubnetRouter {
			stats.SubnetRouters++
		}
		if node.IsEphemeral {
			stats.EphemeralNodes++
		}
		if node.UpdateAvailable {
			stats.NodesWithUpdates++
		}
	}

	// Update network data
	m.network = &tailscale.TailscaleNetwork{
		Domain:        m.config.Tailnet,
		TailnetName:   m.config.Tailnet,
		TotalNodes:    stats.TotalNodes,
		OnlineNodes:   stats.OnlineNodes,
		OfflineNodes:  stats.OfflineNodes,
		ExpiredNodes:  stats.ExpiredNodes,
		ExitNodes:     stats.ExitNodes,
		SubnetRouters: stats.SubnetRouters,
		Nodes:         nodes,
		LastUpdated:   time.Now(),
	}

	m.stats = stats
	m.lastFetch = time.Now()

	// Store aggregated stats in database (disabled - no longer storing in tailscale_summary)
	// if err := m.storeNetworkData(); err != nil {
	// 	slog.Warn("Failed to store Tailscale stats data", "error", err)
	// }

	// Store detailed network data (one record per node)
	if err := m.storeDetailedNetworkData(devices); err != nil {
		slog.Warn("Failed to store Tailscale detailed network data", "error", err)
	}

	slog.Info("Tailscale network data updated",
		"totalNodes", stats.TotalNodes,
		"onlineNodes", stats.OnlineNodes,
		"offlineNodes", stats.OfflineNodes,
		"expiredNodes", stats.ExpiredNodes,
		"exitNodes", stats.ExitNodes,
		"subnetRouters", stats.SubnetRouters)

	return nil
}

// storeNetworkData stores the aggregated stats data in the database
func (m *Manager) storeNetworkData() error {
	if m.network == nil {
		return nil
	}

	// Get the tailscale collection (for storing aggregated stats)
	collection, err := m.hub.FindCollectionByNameOrId("tailscale_summary")
	if err != nil {
		slog.Warn("Tailscale collection not found, skipping storage", "error", err)
		return nil
	}

	// Convert stats data to JSON (we only store stats in this collection)
	statsData, err := json.Marshal(m.stats)
	if err != nil {
		return fmt.Errorf("failed to marshal stats data: %w", err)
	}

	// Create a new record
	record := core.NewRecord(collection)
	record.Set("tailnet", m.config.Tailnet)
	record.Set("stats_data", string(statsData))

	// Save the record to the database
	if err := m.hub.Save(record); err != nil {
		return fmt.Errorf("failed to save record: %w", err)
	}

	slog.Info("Tailscale stats data saved to database",
		"tailnet", m.config.Tailnet,
		"totalNodes", m.stats.TotalNodes,
		"onlineNodes", m.stats.OnlineNodes,
		"recordId", record.Id)

	return nil
}

// storeDetailedNetworkData stores individual node information in the tailscales_stats collection
func (m *Manager) storeDetailedNetworkData(devices []tsclient.Device) error {
	if m.network == nil || len(m.network.Nodes) == 0 {
		return nil
	}

	// Get the tailscale_stats collection (for storing individual node data)
	collection, err := m.hub.FindCollectionByNameOrId("tailscale_detailed")
	if err != nil {
		slog.Warn("Tailscale stats collection not found, skipping detailed storage", "error", err)
		return nil
	}

	// Create a map of device ID to device for quick lookup
	deviceMap := make(map[string]*tsclient.Device)
	for i := range devices {
		deviceMap[devices[i].ID] = &devices[i]
	}

	// Save one record per node
	for _, node := range m.network.Nodes {
		// Get endpoints from network data if available
		if device, exists := deviceMap[node.ID]; exists && device.ClientConnectivity != nil {
			node.Endpoints = device.ClientConnectivity.Endpoints
		}
		
		// Convert node data to JSON for the info field
		nodeData, err := json.Marshal(node)
		if err != nil {
			slog.Warn("Failed to marshal node data", "node", node.Name, "error", err)
			continue
		}

		// Create a new record for this node
		record := core.NewRecord(collection)
		record.Set("tailnet", m.config.Tailnet)
		record.Set("node_id", node.ID)
		
		// Store network connectivity information in the network field
		var networkData map[string]interface{}
		if device, exists := deviceMap[node.ID]; exists && device.ClientConnectivity != nil {
			networkData = map[string]interface{}{
				"endpoints": device.ClientConnectivity.Endpoints,
				"derp":      device.ClientConnectivity.DERP,
				"latency":   device.ClientConnectivity.DERPLatency,
			}
		} else {
			// Skip this record if connectivity data is not available
			continue
		}
		networkJSON, _ := json.Marshal(networkData)
		record.Set("network", string(networkJSON))
		
		record.Set("info", string(nodeData))

		// Save the record to the database
		if err := m.hub.Save(record); err != nil {
			slog.Warn("Failed to save node record", "node", node.Name, "error", err)
			continue
		}

		slog.Debug("Tailscale node data saved to database",
			"tailnet", m.config.Tailnet,
			"nodeName", node.Name,
			"nodeID", node.ID,
			"online", node.Online,
			"recordId", record.Id)
	}

	slog.Info("Tailscale node data saved to database",
		"tailnet", m.config.Tailnet,
		"totalNodes", len(m.network.Nodes))

	return nil
}

// convertDeviceToNode converts a Tailscale device to our internal node format
func (m *Manager) convertDeviceToNode(device *tsclient.Device) *tailscale.TailscaleNode {
	// Extract IP addresses from the addresses slice
	var ip, ipv6 string
	if len(device.Addresses) > 0 {
		ip = device.Addresses[0] // First address is typically IPv4
		if len(device.Addresses) > 1 {
			ipv6 = device.Addresses[1] // Second address is typically IPv6
		}
	}

	// Determine if device is online based on last seen time
	online := !device.LastSeen.IsZero() && time.Since(device.LastSeen.Time) < 5*time.Minute

	node := &tailscale.TailscaleNode{
		ID:             device.ID,
		Name:           device.Name,
		Hostname:       device.Hostname,
		IP:             ip,
		IPv6:           ipv6,
		OS:             device.OS,
		Version:        device.ClientVersion,
		LastSeen:       device.LastSeen.Time,
		Online:         online,
		Tags:           device.Tags,
		IsExitNode:     false, // Not available in basic device info
		IsSubnetRouter: false, // Not available in basic device info
		KeyExpiry:      device.Expires.Time,
		AdvertisedRoutes: device.AdvertisedRoutes,
		EnabledRoutes:    device.EnabledRoutes,
		Endpoints:      []string{}, // Will be populated from network data
	}

	return node
}

// GetNetworkData returns the current network data
func (m *Manager) GetNetworkData() *tailscale.TailscaleNetwork {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.network
}

// GetStats returns the current network statistics
func (m *Manager) GetStats() *tailscale.TailscaleStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.stats
}

// GetLastFetchTime returns when the data was last fetched
func (m *Manager) GetLastFetchTime() time.Time {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.lastFetch
}

// IsEnabled returns whether Tailscale monitoring is enabled
func (m *Manager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}
