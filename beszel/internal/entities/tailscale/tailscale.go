package tailscale

import (
	"time"
)

// TailscaleNode represents a node in the Tailscale network
type TailscaleNode struct {
	ID                        string                 `json:"id" cbor:"0,keyasint"`
	NodeID                    string                 `json:"nodeId" cbor:"1,keyasint"`
	Name                      string                 `json:"name" cbor:"2,keyasint"`
	Hostname                  string                 `json:"hostname" cbor:"3,keyasint"`
	Addresses                 []string               `json:"addresses" cbor:"4,keyasint"`
	User                      string                 `json:"user" cbor:"5,keyasint"`
	OS                        string                 `json:"os" cbor:"6,keyasint"`
	Version                   string                 `json:"version" cbor:"7,keyasint"`
	Created                   time.Time              `json:"created" cbor:"8,keyasint"`
	LastSeen                  time.Time              `json:"lastSeen" cbor:"9,keyasint"`
	Online                    bool                   `json:"online" cbor:"10,keyasint"`
	KeyExpiry                 time.Time              `json:"keyExpiry" cbor:"11,keyasint"`
	KeyExpiryDisabled         bool                   `json:"keyExpiryDisabled" cbor:"12,keyasint"`
	Authorized                bool                   `json:"authorized" cbor:"13,keyasint"`
	IsExternal                bool                   `json:"isExternal" cbor:"14,keyasint"`
	UpdateAvailable           bool                   `json:"updateAvailable" cbor:"15,keyasint"`
	BlocksIncomingConnections bool                   `json:"blocksIncomingConnections" cbor:"16,keyasint"`
	MachineKey                string                 `json:"machineKey" cbor:"17,keyasint"`
	NodeKey                   string                 `json:"nodeKey" cbor:"18,keyasint"`
	TailnetLockKey            string                 `json:"tailnetLockKey" cbor:"19,keyasint"`
	TailnetLockError          string                 `json:"tailnetLockError,omitempty" cbor:"20,keyasint,omitempty"`
	Tags                      []string               `json:"tags,omitempty" cbor:"21,keyasint,omitempty"`
	AdvertisedRoutes          []string               `json:"advertisedRoutes,omitempty" cbor:"22,keyasint,omitempty"`
	EnabledRoutes             []string               `json:"enabledRoutes,omitempty" cbor:"23,keyasint,omitempty"`
	Endpoints                 []string               `json:"endpoints,omitempty" cbor:"24,keyasint,omitempty"`
	MappingVariesByDestIP     bool                   `json:"mappingVariesByDestIP" cbor:"25,keyasint"`
	DERPLatency               map[string]DERPLatency `json:"derpLatency,omitempty" cbor:"26,keyasint,omitempty"`
	ClientSupports            *ClientSupports        `json:"clientSupports,omitempty" cbor:"27,keyasint,omitempty"`
}

// DERPLatency represents latency information for a DERP region
type DERPLatency struct {
	Preferred bool    `json:"preferred,omitempty" cbor:"0,keyasint,omitempty"`
	LatencyMs float64 `json:"latencyMs" cbor:"1,keyasint"`
}

// ClientSupports represents the client capabilities
type ClientSupports struct {
	HairPinning *bool `json:"hairPinning" cbor:"0,keyasint"`
	IPv6        bool  `json:"ipv6" cbor:"1,keyasint"`
	PCP         bool  `json:"pcp" cbor:"2,keyasint"`
	PMP         bool  `json:"pmp" cbor:"3,keyasint"`
	UDP         bool  `json:"udp" cbor:"4,keyasint"`
	UPnP        bool  `json:"upnp" cbor:"5,keyasint"`
}

// TailscaleNetwork represents the overall Tailscale network state
type TailscaleNetwork struct {
	Domain       string           `json:"domain" cbor:"0,keyasint"`
	TailnetName  string           `json:"tailnetName" cbor:"1,keyasint"`
	TotalNodes   int              `json:"totalNodes" cbor:"2,keyasint"`
	OnlineNodes  int              `json:"onlineNodes" cbor:"3,keyasint"`
	OfflineNodes int              `json:"offlineNodes" cbor:"4,keyasint"`
	Nodes        []*TailscaleNode `json:"nodes" cbor:"5,keyasint"`
	LastUpdated  time.Time        `json:"lastUpdated" cbor:"6,keyasint"`
}

// TailscaleConfig represents configuration for Tailscale API access
type TailscaleConfig struct {
	APIKey       string `json:"apiKey" cbor:"0,keyasint"`
	Tailnet      string `json:"tailnet" cbor:"1,keyasint"`
	ClientID     string `json:"clientId" cbor:"2,keyasint"`
	ClientSecret string `json:"clientSecret" cbor:"3,keyasint"`
	Enabled      bool   `json:"enabled" cbor:"4,keyasint"`
}
