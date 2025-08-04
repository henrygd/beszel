package tailscale

import (
	"time"
)

// TailscaleNode represents a node in the Tailscale network
type TailscaleNode struct {
	ID                   string    `json:"id" cbor:"0,keyasint"`
	Name                 string    `json:"name" cbor:"1,keyasint"`
	Hostname             string    `json:"hostname" cbor:"2,keyasint"`
	IP                   string    `json:"ip" cbor:"3,keyasint"`
	IPv6                 string    `json:"ipv6,omitempty" cbor:"4,keyasint,omitempty"`
	OS                   string    `json:"os" cbor:"5,keyasint"`
	Version              string    `json:"version" cbor:"6,keyasint"`
	LastSeen             time.Time `json:"lastSeen" cbor:"7,keyasint"`
	Online               bool      `json:"online" cbor:"8,keyasint"`
	Tags                 []string  `json:"tags,omitempty" cbor:"9,keyasint,omitempty"`
	IsExitNode           bool      `json:"isExitNode" cbor:"10,keyasint"`
	IsSubnetRouter       bool      `json:"isSubnetRouter" cbor:"11,keyasint"`
	MachineKey           string    `json:"machineKey" cbor:"12,keyasint"`
	NodeKey              string    `json:"nodeKey" cbor:"13,keyasint"`
	DiscoKey             string    `json:"discoKey" cbor:"14,keyasint"`
	Endpoints            []string  `json:"endpoints,omitempty" cbor:"15,keyasint,omitempty"`
	Derp                 string    `json:"derp" cbor:"16,keyasint"`
	InNetworkMap         bool      `json:"inNetworkMap" cbor:"17,keyasint"`
	InMagicSock          bool      `json:"inMagicSock" cbor:"18,keyasint"`
	InEngine             bool      `json:"inEngine" cbor:"19,keyasint"`
	Created              time.Time `json:"created" cbor:"20,keyasint"`
	KeyExpiry            time.Time `json:"keyExpiry" cbor:"21,keyasint"`
	Capabilities         []string  `json:"capabilities,omitempty" cbor:"22,keyasint,omitempty"`
	ComputedName         string    `json:"computedName" cbor:"23,keyasint"`
	ComputedNameWithHost string    `json:"computedNameWithHost" cbor:"24,keyasint"`
	PrimaryRoutes        []string  `json:"primaryRoutes,omitempty" cbor:"25,keyasint,omitempty"`
	AllowedIPs           []string  `json:"allowedIPs,omitempty" cbor:"26,keyasint,omitempty"`
	AdvertisedRoutes     []string  `json:"advertisedRoutes,omitempty" cbor:"27,keyasint,omitempty"`
	EnabledRoutes        []string  `json:"enabledRoutes,omitempty" cbor:"28,keyasint,omitempty"`
	IsEphemeral          bool      `json:"isEphemeral" cbor:"29,keyasint"`
	Expired              bool      `json:"expired" cbor:"30,keyasint"`
	KeyExpired           bool      `json:"keyExpired" cbor:"31,keyasint"`
	ConnectedToControl   bool      `json:"connectedToControl" cbor:"32,keyasint"`
	UpdateAvailable      bool      `json:"updateAvailable" cbor:"33,keyasint"`
	Authorized           bool      `json:"authorized" cbor:"34,keyasint"`
	IsExternal           bool      `json:"isExternal" cbor:"35,keyasint"`
	KeyExpiryDisabled    bool           `json:"keyExpiryDisabled" cbor:"36,keyasint"`
	ClientSupports       *ClientSupports `json:"clientSupports,omitempty" cbor:"37,keyasint,omitempty"`
}

// ClientSupports represents the client capabilities
type ClientSupports struct {
	HairPinning bool `json:"hairPinning" cbor:"0,keyasint"`
	IPV6        bool `json:"ipv6" cbor:"1,keyasint"`
	PCP         bool `json:"pcp" cbor:"2,keyasint"`
	PMP         bool `json:"pmp" cbor:"3,keyasint"`
	UDP         bool `json:"udp" cbor:"4,keyasint"`
	UPNP        bool `json:"upnp" cbor:"5,keyasint"`
}

// TailscaleNetwork represents the overall Tailscale network state
type TailscaleNetwork struct {
	Domain        string           `json:"domain" cbor:"0,keyasint"`
	TailnetName   string           `json:"tailnetName" cbor:"1,keyasint"`
	TotalNodes    int              `json:"totalNodes" cbor:"2,keyasint"`
	OnlineNodes   int              `json:"onlineNodes" cbor:"3,keyasint"`
	OfflineNodes  int              `json:"offlineNodes" cbor:"4,keyasint"`
	ExpiredNodes  int              `json:"expiredNodes" cbor:"5,keyasint"`
	ExitNodes     int              `json:"exitNodes" cbor:"6,keyasint"`
	SubnetRouters int              `json:"subnetRouters" cbor:"7,keyasint"`
	Nodes         []*TailscaleNode `json:"nodes" cbor:"8,keyasint"`
	LastUpdated   time.Time        `json:"lastUpdated" cbor:"9,keyasint"`
}

// TailscaleStats represents aggregated statistics about the Tailscale network
type TailscaleStats struct {
	TotalNodes       int       `json:"totalNodes" cbor:"0,keyasint"`
	OnlineNodes      int       `json:"onlineNodes" cbor:"1,keyasint"`
	OfflineNodes     int       `json:"offlineNodes" cbor:"2,keyasint"`
	ExpiredNodes     int       `json:"expiredNodes" cbor:"3,keyasint"`
	ExitNodes        int       `json:"exitNodes" cbor:"4,keyasint"`
	SubnetRouters    int       `json:"subnetRouters" cbor:"5,keyasint"`
	EphemeralNodes   int       `json:"ephemeralNodes" cbor:"6,keyasint"`
	NodesWithUpdates int       `json:"nodesWithUpdates" cbor:"7,keyasint"`
	LastUpdated      time.Time `json:"lastUpdated" cbor:"8,keyasint"`
}

// TailscaleConfig represents configuration for Tailscale API access
type TailscaleConfig struct {
	APIKey       string `json:"apiKey" cbor:"0,keyasint"`
	Tailnet      string `json:"tailnet" cbor:"1,keyasint"`
	ClientID     string `json:"clientId" cbor:"2,keyasint"`
	ClientSecret string `json:"clientSecret" cbor:"3,keyasint"`
	Enabled      bool   `json:"enabled" cbor:"4,keyasint"`
}
