package hardware

// Security is the platform's boot and trust state.
type Security struct {
	// SecureBoot is "enabled" or "disabled" on UEFI systems, "legacy" on BIOS
	// boot, and empty when unknown.
	SecureBoot string `json:"secure_boot,omitempty" cbor:"0,keyasint,omitempty"`
	// TPM is the TPM specification version ("2.0", "1.2"), "present" when the kernel
	// does not report it, or empty when no TPM was found.
	TPM string `json:"tpm,omitempty" cbor:"1,keyasint,omitempty"`
}

// Battery is one system battery (laptops and other battery-powered systems);
// peripheral batteries (mice, headsets) are not included.
type Battery struct {
	Name         string `json:"name" cbor:"0,keyasint"`
	Manufacturer string `json:"manufacturer,omitempty" cbor:"1,keyasint,omitempty"`
	Model        string `json:"model,omitempty" cbor:"2,keyasint,omitempty"`
	Technology   string `json:"technology,omitempty" cbor:"3,keyasint,omitempty"`
	// HealthPct is the full charge capacity as a percentage of the design capacity.
	HealthPct  int    `json:"health_pct,omitempty" cbor:"4,keyasint,omitempty"`
	CycleCount int    `json:"cycle_count,omitempty" cbor:"5,keyasint,omitempty"`
	Serial     string `json:"serial,omitempty" cbor:"6,keyasint,omitempty"` // identifier
}
