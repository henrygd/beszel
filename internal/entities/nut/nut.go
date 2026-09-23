// Package nut defines the data model for UPS/PDU information collected from
// Network UPS Tools (NUT) and shipped from the agent to the hub.
package nut

// Health states derived from the NUT ups.status flags. They follow a severity
// ordering used by alerts (see internal/alerts) and the UI.
const (
	HealthOnline     = "ONLINE"
	HealthOnBattery  = "ON_BATTERY"
	HealthLowBattery = "LOW_BATTERY"
	HealthOverload   = "OVERLOAD"
	HealthFault      = "FAULT"
	HealthUnknown    = "UNKNOWN"
)

// Device types reported for a NUT device.
const (
	DeviceTypeUPS   = "ups"
	DeviceTypePDU   = "pdu"
	DeviceTypeATS   = "ats"
	DeviceTypeOther = "other"
)

// NutData is the per-device wire record sent from the agent to the hub.
// Values are parsed from the flat variable map produced by `upsc -j`.
type NutData struct {
	// Identity
	Model        string `json:"mn,omitempty" cbor:"0,keyasint,omitempty"`
	Manufacturer string `json:"mf,omitempty" cbor:"1,keyasint,omitempty"`
	Serial       string `json:"sn,omitempty" cbor:"2,keyasint,omitempty"`
	Firmware     string `json:"fv,omitempty" cbor:"3,keyasint,omitempty"`
	Driver       string `json:"dr,omitempty" cbor:"4,keyasint,omitempty"`
	DisplayName  string `json:"dn,omitempty" cbor:"5,keyasint,omitempty"` // ups.conf description
	DeviceType   string `json:"dt,omitempty" cbor:"6,keyasint,omitempty"` // ups | pdu | ats | other

	// Status
	Status string `json:"st,omitempty" cbor:"7,keyasint,omitempty"` // raw ups.status (e.g. "OL")
	Health string `json:"h,omitempty" cbor:"8,keyasint,omitempty"`  // derived health state

	// Battery
	BatteryCharge  float64 `json:"bc,omitempty" cbor:"9,keyasint,omitempty"`  // percent
	BatteryVoltage float64 `json:"bv,omitempty" cbor:"10,keyasint,omitempty"` // volts
	BatteryRuntime int64   `json:"br,omitempty" cbor:"11,keyasint,omitempty"` // seconds

	// Voltages
	InputVoltage  float64 `json:"iv,omitempty" cbor:"12,keyasint,omitempty"`  // volts
	OutputVoltage float64 `json:"ov,omitempty" cbor:"13,keyasint,omitempty"`  // volts
	InputNominal  float64 `json:"ivn,omitempty" cbor:"14,keyasint,omitempty"` // nominal input volts

	// Load / power
	Load          float64 `json:"ld,omitempty" cbor:"15,keyasint,omitempty"` // percent
	OutputCurrent float64 `json:"oc,omitempty" cbor:"16,keyasint,omitempty"` // amps
	OutputPower   float64 `json:"op,omitempty" cbor:"17,keyasint,omitempty"` // watts

	// Outlets (PDU)
	Outlets []*NutOutlet `json:"o,omitempty" cbor:"18,keyasint,omitempty"`
}

// NutOutlet describes a single PDU outlet.
type NutOutlet struct {
	ID      string  `json:"id" cbor:"0,keyasint"`
	Desc    string  `json:"d,omitempty" cbor:"1,keyasint,omitempty"`
	Status  string  `json:"s,omitempty" cbor:"2,keyasint,omitempty"` // on | off | unknown
	Current float64 `json:"c,omitempty" cbor:"3,keyasint,omitempty"` // amps
	Power   float64 `json:"p,omitempty" cbor:"4,keyasint,omitempty"` // watts
	Energy  float64 `json:"e,omitempty" cbor:"5,keyasint,omitempty"` // kWh
	Voltage float64 `json:"v,omitempty" cbor:"6,keyasint,omitempty"` // volts
}

// NutDataResponse is the agent-to-hub payload. Complete reports whether every
// discovered device was collected; older agents omit it, so hubs must not prune
// from it.
type NutDataResponse struct {
	Data     map[string]NutData `json:"data" cbor:"0,keyasint"`
	Complete bool               `json:"complete" cbor:"1,keyasint,omitempty"`
}