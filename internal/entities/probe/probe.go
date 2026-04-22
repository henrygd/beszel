package probe

import "strconv"

// Config defines a network probe task sent from hub to agent.
type Config struct {
	Target   string `cbor:"0,keyasint" json:"target"`
	Protocol string `cbor:"1,keyasint" json:"protocol"` // "icmp", "tcp", or "http"
	Port     uint16 `cbor:"2,keyasint,omitempty" json:"port,omitempty"`
	Interval uint16 `cbor:"3,keyasint" json:"interval"` // seconds
}

// Result holds aggregated probe results for a single target.
//
// 0: avg response in ms
//
// 1: average response over the last hour in ms
//
// 2: min response over the last hour in ms
//
// 3: max response over the last hour in ms
//
// 4: packet loss percentage over the last hour (0-100)
type Result []float64

// Key returns the map key used for this probe config (e.g. "icmp:1.1.1.1", "tcp:host:443", "http:https://example.com").
func (c Config) Key() string {
	switch c.Protocol {
	case "tcp":
		return c.Protocol + ":" + c.Target + ":" + strconv.FormatUint(uint64(c.Port), 10)
	default:
		return c.Protocol + ":" + c.Target
	}
}
