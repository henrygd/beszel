package system

// PackageUpdate is one pending package update on the host.
type PackageUpdate struct {
	Name      string `json:"name" cbor:"0,keyasint"`
	Current   string `json:"current,omitempty" cbor:"1,keyasint,omitempty"` // installed version, empty if unknown
	Available string `json:"available" cbor:"2,keyasint"`
	Security  bool   `json:"security,omitempty" cbor:"3,keyasint,omitempty"`
}

// PackageUpdates is the detail payload returned by the agent for the
// GetPackageUpdates action. The counts in Info.PackageUpdates come from the same check.
type PackageUpdates struct {
	Manager string `json:"manager,omitempty" cbor:"0,keyasint,omitempty"`
	// CheckedAt is the Unix time in seconds of the last check, 0 if none has finished.
	CheckedAt int64 `json:"checkedAt,omitempty" cbor:"1,keyasint,omitempty"`
	// SecurityKnown is true if the package manager flags security updates per package.
	SecurityKnown bool            `json:"securityKnown,omitempty" cbor:"2,keyasint,omitempty"`
	Packages      []PackageUpdate `json:"packages" cbor:"3,keyasint"`
}
