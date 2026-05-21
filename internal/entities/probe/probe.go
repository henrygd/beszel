package probe

import "fmt"

// Config defines a network probe task sent from hub to agent.
type Config struct {
	Target   string `cbor:"0,keyasint" json:"target"`
	Protocol string `cbor:"1,keyasint" json:"protocol"` // "icmp", "tcp", or "http"
	Port     uint16 `cbor:"2,keyasint,omitempty" json:"port,omitempty"`
	Interval uint16 `cbor:"3,keyasint" json:"interval"` // seconds
}

// Result holds aggregated probe results for a single target.
type Result struct {
	AvgMs float64 `cbor:"0,keyasint" json:"avg"`
	MinMs float64 `cbor:"1,keyasint" json:"min"`
	MaxMs float64 `cbor:"2,keyasint" json:"max"`
	Loss  float64 `cbor:"3,keyasint" json:"loss"` // packet loss %
}

// Key returns the map key used for this probe config (e.g. "icmp:1.1.1.1", "tcp:host:443", "http:https://example.com").
func (c Config) Key() string {
	switch c.Protocol {
	case "tcp":
		return c.Protocol + ":" + c.Target + ":" + fmt.Sprintf("%d", c.Port)
	default:
		return c.Protocol + ":" + c.Target
	}
}
